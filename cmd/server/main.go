// 题目站点服务入口。
//
// 用法：
//
//	go run ./cmd/server [-addr :8080]
//
// 环境变量 JWT_SECRET 可覆盖签名密钥；未设置时启动期随机生成，
// 服务重启后所有会话失效（符合单实例内存态的设计边界）。
package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"requestmocking/internal/api"
	"requestmocking/internal/challenge"
	"requestmocking/internal/token"
)

func main() {
	addr := flag.String("addr", ":8080", "监听地址")
	flag.Parse()

	// 保留 access 日志（不记录请求头与响应体，符合 4.4 要求），关闭调试告警。
	gin.SetMode(gin.ReleaseMode)

	tm, err := token.NewFromEnv()
	if err != nil {
		log.Fatalf("初始化签名密钥失败: %v", err)
	}

	store := challenge.NewStore(challenge.Config{
		IssuePass: func(sid string, now time.Time) (string, time.Time, error) {
			tok, _, exp, err := tm.Issue(token.StageReward, sid, challenge.DefaultPassTTL, now)
			return tok, exp, err
		},
	})
	stopJanitor := store.StartJanitor()
	defer stopJanitor()

	srv := &http.Server{
		Addr:              *addr,
		Handler:           api.NewRouter(api.Deps{Store: store, Tokens: tm}),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		log.Printf("鱼饼找回计划已启动，监听 %s", *addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("服务异常退出: %v", err)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()

	log.Println("正在优雅关闭……")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("关闭超时: %v", err)
	}
}
