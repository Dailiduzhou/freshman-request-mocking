// 鱼饼找回计划参考解法（仅供出题方与评审，见 project.md 5.3）。
//
// 从首页出发，沿响应头线索逐步发现接口，从领票正文中提取初始凭证，
// 共享一个 http.Client 并发提交十个编号，等待全部响应完成后再取回通关信息。
//
// 运行：
//
//	go run ./cmd/solver -base http://localhost:8080
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

type response struct {
	header http.Header
	body   string
}

func request(client *http.Client, method, endpoint, token string, payload []byte) (response, error) {
	req, err := http.NewRequest(method, endpoint, bytes.NewReader(payload))
	if err != nil {
		return response{}, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := client.Do(req)
	if err != nil {
		return response{}, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return response{}, err
	}
	result := response{header: res.Header.Clone(), body: string(body)}
	if res.StatusCode != http.StatusOK {
		return result, fmt.Errorf("%s %s: HTTP %d\n%s", method, endpoint, res.StatusCode, body)
	}
	return result, nil
}

func tokenFrom(body, prefix string) (string, error) {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, prefix) {
			token := strings.TrimSpace(strings.TrimPrefix(line, prefix))
			if token != "" {
				return token, nil
			}
		}
	}
	return "", fmt.Errorf("响应中未找到通行证：\n%s", body)
}

func nextURL(current string, header http.Header) (string, error) {
	path := header.Get("X-Next-Step")
	if path == "" {
		return "", fmt.Errorf("响应缺少 X-Next-Step")
	}
	base, err := url.Parse(current)
	if err != nil {
		return "", err
	}
	ref, err := url.Parse(path)
	if err != nil {
		return "", err
	}
	next := base.ResolveReference(ref)
	if next.Scheme != base.Scheme || next.Host != base.Host {
		return "", fmt.Errorf("拒绝向其他来源发送通行证：%s", next)
	}
	return next.String(), nil
}

func run(base string) error {
	client := &http.Client{
		Timeout: 10 * time.Second,
		// 本题不需要重定向，避免意外把凭证带到其他地址。
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	defer client.CloseIdleConnections()

	root := strings.TrimRight(base, "/") + "/"
	home, err := request(client, http.MethodGet, root, "", nil)
	if err != nil {
		return err
	}
	ticketURL, err := nextURL(root, home.header)
	if err != nil {
		return err
	}
	ticket, err := request(client, http.MethodGet, ticketURL, "", nil)
	if err != nil {
		return err
	}
	initialToken, err := tokenFrom(ticket.body, "JWT: ")
	if err != nil {
		return err
	}
	challengeURL, err := nextURL(ticketURL, ticket.header)
	if err != nil {
		return err
	}
	instructions, err := request(client, http.MethodGet, challengeURL, initialToken, nil)
	if err != nil {
		return err
	}
	fmt.Println(instructions.body)

	type result struct {
		seq int
		res response
		err error
	}
	results := make(chan result, 10)
	var wg sync.WaitGroup
	// 先按一个非升序、非降序排列启动任务，降低服务端收到完整升降序的概率。
	// 网络与调度仍可能改变接收顺序；若收到 409，应查看提示后重新尝试。
	for _, seq := range []int{3, 1, 7, 2, 9, 4, 10, 6, 8, 5} {
		wg.Add(1)
		go func(seq int) {
			defer wg.Done()
			payload, err := json.Marshal(struct {
				Seq int `json:"seq"`
			}{Seq: seq})
			if err != nil {
				results <- result{seq: seq, err: err}
				return
			}
			res, err := request(client, http.MethodPost, challengeURL, initialToken, payload)
			results <- result{seq: seq, res: res, err: err}
		}(seq)
	}
	wg.Wait()
	close(results)

	for item := range results {
		if item.err != nil {
			fmt.Fprintf(os.Stderr, "编号 %d 提交失败：%v\n", item.seq, item.err)
			continue
		}
		fmt.Printf("编号 %d 的响应：\n%s\n", item.seq, item.res.body)
	}

	// 已等待全部提交结束，用初始凭证取回成功信息，
	// 不依赖第 10 个启动的 goroutine 恰好收到通关响应。
	progress, err := request(client, http.MethodGet, challengeURL, initialToken, nil)
	if err != nil {
		return err
	}
	rewardToken, err := tokenFrom(progress.body, "Authorization: Bearer ")
	if err != nil {
		return fmt.Errorf("尚未通关，请检查响应；修正问题后可重新运行：%w", err)
	}
	rewardURL, err := nextURL(challengeURL, progress.header)
	if err != nil {
		return err
	}
	reward, err := request(client, http.MethodGet, rewardURL, rewardToken, nil)
	if err != nil {
		return err
	}
	fmt.Println(reward.body)
	return nil
}

func main() {
	base := flag.String("base", "http://localhost:8080", "题目网站地址")
	flag.Parse()
	if err := run(*base); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
