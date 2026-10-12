package challenge

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

const testWindow = 5 * time.Second

// newTestStore 返回使用固定签发器的存储：
// 通关凭证形如 pass-<sid>-<序号>，便于断言复用与重签。
func newTestStore(cfg Config) (*Store, *issueCounter) {
	counter := &issueCounter{}
	cfg.IssuePass = func(sid string, now time.Time) (string, time.Time, error) {
		n := counter.next()
		return fmt.Sprintf("pass-%s-%d", sid, n), now.Add(cfg.PassTTL), nil
	}
	return NewStore(cfg), counter
}

type issueCounter struct {
	mu sync.Mutex
	n  int
}

func (c *issueCounter) next() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.n++
	return c.n
}

func (c *issueCounter) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.n
}

func newSession(store *Store, id string) *Session {
	return store.Create(id, time.Now().Add(30*time.Minute))
}

func submit(t *testing.T, store *Store, sess *Session, now time.Time, seq int) Outcome {
	t.Helper()
	out, err := sess.Submit(now, seq, store.PassIssuer())
	if err != nil {
		t.Fatalf("Submit(%d) 意外出错: %v", seq, err)
	}
	return out
}

func TestSubmitPassWithShuffledOrder(t *testing.T) {
	store, counter := newTestStore(Config{Window: testWindow, PassTTL: 10 * time.Minute})
	sess := newSession(store, "s1")
	now := time.Now()

	for _, seq := range []int{3, 1, 7, 2, 9, 4, 10, 6, 8} {
		out := submit(t, store, sess, now, seq)
		if out.Kind != KindProgress && out.Kind != KindNewRound {
			t.Fatalf("编号 %d 应为进行中，得到 %v", seq, out.Kind)
		}
	}
	out := submit(t, store, sess, now, 5)
	if out.Kind != KindPass {
		t.Fatalf("乱序集齐应通关，得到 %v", out.Kind)
	}
	if out.PassToken == "" {
		t.Fatal("通关响应应携带通行证")
	}
	if counter.count() != 1 {
		t.Fatalf("应只签发一次通关凭证，实际 %d 次", counter.count())
	}

	// 通过后 POST 与 GET 均返回通关信息，不重新计数。
	again := submit(t, store, sess, now.Add(time.Second), 7)
	if again.Kind != KindPass {
		t.Fatalf("通过后再次提交应返回通关信息，得到 %v", again.Kind)
	}
	if again.PassToken != out.PassToken {
		t.Fatal("通关凭证未过期时应复用同一张")
	}
	st, err := sess.Status(now.Add(time.Second), store.PassIssuer())
	if err != nil || st.Kind != StatusPassed {
		t.Fatalf("通过后 GET 应返回通关状态，得到 %v err=%v", st.Kind, err)
	}
}

func TestRejectAscendingAndDescenDing(t *testing.T) {
	for _, tc := range []struct {
		name string
		seqs []int
		asc  bool
	}{
		{"升序", []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, true},
		{"降序", []int{10, 9, 8, 7, 6, 5, 4, 3, 2, 1}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store, counter := newTestStore(Config{Window: testWindow, PassTTL: 10 * time.Minute})
			sess := newSession(store, "s1")
			now := time.Now()

			for _, seq := range tc.seqs[:9] {
				submit(t, store, sess, now, seq)
			}
			out := submit(t, store, sess, now, tc.seqs[9])
			if out.Kind != KindRejected {
				t.Fatalf("完整%s应被拒绝，得到 %v", tc.name, out.Kind)
			}
			if out.RejectAscending != tc.asc {
				t.Fatalf("拒绝方向错误: want asc=%v got %v", tc.asc, out.RejectAscending)
			}
			if out.RejectOrder == "" {
				t.Fatal("409 应说明本轮首次接收顺序")
			}
			if counter.count() != 0 {
				t.Fatal("被拒绝的轮次不应签发通关凭证")
			}

			// GET 可查询最近一次失败原因。
			st, err := sess.Status(now, store.PassIssuer())
			if err != nil || st.Kind != StatusRejected {
				t.Fatalf("拒绝后 GET 应展示失败原因，得到 %v err=%v", st.Kind, err)
			}
			if st.RejectAscending != tc.asc {
				t.Fatalf("GET 中的拒绝方向错误: %v", st.RejectAscending)
			}

			// 下一个有效提交开启新一轮，拒绝时的最后一个编号不自动计入。
			next := submit(t, store, sess, now.Add(time.Millisecond), tc.seqs[9])
			if next.Kind != KindRoundAfterReject {
				t.Fatalf("拒绝后的提交应开启新一轮，得到 %v", next.Kind)
			}
			if next.Count != 1 {
				t.Fatalf("新轮次进度应为 1，得到 %d", next.Count)
			}
			// 开始新一轮后失败原因被清除。
			st, _ = sess.Status(now, store.PassIssuer())
			if st.Kind != StatusActive {
				t.Fatalf("新一轮进行中 GET 应展示进度，得到 %v", st.Kind)
			}
		})
	}
}

func TestDuplicatesDoNotCountOrAlterOrder(t *testing.T) {
	store, _ := newTestStore(Config{Window: testWindow, PassTTL: 10 * time.Minute})
	sess := newSession(store, "s1")
	now := time.Now()

	out := submit(t, store, sess, now, 1)
	if out.Kind != KindNewRound {
		t.Fatalf("首个编号应开启新一轮，得到 %v", out.Kind)
	}
	dup := submit(t, store, sess, now.Add(time.Millisecond), 1)
	if dup.Kind != KindDuplicate || dup.Count != 1 {
		t.Fatalf("重复编号不应计数: kind=%v count=%d", dup.Kind, dup.Count)
	}
	// 重复编号不能破坏升降序判定：插入重复后仍按完整升序拒绝。
	for _, seq := range []int{2, 2, 3, 4, 5, 6, 7, 8, 9, 9, 10} {
		out = submit(t, store, sess, now.Add(2*time.Millisecond), seq)
	}
	if out.Kind != KindRejected || !out.RejectAscending {
		t.Fatalf("插入重复编号的完整升序仍应被拒绝，得到 kind=%v asc=%v", out.Kind, out.RejectAscending)
	}
	if out.RejectOrder != "1, 2, 3, 4, 5, 6, 7, 8, 9, 10" {
		t.Fatalf("首次接收序列应不含重复编号，得到 %q", out.RejectOrder)
	}
}

func TestNonMonotonicPrefixPasses(t *testing.T) {
	store, _ := newTestStore(Config{Window: testWindow, PassTTL: 10 * time.Minute})
	sess := newSession(store, "s1")
	now := time.Now()

	// 1,2,3,4,5,6,7,8,10,9：局部递增不应提前拒绝。
	for _, seq := range []int{1, 2, 3, 4, 5, 6, 7, 8, 10, 9} {
		out := submit(t, store, sess, now, seq)
		if out.Kind == KindRejected {
			t.Fatal("非完整升降序不应被拒绝")
		}
	}
	st, _ := sess.Status(now, store.PassIssuer())
	if st.Kind != StatusPassed {
		t.Fatalf("非完整升降序应通关，得到 %v", st.Kind)
	}
}

func TestTimeoutResetsLazily(t *testing.T) {
	store, _ := newTestStore(Config{Window: 20 * time.Millisecond, PassTTL: 10 * time.Minute})
	sess := newSession(store, "s1")
	start := time.Now()

	submit(t, store, sess, start, 1)

	// GET 只展示超时状态，不启动新轮次。
	st, err := sess.Status(start.Add(30*time.Millisecond), store.PassIssuer())
	if err != nil || st.Kind != StatusTimedOut {
		t.Fatalf("截止后 GET 应展示超时，得到 %v err=%v", st.Kind, err)
	}

	// 超时后的下一个有效提交开启新轮次，旧编号可重新计数。
	out := submit(t, store, sess, start.Add(30*time.Millisecond), 1)
	if out.Kind != KindRoundAfterTimeout {
		t.Fatalf("超时后提交应开启新一轮，得到 %v", out.Kind)
	}
	if out.Count != 1 {
		t.Fatalf("新轮次进度应为 1，得到 %d", out.Count)
	}
}

func TestPassTokenReissue(t *testing.T) {
	store, counter := newTestStore(Config{Window: testWindow, PassTTL: 20 * time.Millisecond})
	sess := newSession(store, "s1")
	now := time.Now()

	for _, seq := range []int{3, 1, 7, 2, 9, 4, 10, 6, 8, 5} {
		submit(t, store, sess, now, seq)
	}
	if counter.count() != 1 {
		t.Fatalf("通关时应签发一次凭证，实际 %d", counter.count())
	}

	// 通关 JWT 过期但初始会话仍有效时，可重新签发。
	st, err := sess.Status(now.Add(30*time.Millisecond), store.PassIssuer())
	if err != nil || st.Kind != StatusPassed {
		t.Fatalf("过期后 GET 仍应返回通关信息，得到 %v err=%v", st.Kind, err)
	}
	if counter.count() != 2 {
		t.Fatalf("通关凭证过期后应重签，实际签发 %d 次", counter.count())
	}
}

func TestSessionsIndependent(t *testing.T) {
	store, _ := newTestStore(Config{Window: testWindow, PassTTL: 10 * time.Minute})
	now := time.Now()
	a := newSession(store, "a")
	b := newSession(store, "b")

	submit(t, store, a, now, 1)
	submit(t, store, a, now, 2)
	submit(t, store, b, now, 5)

	stA, _ := a.Status(now, store.PassIssuer())
	stB, _ := b.Status(now, store.PassIssuer())
	if stA.Count != 2 || stB.Count != 1 {
		t.Fatalf("会话进度不应互相影响: a=%d b=%d", stA.Count, stB.Count)
	}
}

func TestConcurrentSameSeqCountsOnce(t *testing.T) {
	store, _ := newTestStore(Config{Window: testWindow, PassTTL: 10 * time.Minute})
	sess := newSession(store, "s1")
	now := time.Now()

	const n = 32
	var wg sync.WaitGroup
	kinds := make([]OutcomeKind, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			out, err := sess.Submit(now, 7, store.PassIssuer())
			if err != nil {
				t.Errorf("并发提交出错: %v", err)
				return
			}
			kinds[i] = out.Kind
		}(i)
	}
	wg.Wait()

	newRound := 0
	for _, k := range kinds {
		switch k {
		case KindNewRound, KindRoundAfterTimeout, KindRoundAfterReject:
			newRound++
		case KindDuplicate:
		default:
			t.Fatalf("并发重复编号出现意外结果: %v", k)
		}
	}
	if newRound != 1 {
		t.Fatalf("同一编号并发提交只应计数一次，实际开启轮次 %d", newRound)
	}
	st, _ := sess.Status(now, store.PassIssuer())
	if st.Count != 1 {
		t.Fatalf("进度应为 1，得到 %d", st.Count)
	}
}

func TestConcurrentCompleteRoundIsConsistent(t *testing.T) {
	store, _ := newTestStore(Config{Window: testWindow, PassTTL: 10 * time.Minute})
	sess := newSession(store, "s1")
	now := time.Now()

	// 并发提交 1～10；真实接收顺序可能恰好为完整升降序而被拒绝，
	// 此时按题目规则重试整轮，最终必须能通过且无数据竞争。
	for attempt := 0; attempt < 20; attempt++ {
		var wg sync.WaitGroup
		for seq := 1; seq <= 10; seq++ {
			wg.Add(1)
			go func(seq int) {
				defer wg.Done()
				_, _ = sess.Submit(now.Add(time.Duration(attempt)*time.Second), seq, store.PassIssuer())
			}(seq)
		}
		wg.Wait()

		st, _ := sess.Status(now.Add(time.Duration(attempt)*time.Second), store.PassIssuer())
		switch st.Kind {
		case StatusPassed:
			return // 通关，无竞争且状态一致
		case StatusRejected:
			// 恰好完整升降序被拒：通过下一个有效提交重开整轮。
			var wg2 sync.WaitGroup
			for seq := 1; seq <= 10; seq++ {
				wg2.Add(1)
				go func(seq int) {
					defer wg2.Done()
					_, _ = sess.Submit(now.Add(time.Duration(attempt+1)*time.Second), seq, store.PassIssuer())
				}(seq)
			}
			wg2.Wait()
			st2, _ := sess.Status(now.Add(time.Duration(attempt+1)*time.Second), store.PassIssuer())
			if st2.Kind == StatusPassed {
				return
			}
		case StatusActive, StatusTimedOut:
			t.Fatalf("十次唯一编号提交后不应停留在 %v", st.Kind)
		}
	}
	t.Fatal("并发多轮重试后仍未通关")
}

func TestJanitorRemovesExpiredSessions(t *testing.T) {
	store, _ := newTestStore(Config{
		Window:          5 * time.Second,
		SessionTTL:      10 * time.Millisecond,
		PassTTL:         10 * time.Millisecond,
		JanitorInterval: 5 * time.Millisecond,
	})
	store.Create("old", time.Now().Add(-time.Second))
	stop := store.StartJanitor()
	defer stop()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && store.Len() != 0 {
		time.Sleep(2 * time.Millisecond)
	}
	if store.Len() != 0 {
		t.Fatal("过期会话应被后台清理")
	}
}

func TestJanitorKeepsPassedSessionUntilPassExpiry(t *testing.T) {
	store, _ := newTestStore(Config{
		Window:          5 * time.Second,
		SessionTTL:      10 * time.Millisecond,
		PassTTL:         500 * time.Millisecond,
		JanitorInterval: 5 * time.Millisecond,
	})
	sess := store.Create("s1", time.Now().Add(10*time.Millisecond))
	now := time.Now()
	for _, seq := range []int{3, 1, 7, 2, 9, 4, 10, 6, 8, 5} {
		submit(t, store, sess, now, seq)
	}

	stop := store.StartJanitor()
	defer stop()

	// 初始会话已过期，但通关 JWT 仍有效：会话必须保留。
	time.Sleep(60 * time.Millisecond)
	if _, ok := store.Get("s1"); !ok {
		t.Fatal("已通过会话在通关 JWT 有效期内不应被清理")
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := store.Get("s1"); !ok {
			return // 通关 JWT 也过期后，最终被清理
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("全部凭证过期后会话应被清理")
}
