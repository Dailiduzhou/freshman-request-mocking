# 鱼饼找回计划（出题方维护手册）

面向新生的 HTTP 基础分阶段挑战站点：从响应头发现线索 → 领取并携带 JWT →
POST JSON 提交编号 → 并发集齐 → 换取通关凭证领回 🍥 与密钥。

> 完整题面、接口契约、判题规则与验收清单见 [`project.md`](project.md)，
> 该文档**仅供出题方与评审**，不要部署到题目站点。

## 构建

```bash
go build ./...        # 编译
go test -race ./...   # 单元 + 端到端测试（含并发安全检查）
```

## 本地运行

```bash
go run ./cmd/server -addr :8080
```

- `-addr`：监听地址，默认 `:8080`。
- `JWT_SECRET` 环境变量：签名密钥。未设置时启动期随机生成——
  服务重启后所有会话失效并提示重新领票（符合单实例内存态设计）。

## Docker

```bash
docker build -t fish-cake .
docker run --rm -p 8080:8080 fish-cake
# 生产部署建议置于 HTTPS 反向代理之后
```

镜像为多阶段构建（`golang:1.27.2-alpine3.24` → `scratch`），静态二进制、无 shell。

## 验收

参考解法（`project.md` 5.3 的落地，跑通即完成 6.4 首项验收）：

```bash
go run ./cmd/solver -base http://localhost:8080
```

手动复现探索路径（裸 JWT → 401 提示 Bearer、完整升序 → 409 等）见 `project.md` 5.2。

## 代码结构

```
cmd/server          服务入口（flag 装配、优雅关闭）
cmd/solver          评审用参考解法，不出现在题目站点
internal/token      JWT 签发与校验（固定 HS256，jti/stage/sid）
internal/strictjson 严格 JSON 请求体校验（拒绝重复字段、尾随内容等）
internal/challenge  会话判题状态机（锁内判题、惰性超时、后台清理）
internal/api        Gin 路由、鉴权/CORS 中间件与全部交互文案
```

## 边界提醒

- 单实例部署；挑战进度保存在内存中，不支持跨实例共享。
- 签名密钥仅保留在服务端，不出现在任何响应、日志或公开资源中。
- `project.md` 第 2 节为新生可见题面，第 3 节为站点交互文案来源，
  第 5 节评审解析与 `cmd/solver` 均不得暴露给新生。