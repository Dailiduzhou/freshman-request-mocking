package api

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"requestmocking/internal/challenge"
	"requestmocking/internal/token"
)

// newTestServer 用可配置的时长构建站点。默认值与生产一致。
func newTestServer(t *testing.T, mutate func(*challenge.Config)) (*httptest.Server, *challenge.Store, *token.Manager) {
	t.Helper()
	tm := token.New([]byte("test-secret-for-httptest-only"))
	cfg := challenge.Config{
		Window:     challenge.DefaultWindow,
		SessionTTL: challenge.DefaultSessionTTL,
		PassTTL:    challenge.DefaultPassTTL,
	}
	if mutate != nil {
		mutate(&cfg)
	}
	store := newStoreWithIssuer(cfg, tm)
	r := NewRouter(Deps{Store: store, Tokens: tm})
	ts := httptest.NewServer(r)
	t.Cleanup(ts.Close)
	return ts, store, tm
}

// newStoreWithIssuer 将通关凭证签发器接入 store。
func newStoreWithIssuer(cfg challenge.Config, tm *token.Manager) *challenge.Store {
	cfg.IssuePass = func(sid string, now time.Time) (string, time.Time, error) {
		tok, _, exp, err := tm.Issue(token.StageReward, sid, cfg.PassTTL, now)
		return tok, exp, err
	}
	return challenge.NewStore(cfg)
}

// do 发送请求并返回状态码、响应头与正文。
func do(t *testing.T, method, url, auth, contentType, body string) (int, http.Header, string) {
	t.Helper()
	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatalf("构造请求失败: %v", err)
	}
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("读取响应失败: %v", err)
	}
	return res.StatusCode, res.Header, string(b)
}

func tokenFrom(t *testing.T, body, prefix string) string {
	t.Helper()
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, prefix) {
			tok := strings.TrimSpace(strings.TrimPrefix(line, prefix))
			if tok != "" {
				return tok
			}
		}
	}
	t.Fatalf("响应中未找到 %q 行:\n%s", prefix, body)
	return ""
}

func getTicket(t *testing.T, ts *httptest.Server) string {
	t.Helper()
	status, _, body := do(t, "GET", ts.URL+"/api/ticket", "", "", "")
	if status != 200 {
		t.Fatalf("领票失败: %d\n%s", status, body)
	}
	return tokenFrom(t, body, "JWT: ")
}

func postSeq(t *testing.T, ts *httptest.Server, auth string, body string) (int, string) {
	t.Helper()
	status, _, resp := do(t, "POST", ts.URL+"/api/challenge", auth, "application/json", body)
	return status, resp
}

// ---- 流程与提示（验收清单 6.1）----

func TestHomeRevealsNextStepWithoutBearer(t *testing.T) {
	ts, _, _ := newTestServer(t, nil)
	status, h, body := do(t, "GET", ts.URL+"/", "", "", "")
	if status != 200 {
		t.Fatalf("首页状态码 %d", status)
	}
	if got := h.Get("X-Next-Step"); got != "/api/ticket" {
		t.Fatalf("首页应通过 X-Next-Step 暴露入口，得到 %q", got)
	}
	if strings.Contains(body, "Bearer") || strings.Contains(h.Get("X-Next-Step"), "Bearer") {
		t.Fatal("首页不应提前泄露 Bearer 写法")
	}
}

func TestTicketIssuesJWTWithoutBearerHint(t *testing.T) {
	ts, _, tm := newTestServer(t, nil)
	status, h, body := do(t, "GET", ts.URL+"/api/ticket", "", "", "")
	if status != 200 {
		t.Fatalf("领票状态码 %d", status)
	}
	if h.Get("X-Next-Step") != "/api/challenge" {
		t.Fatalf("领票应指向挑战接口，得到 %q", h.Get("X-Next-Step"))
	}
	if h.Get("Cache-Control") != "no-store" {
		t.Fatal("领票响应必须 no-store")
	}
	if strings.Contains(body, "Bearer") || strings.Contains(body, "WWW-Authenticate") {
		t.Fatal("领票正文不应提前泄露 Bearer 写法")
	}
	jwtStr := tokenFrom(t, body, "JWT: ")
	if _, err := tm.Parse(jwtStr); err != nil {
		t.Fatalf("领票返回的 JWT 应能验签: %v", err)
	}
}

func TestAuthFailuresRevealBearerFormatWithoutSideEffects(t *testing.T) {
	ts, _, _ := newTestServer(t, nil)
	tok := getTicket(t, ts)

	// 缺少 Authorization：首次失败即给出完整写法。
	status, h, body := do(t, "GET", ts.URL+"/api/challenge", "", "", "")
	if status != 401 || h.Get("WWW-Authenticate") != "Bearer" || !strings.Contains(body, "Authorization: Bearer <") {
		t.Fatalf("缺少凭证应返回 401+WWW-Authenticate+完整写法，得到 %d\n%s", status, body)
	}

	// 裸 JWT。
	status, h, body = do(t, "GET", ts.URL+"/api/challenge", tok, "", "")
	if status != 401 || h.Get("WWW-Authenticate") != "Bearer" || !strings.Contains(body, "空格") {
		t.Fatalf("裸 JWT 应返回 401 并提示加 Bearer，得到 %d\n%s", status, body)
	}

	// 垃圾值。
	status, _, body = do(t, "GET", ts.URL+"/api/challenge", "Bearer ", "", "")
	if status != 401 {
		t.Fatalf("空凭证应 401，得到 %d\n%s", status, body)
	}

	// 正确 Bearer 格式直接通过，不要求先失败一次。
	status, _, body = do(t, "GET", ts.URL+"/api/challenge", "Bearer "+tok, "", "")
	if status != 200 || !strings.Contains(body, "鱼饼保险箱") {
		t.Fatalf("正确格式应直接通过，得到 %d\n%s", status, body)
	}
	if !strings.Contains(body, "阅读本说明不会启动计时") {
		t.Fatal("说明应包含计时提示")
	}

	// 上述鉴权失败不应启动计时或修改进度。
	status, _, body = do(t, "GET", ts.URL+"/api/challenge", "Bearer "+tok, "", "")
	if !strings.Contains(body, "当前没有进行中的挑战") {
		t.Fatalf("鉴权失败不应启动计时，正文:\n%s", body)
	}
}

func TestInvalidAndExpiredTokensRejected(t *testing.T) {
	ts, _, tm := newTestServer(t, nil)

	// 篡改签名。
	tok := getTicket(t, ts)
	status, _, body := do(t, "GET", ts.URL+"/api/challenge", "Bearer "+tok+"x", "", "")
	if status != 401 || !strings.Contains(body, "不要修改 JWT 内容") {
		t.Fatalf("篡改 JWT 应 401，得到 %d\n%s", status, body)
	}

	// 过期凭证。
	expired, _, _, err := tm.Issue(token.StageChallenge, "", -time.Minute, time.Now())
	if err != nil {
		t.Fatalf("签发测试凭证失败: %v", err)
	}
	status, _, body = do(t, "GET", ts.URL+"/api/challenge", "Bearer "+expired, "", "")
	if status != 401 || !strings.Contains(body, "已过期") {
		t.Fatalf("过期 JWT 应 401，得到 %d\n%s", status, body)
	}

	// 阶段不匹配：初始 JWT 不能领奖。
	status, _, body = do(t, "GET", ts.URL+"/api/reward", "Bearer "+tok, "", "")
	if status != 403 || !strings.Contains(body, "不能访问此阶段") {
		t.Fatalf("初始 JWT 领奖应 403，得到 %d\n%s", status, body)
	}
}

func TestSessionGoneAfterRestart(t *testing.T) {
	ts, _, _ := newTestServer(t, nil)
	tok := getTicket(t, ts)

	// 用同一密钥、全新 store 模拟服务重启。
	tm := token.New([]byte("test-secret-for-httptest-only"))
	cfg := challenge.Config{}
	cfg.IssuePass = func(sid string, now time.Time) (string, time.Time, error) {
		t, _, exp, err := tm.Issue(token.StageReward, sid, cfg.PassTTL, now)
		return t, exp, err
	}
	store := challenge.NewStore(cfg)
	ts2 := httptest.NewServer(NewRouter(Deps{Store: store, Tokens: tm}))
	defer ts2.Close()

	status, _, body := do(t, "GET", ts2.URL+"/api/challenge", "Bearer "+tok, "", "")
	if status != 401 || !strings.Contains(body, "会话已失效") {
		t.Fatalf("重启后旧凭证应提示重新领票，得到 %d\n%s", status, body)
	}
}

// ---- 格式校验（验收清单 6.2/6.3）----

func TestInvalidPostsDoNotStartTimer(t *testing.T) {
	ts, _, _ := newTestServer(t, nil)
	tok := getTicket(t, ts)
	auth := "Bearer " + tok

	// 媒体类型错误 → 415。
	status, _, body := do(t, "POST", ts.URL+"/api/challenge", auth, "text/plain", `{"seq":1}`)
	if status != 415 || !strings.Contains(body, "application/json") {
		t.Fatalf("错误媒体类型应 415，得到 %d\n%s", status, body)
	}

	// 各类 JSON 违规 → 400。
	for name, payload := range map[string]string{
		"字符串":  `{"seq":"1"}`,
		"布尔":   `{"seq":true}`,
		"null": `{"seq":null}`,
		"小数":   `{"seq":1.5}`,
		"指数":   `{"seq":1e1}`,
		"数组":   `[1]`,
		"缺字段":  `{}`,
		"未知字段": `{"seq":1,"x":2}`,
		"重复字段": `{"seq":1,"seq":2}`,
		"多值":   `{"seq":1}{"seq":2}`,
		"尾随":   `{"seq":1} x`,
		"越界0":  `{"seq":0}`,
		"越界11": `{"seq":11}`,
		"空体":   ``,
	} {
		status, body = postSeq(t, ts, auth, payload)
		if status != 400 {
			t.Fatalf("%s 应返回 400，得到 %d\n%s", name, status, body)
		}
	}

	// 无效请求不启动计时。
	_, _, body = do(t, "GET", ts.URL+"/api/challenge", auth, "", "")
	if !strings.Contains(body, "当前没有进行中的挑战") {
		t.Fatalf("格式校验失败不应启动计时:\n%s", body)
	}

	// 第一个有效提交才开启新一轮。
	status, body = postSeq(t, ts, auth, `{"seq":1}`)
	if status != 200 || !strings.Contains(body, "已开启新一轮挑战") {
		t.Fatalf("首个有效提交应开启新一轮，得到 %d\n%s", status, body)
	}
}

func TestAscDescRejectedWith409(t *testing.T) {
	for _, tc := range []struct {
		name string
		seqs []int
		kind string
	}{
		{"升序", []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, "升序"},
		{"降序", []int{10, 9, 8, 7, 6, 5, 4, 3, 2, 1}, "降序"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ts, _, _ := newTestServer(t, nil)
			tok := getTicket(t, ts)
			auth := "Bearer " + tok

			var status int
			var body string
			for _, seq := range tc.seqs {
				status, body = postSeq(t, ts, auth, fmt.Sprintf(`{"seq":%d}`, seq))
			}
			if status != 409 || !strings.Contains(body, "完整"+tc.kind) {
				t.Fatalf("完整%s应 409，得到 %d\n%s", tc.kind, status, body)
			}
			if strings.Contains(body, "Authorization: Bearer") {
				t.Fatal("被拒绝的轮次不应签发通关凭证")
			}

			// GET 可查询失败原因。
			_, _, body = do(t, "GET", ts.URL+"/api/challenge", auth, "", "")
			if !strings.Contains(body, "完整"+tc.kind) || !strings.Contains(body, "重新提交全部 10 个编号") {
				t.Fatalf("GET 应展示失败原因与重试提示:\n%s", body)
			}

			// 下一个有效提交开启新一轮，进度归 1。
			status, body = postSeq(t, ts, auth, `{"seq":5}`)
			if status != 200 || !strings.Contains(body, "已开启新一轮挑战") || !strings.Contains(body, "当前进度：1/10") {
				t.Fatalf("拒绝后的提交应开启新一轮，得到 %d\n%s", status, body)
			}
		})
	}
}

func TestDuplicatesDoNotAdvanceProgress(t *testing.T) {
	ts, _, _ := newTestServer(t, nil)
	tok := getTicket(t, ts)
	auth := "Bearer " + tok

	postSeq(t, ts, auth, `{"seq":3}`)
	status, body := postSeq(t, ts, auth, `{"seq":3}`)
	if status != 200 || !strings.Contains(body, "不重复计数") || !strings.Contains(body, "当前进度：1/10") {
		t.Fatalf("重复编号应提示且不计数，得到 %d\n%s", status, body)
	}
	if !strings.Contains(body, "计时不会延长") {
		t.Fatal("重复提交应说明不延长计时")
	}
}

func TestTimeoutFlow(t *testing.T) {
	ts, _, _ := newTestServer(t, func(c *challenge.Config) { c.Window = 30 * time.Millisecond })
	tok := getTicket(t, ts)
	auth := "Bearer " + tok

	postSeq(t, ts, auth, `{"seq":1}`)
	time.Sleep(60 * time.Millisecond)

	// GET 只提示超时，不开启新轮次。
	_, _, body := do(t, "GET", ts.URL+"/api/challenge", auth, "", "")
	if !strings.Contains(body, "上一轮已超时") {
		t.Fatalf("超时后 GET 应提示重置:\n%s", body)
	}

	// 下一个有效提交开启新一轮。
	status, body := postSeq(t, ts, auth, `{"seq":1}`)
	if status != 200 || !strings.Contains(body, "上一轮已超时，旧进度已清空") {
		t.Fatalf("超时后提交应提示新一轮，得到 %d\n%s", status, body)
	}
}

// ---- 完整通关流程 ----

func TestFullPassFlow(t *testing.T) {
	ts, _, _ := newTestServer(t, nil)
	tok := getTicket(t, ts)
	auth := "Bearer " + tok

	var status int
	var body string
	var hdr http.Header
	var lastHeader http.Header
	for _, seq := range []int{3, 1, 7, 2, 9, 4, 10, 6, 8, 5} {
		req, _ := http.NewRequest("POST", ts.URL+"/api/challenge", strings.NewReader(fmt.Sprintf(`{"seq":%d}`, seq)))
		req.Header.Set("Authorization", auth)
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("提交编号 %d 失败: %v", seq, err)
		}
		b, _ := io.ReadAll(res.Body)
		res.Body.Close()
		status, body, lastHeader = res.StatusCode, string(b), res.Header
	}
	if status != 200 || !strings.Contains(body, "鱼饼保险箱已打开") {
		t.Fatalf("乱序集齐应通关，得到 %d\n%s", status, body)
	}
	if lastHeader.Get("X-Next-Step") != "/api/reward" {
		t.Fatalf("通关响应应指向奖励入口，得到 %q", lastHeader.Get("X-Next-Step"))
	}
	if lastHeader.Get("Cache-Control") != "no-store" {
		t.Fatal("通关响应必须 no-store")
	}
	passTok := tokenFrom(t, body, "Authorization: Bearer ")

	// 再次 GET 可取回通关信息（错过通关响应的情形）。
	_, _, body = do(t, "GET", ts.URL+"/api/challenge", auth, "", "")
	if !strings.Contains(body, "Authorization: Bearer "+passTok) {
		t.Fatalf("GET 应取回通关凭证:\n%s", body)
	}

	// 通关 JWT 访问挑战接口：阶段不匹配 → 403。
	status, _, body = do(t, "GET", ts.URL+"/api/challenge", "Bearer "+passTok, "", "")
	if status != 403 {
		t.Fatalf("通关 JWT 访问挑战接口应 403，得到 %d\n%s", status, body)
	}

	// 领取奖励。
	status, hdr, body = do(t, "GET", ts.URL+"/api/reward", "Bearer "+passTok, "", "")
	if status != 200 || !strings.Contains(body, "🍥") || !strings.Contains(body, "d2pq5a2m6ZW/54ix6bG86aW855qE") {
		t.Fatalf("领奖失败，得到 %d\n%s", status, body)
	}
	if hdr.Get("Cache-Control") != "no-store" {
		t.Fatal("奖励响应必须 no-store")
	}
	if strings.Contains(body, "wjj学长爱鱼饼的") {
		// 密钥是 Base64 文本，明文不应出现在奖励页。
		t.Fatal("奖励页不应出现未编码的明文密钥")
	}
}

func TestConcurrentSubmissionsPassAndNoRace(t *testing.T) {
	ts, _, _ := newTestServer(t, nil)
	tok := getTicket(t, ts)
	auth := "Bearer " + tok

	var wg sync.WaitGroup
	for _, seq := range []int{3, 1, 7, 2, 9, 4, 10, 6, 8, 5} {
		wg.Add(1)
		go func(seq int) {
			defer wg.Done()
			status, _ := postSeq(t, ts, auth, fmt.Sprintf(`{"seq":%d}`, seq))
			if status == 500 {
				t.Errorf("编号 %d 服务端错误", seq)
			}
		}(seq)
	}
	wg.Wait()

	_, _, body := do(t, "GET", ts.URL+"/api/challenge", auth, "", "")
	if !strings.Contains(body, "鱼饼保险箱已打开") {
		t.Fatalf("并发集齐后应能取回通关信息:\n%s", body)
	}
}

// ---- 方法、路径与跨域（验收清单 6.3）----

func TestMethodAndRouteHandling(t *testing.T) {
	ts, _, _ := newTestServer(t, nil)

	// 未知路径 404，且带 CORS 头。
	status, h, body := do(t, "GET", ts.URL+"/nope", "", "", "")
	if status != 404 || !strings.Contains(body, "从首页") {
		t.Fatalf("未知路径应 404，得到 %d\n%s", status, body)
	}
	if h.Get("Access-Control-Allow-Origin") != "*" {
		t.Fatal("404 响应也应携带 CORS 头")
	}

	// 已知路径错误方法 405 + Allow。
	status, h, _ = do(t, "POST", ts.URL+"/", "", "", "")
	if status != 405 || h.Get("Allow") != "GET, OPTIONS" {
		t.Fatalf("POST / 应 405 并带 Allow，得到 %d %q", status, h.Get("Allow"))
	}
	status, h, _ = do(t, "DELETE", ts.URL+"/api/challenge", "", "", "")
	if status != 405 || h.Get("Allow") != "GET, POST, OPTIONS" {
		t.Fatalf("DELETE 挑战接口应 405，得到 %d %q", status, h.Get("Allow"))
	}

	// 预检：204、无鉴权、不创建会话。
	status, h, _ = do(t, "OPTIONS", ts.URL+"/api/challenge", "", "", "")
	if status != 204 {
		t.Fatalf("预检应 204，得到 %d", status)
	}
	if h.Get("Access-Control-Allow-Origin") != "*" ||
		h.Get("Access-Control-Allow-Headers") != "Authorization, Content-Type" ||
		h.Get("Access-Control-Expose-Headers") != "X-Next-Step" {
		t.Fatalf("预检 CORS 头不完整: %v", h)
	}

	// 无鉴权也能收到错误响应正文与 CORS 头（浏览器可读）。
	status, h, body = do(t, "GET", ts.URL+"/api/challenge", "", "", "")
	if status != 401 || h.Get("Access-Control-Allow-Origin") != "*" || !strings.Contains(body, "Authorization") {
		t.Fatalf("鉴权失败响应应可被浏览器读取: %d\n%s", status, body)
	}
}

func TestSessionsIndependentAcrossTickets(t *testing.T) {
	ts, _, _ := newTestServer(t, nil)
	a := getTicket(t, ts)
	b := getTicket(t, ts)

	postSeq(t, ts, "Bearer "+a, `{"seq":1}`)
	_, _, body := do(t, "GET", ts.URL+"/api/challenge", "Bearer "+b, "", "")
	if !strings.Contains(body, "当前没有进行中的挑战") {
		t.Fatalf("不同领票的进度不应共享:\n%s", body)
	}
	_, _, body = do(t, "GET", ts.URL+"/api/challenge", "Bearer "+a, "", "")
	if !strings.Contains(body, "当前进度：1/10") {
		t.Fatalf("会话 a 的进度应保留:\n%s", body)
	}
}
