// Package challenge 实现判题状态机。
//
// 每个初始 JWT 的 jti 对应一个独立会话。“检查是否通过、检查窗口、
// 必要时重置、去重、更新集合与接收序列、集齐后判定顺序、标记通过或拒绝”
// 全部在会话锁内完成；全站仅持有 map 级别的短读写锁，不持有全局长锁。
package challenge

import (
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"
)

// 题目规定的默认时长。
const (
	DefaultWindow          = 5 * time.Second
	DefaultSessionTTL      = 30 * time.Minute
	DefaultPassTTL         = 10 * time.Minute
	DefaultJanitorInterval = time.Minute
)

// 时间窗口为半开区间 [start, start+Window)；
// now.Before(deadline) 为 false 时视为到达截止时间。
type PassIssuer func(sid string, now time.Time) (token string, expires time.Time, err error)

type Config struct {
	Window          time.Duration
	SessionTTL      time.Duration
	PassTTL         time.Duration
	JanitorInterval time.Duration
	IssuePass       PassIssuer
}

func (c *Config) normalize() {
	if c.Window <= 0 {
		c.Window = DefaultWindow
	}
	if c.SessionTTL <= 0 {
		c.SessionTTL = DefaultSessionTTL
	}
	if c.PassTTL <= 0 {
		c.PassTTL = DefaultPassTTL
	}
	if c.JanitorInterval <= 0 {
		c.JanitorInterval = DefaultJanitorInterval
	}
	if c.IssuePass == nil {
		c.IssuePass = func(string, time.Time) (string, time.Time, error) {
			return "", time.Time{}, errors.New("未配置通关凭证签发器")
		}
	}
}

// OutcomeKind 是一次有效提交的处理结果类型。
type OutcomeKind int

const (
	KindPass OutcomeKind = iota
	KindNewRound
	KindProgress
	KindDuplicate
	KindRoundAfterTimeout
	KindRoundAfterReject
	KindRejected // 返回 409
)

// Outcome 携带一次提交判定的全部信息，由上层翻译成响应文案。
type Outcome struct {
	Kind            OutcomeKind
	Seq             int
	Count           int
	Order           []int
	PassToken       string
	RejectAscending bool
	RejectOrder     string
}

// StatusKind 是 GET 挑战接口可查询到的会话状态。
type StatusKind int

const (
	StatusNotStarted StatusKind = iota
	StatusActive
	StatusTimedOut
	StatusRejected
	StatusPassed
)

type Status struct {
	Kind            StatusKind
	Count           int
	Order           []int
	PassToken       string
	RejectAscending bool
	RejectOrder     string
}

// Session 是一个独立挑战会话。所有字段只能在 mu 临界区内读写。
type Session struct {
	mu        sync.Mutex
	id        string
	expiresAt time.Time // 初始会话过期时间（与初始 JWT 的 exp 一致）
	window    time.Duration

	active    bool      // 本轮是否进行中
	deadline  time.Time // 本轮截止时间
	collected uint16    // 已收到编号的 10 位 bitset
	order     []int     // 不重复编号的首次接收序列

	rejected        bool
	rejectAscending bool
	rejectOrder     string

	passed      bool
	passToken   string
	passExpires time.Time
}

// Store 管理全部会话。map 访问由短读写锁保护，
// 判题逻辑在会话自身的锁内完成，互不阻塞。
type Store struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	cfg      Config
}

func NewStore(cfg Config) *Store {
	cfg.normalize()
	return &Store{
		sessions: make(map[string]*Session),
		cfg:      cfg,
	}
}

// Create 新建会话。jti 由 token 包生成，碰撞概率可忽略。
func (s *Store) Create(id string, expiresAt time.Time) *Session {
	sess := &Session{
		id:        id,
		expiresAt: expiresAt,
		window:    s.cfg.Window,
	}
	s.mu.Lock()
	s.sessions[id] = sess
	s.mu.Unlock()
	return sess
}

func (s *Store) Get(id string) (*Session, bool) {
	s.mu.RLock()
	sess, ok := s.sessions[id]
	s.mu.RUnlock()
	return sess, ok
}

// Len 返回当前会话数，供测试与清理验证使用。
func (s *Store) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.sessions)
}

func (s *Store) Window() time.Duration     { return s.cfg.Window }
func (s *Store) SessionTTL() time.Duration { return s.cfg.SessionTTL }
func (s *Store) PassTTL() time.Duration    { return s.cfg.PassTTL }
func (s *Store) PassIssuer() PassIssuer    { return s.cfg.IssuePass }

// StartJanitor 启动后台清扫协程，定期移除所有已签发凭证均过期的会话，
// 避免已通过会话泄漏内存。返回的 stop 幂等。
func (s *Store) StartJanitor() (stop func()) {
	ticker := time.NewTicker(s.cfg.JanitorInterval)
	stopCh := make(chan struct{})
	go func() {
		for {
			select {
			case <-stopCh:
				ticker.Stop()
				return
			case now := <-ticker.C:
				s.cleanup(now)
			}
		}
	}()
	var once sync.Once
	return func() { once.Do(func() { close(stopCh) }) }
}

func (s *Store) cleanup(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, sess := range s.sessions {
		sess.mu.Lock()
		retention := sess.expiresAt
		// 已通过的会话需保留到通关 JWT 也过期，供领奖接口校验通过状态。
		if sess.passed && sess.passExpires.After(retention) {
			retention = sess.passExpires
		}
		sess.mu.Unlock()
		if now.After(retention) {
			delete(s.sessions, id)
		}
	}
}

// Submit 处理一个通过格式校验的编号。整个判定在同一临界区内完成，
// 并发安全；判题时刻以进入会话锁后的服务端时钟读数（含单调时钟）为准。
func (sess *Session) Submit(now time.Time, seq int, issue PassIssuer) (Outcome, error) {
	sess.mu.Lock()
	defer sess.mu.Unlock()

	// 已通过：GET 或有效 POST 均返回通关信息，不重新计数。
	if sess.passed {
		tok, err := sess.ensurePass(now, issue)
		if err != nil {
			return Outcome{}, err
		}
		return Outcome{Kind: KindPass, PassToken: tok}, nil
	}

	// 惰性超时判定：到达截止时间即清空旧进度。
	prevTimedOut := false
	if sess.active && !now.Before(sess.deadline) {
		sess.active = false
		sess.collected = 0
		sess.order = nil
		prevTimedOut = true
	}

	// 未进行中：本次提交开启新一轮，该编号计为第一个。
	if !sess.active {
		prevRejected := sess.rejected
		prevRejectAscending := sess.rejectAscending
		sess.active = true
		sess.deadline = now.Add(sess.window)
		sess.collected = 1 << seq
		sess.order = []int{seq}
		sess.rejected = false
		sess.rejectOrder = ""
		kind := KindNewRound
		switch {
		case prevTimedOut:
			kind = KindRoundAfterTimeout
		case prevRejected:
			// 携带上一轮的拒绝方向，供响应文案说明具体原因。
			kind = KindRoundAfterReject
		}
		return Outcome{Kind: kind, Seq: seq, Count: 1, RejectAscending: prevRejectAscending}, nil
	}

	// 重复编号：不计数、不追加序列、不延长截止时间。
	bit := uint16(1) << seq
	if sess.collected&bit != 0 {
		return Outcome{Kind: KindDuplicate, Seq: seq, Count: len(sess.order)}, nil
	}

	sess.collected |= bit
	sess.order = append(sess.order, seq)

	// 仅在集齐 10 个不同编号时判定完整序列。
	if len(sess.order) == 10 {
		if ascending(sess.order) || descending(sess.order) {
			asc := ascending(sess.order)
			orderStr := joinOrder(orderSnapshot(sess.order))
			sess.active = false
			sess.collected = 0
			sess.order = nil
			sess.rejected = true
			sess.rejectAscending = asc
			sess.rejectOrder = orderStr
			return Outcome{
				Kind:            KindRejected,
				RejectAscending: asc,
				RejectOrder:     orderStr,
			}, nil
		}
		tok, exp, err := issue(sess.id, now)
		if err != nil {
			return Outcome{}, err
		}
		sess.passed = true
		sess.passToken = tok
		sess.passExpires = exp
		return Outcome{Kind: KindPass, PassToken: tok}, nil
	}
	return Outcome{Kind: KindProgress, Seq: seq, Count: len(sess.order), Order: orderSnapshot(sess.order)}, nil
}

// Status 查询会话状态供 GET 展示。GET 不修改进度、不启动或延长计时；
// 超时与拒绝均通过惰性判定呈现，重置只发生在下一个有效 POST 中。
func (sess *Session) Status(now time.Time, issue PassIssuer) (Status, error) {
	sess.mu.Lock()
	defer sess.mu.Unlock()

	if sess.passed {
		tok, err := sess.ensurePass(now, issue)
		if err != nil {
			return Status{}, err
		}
		return Status{Kind: StatusPassed, PassToken: tok}, nil
	}
	if sess.active && !now.Before(sess.deadline) {
		return Status{Kind: StatusTimedOut, Count: len(sess.order), Order: orderSnapshot(sess.order)}, nil
	}
	if sess.active {
		return Status{Kind: StatusActive, Count: len(sess.order), Order: orderSnapshot(sess.order)}, nil
	}
	if sess.rejected {
		return Status{
			Kind:            StatusRejected,
			RejectAscending: sess.rejectAscending,
			RejectOrder:     sess.rejectOrder,
		}, nil
	}
	return Status{Kind: StatusNotStarted}, nil
}

// HasPassed 供奖励接口校验关联会话的通过状态。
func (sess *Session) HasPassed() bool {
	sess.mu.Lock()
	defer sess.mu.Unlock()
	return sess.passed
}

// ensurePass 复用尚未过期的通关 JWT；已过期但初始会话仍有效时可重新签发。
func (sess *Session) ensurePass(now time.Time, issue PassIssuer) (string, error) {
	if now.Before(sess.passExpires) {
		return sess.passToken, nil
	}
	tok, exp, err := issue(sess.id, now)
	if err != nil {
		return "", err
	}
	sess.passToken, sess.passExpires = tok, exp
	return tok, nil
}

func ascending(order []int) bool {
	for i, v := range order {
		if v != i+1 {
			return false
		}
	}
	return true
}

func descending(order []int) bool {
	for i, v := range order {
		if v != 10-i {
			return false
		}
	}
	return true
}

func orderSnapshot(order []int) []int {
	return append([]int(nil), order...)
}

func joinOrder(order []int) string {
	parts := make([]string, len(order))
	for i, v := range order {
		parts[i] = strconv.Itoa(v)
	}
	return strings.Join(parts, ", ")
}
