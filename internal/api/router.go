// Package api 组装 Gin 路由、中间件与各阶段文案，
// 是 project.md 第 3、4 节接口契约的直接实现。
package api

import (
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"requestmocking/internal/challenge"
	"requestmocking/internal/strictjson"
	"requestmocking/internal/token"
)

// Deps 注入会话存储与凭证管理器。
type Deps struct {
	Store  *challenge.Store
	Tokens *token.Manager
}

// knownPaths 供 405 处理器设置 Allow 头并给出提示。
var knownPaths = map[string]struct {
	allow string
	body  string
}{
	"/":              {"GET, OPTIONS", msgMethodGetOnly},
	"/api/ticket":    {"GET, OPTIONS", msgMethodGetOnly},
	"/api/challenge": {"GET, POST, OPTIONS", msgMethodChallenge},
	"/api/reward":    {"GET, OPTIONS", msgMethodGetOnly},
}

// NewRouter 构建题目站点。调用方负责优雅关闭。
func NewRouter(d Deps) *gin.Engine {
	r := gin.New()
	r.HandleMethodNotAllowed = true
	r.Use(gin.Logger(), gin.Recovery(), cors())

	r.NoMethod(methodNotAllowed)
	r.NoRoute(routeNotFound)

	// /api 组：所有含凭证、进度或奖励的响应一律 no-store。
	g := r.Group("/api")
	g.Use(noStore)

	r.GET("/", home)
	r.OPTIONS("/", preflight)
	g.GET("/ticket", ticket(d))
	g.OPTIONS("/ticket", preflight)
	g.GET("/challenge", authChallenge(d), challengeGet(d))
	g.POST("/challenge", authChallenge(d), challengePost(d))
	g.OPTIONS("/challenge", preflight)
	g.GET("/reward", authReward(d), reward)
	g.OPTIONS("/reward", preflight)

	return r
}

// ---- 中间件 ----

// cors 对所有响应（含错误与 404/405）添加跨域头；
// 不启用 Cookie 凭证模式。
func cors() gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.Writer.Header()
		h.Set("Access-Control-Allow-Origin", "*")
		h.Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		h.Set("Access-Control-Expose-Headers", "X-Next-Step")
		c.Next()
	}
}

func noStore(c *gin.Context) {
	c.Writer.Header().Set("Cache-Control", "no-store")
	c.Next()
}

// preflight 处理跨域预检：无需鉴权，不创建会话、不修改挑战状态。
func preflight(c *gin.Context) {
	c.AbortWithStatus(http.StatusNoContent)
}

func methodNotAllowed(c *gin.Context) {
	info, ok := knownPaths[c.Request.URL.Path]
	if !ok {
		routeNotFound(c)
		return
	}
	c.Header("Allow", info.allow)
	c.String(http.StatusMethodNotAllowed, info.body)
}

func routeNotFound(c *gin.Context) {
	c.String(http.StatusNotFound, msgNotFound)
}

// ---- 鉴权 ----

func unauthorized(c *gin.Context, msg string) {
	c.Header("WWW-Authenticate", "Bearer")
	c.String(http.StatusUnauthorized, msg)
	c.Abort()
}

func forbidden(c *gin.Context, msg string) {
	c.String(http.StatusForbidden, msg)
	c.Abort()
}

// requireToken 解析并验签 Authorization。
// 鉴权失败一律 401 + WWW-Authenticate: Bearer + 完整写法提示，
// 不启动挑战计时、不修改进度。
func requireToken(c *gin.Context, tm *token.Manager) *token.Claims {
	h := strings.TrimSpace(c.GetHeader("Authorization"))
	if h == "" {
		unauthorized(c, msgMissingAuth)
		return nil
	}
	var raw string
	if strings.HasPrefix(h, "Bearer ") {
		raw = strings.TrimSpace(h[len("Bearer "):])
		if raw == "" || strings.ContainsAny(raw, " \t") {
			unauthorized(c, msgNeedsBearer)
			return nil
		}
	} else if looksLikeJWT(h) {
		// 直接把裸 JWT 放进 Authorization。
		unauthorized(c, msgNeedsBearer)
		return nil
	} else {
		unauthorized(c, msgMissingAuth)
		return nil
	}
	claims, err := tm.Parse(raw)
	if err != nil {
		unauthorized(c, msgInvalidToken)
		return nil
	}
	return claims
}

func looksLikeJWT(s string) bool {
	if strings.ContainsAny(s, " \t") {
		return false
	}
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return false
	}
	for _, p := range parts {
		if p == "" {
			return false
		}
	}
	return true
}

func authChallenge(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims := requireToken(c, d.Tokens)
		if claims == nil {
			return
		}
		if claims.Stage != token.StageChallenge {
			forbidden(c, msgStageChallenge)
			return
		}
		sess, ok := d.Store.Get(claims.ID)
		if !ok {
			// 服务重启或会话被清理：验签能过，但内存状态不存在。
			unauthorized(c, msgSessionGone)
			return
		}
		c.Set(ctxSession, sess)
		c.Next()
	}
}

func authReward(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims := requireToken(c, d.Tokens)
		if claims == nil {
			return
		}
		if claims.Stage != token.StageReward {
			forbidden(c, msgStageReward)
			return
		}
		sess, ok := d.Store.Get(claims.SID)
		if !ok {
			unauthorized(c, msgSessionGone)
			return
		}
		if !sess.HasPassed() {
			forbidden(c, msgStageReward)
			return
		}
		c.Next()
	}
}

const ctxSession = "session"

func sessionFrom(c *gin.Context) *challenge.Session {
	return c.MustGet(ctxSession).(*challenge.Session)
}

// ---- 处理器 ----

func home(c *gin.Context) {
	c.Header("X-Next-Step", "/api/ticket")
	c.String(http.StatusOK, textHome)
}

func ticket(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		now := time.Now()
		tok, jti, exp, err := d.Tokens.Issue(token.StageChallenge, "", d.Store.SessionTTL(), now)
		if err != nil {
			c.String(http.StatusInternalServerError, msgInternal)
			return
		}
		d.Store.Create(jti, exp)
		c.Header("X-Next-Step", "/api/challenge")
		c.String(http.StatusOK, formatTicket(minutes(d.Store.SessionTTL()), tok))
	}
}

func challengeGet(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		sess := sessionFrom(c)
		st, err := sess.Status(time.Now(), d.Store.PassIssuer())
		if err != nil {
			c.String(http.StatusInternalServerError, msgInternal)
			return
		}
		if st.Kind == challenge.StatusPassed {
			writePass(c, d, st.PassToken)
			return
		}
		body := instructions(d)
		switch st.Kind {
		case challenge.StatusActive:
			body += fmtActive(st.Count, joinInts(st.Order))
		case challenge.StatusTimedOut:
			body += hintTimedOut
		case challenge.StatusRejected:
			kind := "升序"
			if !st.RejectAscending {
				kind = "降序"
			}
			body += fmtRejectedHint(kind, st.RejectOrder, seconds(d.Store.Window()))
		default: // StatusNotStarted
			body += fmtNotStarted(seconds(d.Store.Window()))
		}
		c.String(http.StatusOK, body)
	}
}

func challengePost(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		sess := sessionFrom(c)

		// 媒体类型按解析结果判断，允许 application/json; charset=utf-8。
		mt, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
		if err != nil || mt != "application/json" {
			c.String(http.StatusUnsupportedMediaType, msgUnsupportedMedia)
			return
		}

		body, err := io.ReadAll(io.LimitReader(c.Request.Body, strictjson.MaxBodyBytes+1))
		if err != nil {
			c.String(http.StatusBadRequest, msgBadBody)
			return
		}
		seq, err := strictjson.ParseSeq(body)
		if err != nil {
			c.String(http.StatusBadRequest, msgBadBody)
			return
		}

		out, err := sess.Submit(time.Now(), seq, d.Store.PassIssuer())
		if err != nil {
			c.String(http.StatusInternalServerError, msgInternal)
			return
		}
		switch out.Kind {
		case challenge.KindPass:
			writePass(c, d, out.PassToken)
		case challenge.KindRejected:
			kind := "升序"
			if !out.RejectAscending {
				kind = "降序"
			}
			c.String(http.StatusConflict,
				fmtRejected(kind, out.RejectOrder, seconds(d.Store.Window())))
		case challenge.KindNewRound:
			c.String(http.StatusOK, fmtNewRound(seconds(d.Store.Window()), out.Seq, out.Count))
		case challenge.KindProgress:
			c.String(http.StatusOK, fmtProgress(out.Seq, out.Count, joinInts(out.Order)))
		case challenge.KindDuplicate:
			c.String(http.StatusOK, fmtDuplicate(out.Seq, out.Count))
		case challenge.KindRoundAfterTimeout:
			c.String(http.StatusOK, fmtRoundAfterTimeout(seconds(d.Store.Window()), out.Seq, out.Count))
		case challenge.KindRoundAfterReject:
			kind := "升序"
			if !out.RejectAscending {
				kind = "降序"
			}
			c.String(http.StatusOK, fmtRoundAfterReject(kind, seconds(d.Store.Window()), out.Seq, out.Count))
		}
	}
}

func reward(c *gin.Context) {
	c.String(http.StatusOK, textReward)
}

// writePass 输出通关信息，供 POST 集齐与 GET 取回共用。
func writePass(c *gin.Context, d Deps, passToken string) {
	c.Header("X-Next-Step", "/api/reward")
	c.String(http.StatusOK, formatPass(passToken, minutes(d.Store.PassTTL())))
}

// ---- 文案格式化 ----

func instructions(d Deps) string {
	return fmtInstructions(seconds(d.Store.Window()))
}

func seconds(d time.Duration) int { return int(d / time.Second) }
func minutes(d time.Duration) int { return int(d / time.Minute) }

func joinInts(v []int) string {
	parts := make([]string, len(v))
	for i, n := range v {
		parts[i] = strconv.Itoa(n)
	}
	return strings.Join(parts, ", ")
}
