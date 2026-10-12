# 鱼饼找回计划

## 1. 项目介绍与使用范围

这是一个检测新生网络请求基础掌握情况的分阶段题目网站。新生需要发现响应头中的线索、领取并携带 JWT、通过 POST JSON 提交序号，最终找回鱼饼与密钥。

- **面向新生**：第 2 节的题面，以及第 3 节中对应当前阶段的交互文案。网站应在探索过程中主动给出操作提示。
- **仅供出题方和评审**：完整接口流程、判题规则、参考解法、验收用例及设计局限。不要把本文整体公开到挑战网站，也不在通关后自动展示解析。

### 1.1 考查目标

1. 区分页面正文、HTTP 请求头和响应头，使用开发者工具发现线索。
2. 使用 `Authorization: Bearer <JWT>` 携带凭证，并在多个请求中复用它。
3. 使用正确的 HTTP 方法、`Content-Type` 和 JSON 请求体。
4. 理解并发请求的组织方式，能使用 Go goroutine 发起请求并等待完成。
5. 检查 HTTP 状态码、读取响应正文，根据反馈定位问题。

本题不是安全攻防题，不要求扫描接口、破解 JWT 签名、猜测服务端密钥或制造大流量。服务端不识别客户端语言，也不能证明客户端是否使用了 goroutine。

### 1.2 技术栈与部署边界

- Go、Gin、JWT。
- 允许所有来源跨域访问，使用显式 Authorization 请求头，不依赖 Cookie。
- 单实例部署，挑战进度保存在内存中，所有状态更新必须并发安全。
- 服务重启后挑战会话失效，提示重新领票；多实例共享状态不在本题范围内。
- 正式部署使用 HTTPS；本地练习可使用 HTTP。

## 2. 新生可见题面

### 鱼饼找回计划

wjj 学长在玩毛线球的时候，不小心打翻了自己的服务器源码。他最中意的 🍥 和一段密钥，遗落到了这个网站的某个神秘角落。

请从网站首页出发，沿着线索找到它们。**当网站向你展示 🍥 和密钥时，就表示挑战成功。**

这趟旅程中，你可能需要查看网页背后的网络请求、带上一张通行证，再用程序快速送达一组编号。

提示：

1. 浏览器展示的正文，并不是一次 HTTP 响应的全部。
2. 拿到通行证后，留意网站对 `Authorization` 请求头的提示。
3. Go 的 `go` 关键字可以让函数并发执行；别忘了等待任务完成并读取响应。
4. 只需要少量请求。遇到错误先读返回的提示，不必扫描接口或持续高频重试。

```go
go func(// 你想要的参数){
  // ....函数逻辑
} (// 传入的实参) // 有这个括号才能运行
```

## 3. 新生探索流程与交互文案

本节给出各阶段实际展示的内容。尖括号占位符由服务端替换，不是字面返回值。除预检响应外，正文统一使用 `text/plain; charset=utf-8`，包括最终奖励页面，避免“纯文本 HTML”的歧义。

### 3.1 Phase 1：从响应头发现入口

请求：`GET /`

状态码：`200 OK`

响应头：

```http
X-Next-Step: /api/ticket
```

正文：

```text
这里好像什么都没有……
但你目前能看到的并不是真相。

提示：一次网络响应不只有正文，看看它的“头”部。
浏览器开发者工具的 Network（网络）面板，也许能帮到你。
```

新生应打开开发者工具、刷新页面、选择根路径请求，在 Response Headers 中发现 `X-Next-Step`。也可以用 `curl -i` 查看完整响应。

### 3.2 Phase 2：领取通行证

请求：`GET /api/ticket`，不要求鉴权。

状态码：`200 OK`

响应头：

```http
X-Next-Step: /api/challenge
Cache-Control: no-store
```

正文：

```text
你找到了一张通行证，有效期为 30 分钟。

JWT: <初始 JWT>

下一站：/api/challenge
先使用 GET 尝试访问，再沿着响应中的线索继续探索。

提示：这张通行证似乎应该放进 Authorization 请求头。
如果网站没有放行，记得查看响应头和正文中的提示。
每次领票都会生成独立会话。请只领取一次，并在本轮挑战中复用同一张通行证。
```

**提示释放规则（仅供出题方）：** 首页和领票响应均不提前展示 `Bearer` 或完整的 Authorization 写法，也不在成功领票响应中添加 `WWW-Authenticate`。先让新生尝试如何携带 JWT；当其访问受保护接口但缺失 Authorization、直接放入裸 JWT 或使用错误的认证格式时，返回 `401`，再给出明确提示，例如：

```http
WWW-Authenticate: Bearer
```

```text
通行证需要使用 Bearer 认证格式，请设置请求头：
Authorization: Bearer <你的通行证>

Bearer 和通行证之间有一个空格。
请使用刚才领取的 JWT，不要把它放进 URL 或 JSON 请求体。
如果还没有通行证，请先访问 /api/ticket。
```

提示由当前请求的错误触发，不要求累计失败次数，也不记录“是否看过提示”。已经知道正确写法的新生可以直接通过，不必故意请求失败。鉴权失败不启动挑战计时、不修改进度；鉴权通过后，挑战说明和通关响应可以明确使用 Bearer 写法。

JWT 是服务端签发的凭证，不是需要破解的密文。其 payload 可解码查看，但修改内容会导致验签失败。

### 3.3 Phase 3：阅读挑战说明

请求：

```http
GET /api/challenge
Authorization: Bearer <初始 JWT>
```

未通关时返回 `200 OK`，正文：

```text
你已经找到鱼饼保险箱，但还需要凑齐 10 个编号才能打开。

请向当前地址 /api/challenge 发送 POST 请求：
- 继续携带 Authorization: Bearer <你的通行证>
- 设置 Content-Type: application/json
- 请求体示例：{"seq": 1}

从第一个有效编号被收到开始，你有 5 秒时间提交 1～10。
每个请求只提交一个编号，编号不能重复计数。
阅读本说明不会启动计时。

提示：
- 所有请求必须复用同一张通行证，不要每次重新领票。
- 可以使用 go 关键字并发发送这 10 个请求。
- 主程序需要等待请求完成，并读取每个请求的响应。
- 集齐编号且顺序符合要求时，那次响应会给出新的通行证和奖励入口。
- 可以先打乱编号再并发发送；并发并不保证服务端收到的顺序一定符合要求。
- 超时或因完整升序、完整降序被拒绝后，下一次有效提交会自动开始新一轮。
- 如果错过通关响应，再用原通行证 GET 本接口即可取回通关信息。
```

未通过时再次 GET，还应附上进度提示：未开始、当前已收集的数量与首次接收顺序、“上一轮已超时，下一次有效提交将重新开始”，或上一轮因完整升序、完整降序被拒绝的原因及重试提示。GET 不增加进度、不启动或延长窗口。

### 3.4 Phase 3：提交编号

请求示例：

```http
POST /api/challenge
Authorization: Bearer <初始 JWT>
Content-Type: application/json

{"seq": 1}
```

除“顺序不符合要求”返回 `409 Conflict` 外，以下响应均为 `200 OK`，业务状态通过正文说明：

**首次有效提交：**

```text
已开启新一轮挑战，计时 5 秒。
已收到编号 1，当前进度：1/10。
请在时间窗口内继续提交其余编号。
```

**后续有效提交：**

```text
已收到编号 4，当前进度：2/10。
首次接收顺序：1, 4。
```

**重复编号：**

```text
编号 4 已收到，本次不重复计数。
当前进度：2/10。计时不会延长。
```

**窗口到期后的下一个有效提交：**

```text
上一轮已超时，旧进度已清空。
已开启新一轮挑战，计时 5 秒。
已收到编号 7，当前进度：1/10。
建议准备好全部编号后并发发送，并等待所有响应完成。
```

**集齐但顺序不符合要求：**

触发条件：首次接收顺序为 `1,2,3,4,5,6,7,8,9,10` 或 `10,9,8,7,6,5,4,3,2,1`。返回 `409 Conflict`，不签发通关 JWT，也不返回奖励入口。

```text
10 个编号已集齐，但本轮未通过：接收顺序为完整升序。
本轮首次接收顺序：1, 2, 3, 4, 5, 6, 7, 8, 9, 10。

完整升序和完整降序都会被拒绝。
提示：可以先打乱 1～10，再使用 go 关键字并发发送。
并发不保证接收顺序一定符合要求；请检查每个响应，必要时重新尝试。

本轮进度已清空。请继续使用同一张通行证，重新提交全部 10 个编号。
下一次有效提交会启动新的 5 秒计时，阅读说明不会启动计时。
```

若为降序，提示相应替换为“完整降序”及实际序列。保留最近一次失败原因供 GET 查询，开始新一轮后清除；拒绝本轮的第十个编号不自动计入下一轮。

**成功集齐且顺序符合要求：**

响应头：

```http
X-Next-Step: /api/reward
```

正文：

```text
10 个编号已集齐，且接收顺序既不是完整升序，也不是完整降序。鱼饼保险箱已打开！

Authorization: Bearer <通关 JWT>

奖励入口：/api/reward
请使用 GET 访问，并把 Authorization 请求头替换为上面的新通行证。
新通行证有效期为 10 分钟，旧通行证不能领取奖励。
```

通过状态保持到初始会话过期。之后使用仍有效的初始 JWT 再次 GET 挑战接口，或进行有效 POST 提交，均返回通关信息，不重新计数。可复用尚未过期的通关 JWT；若它已过期但初始会话仍有效，可重新签发。

### 3.5 Phase 4：领取奖励

请求：

```http
GET /api/reward
Authorization: Bearer <通关 JWT>
```

状态码：`200 OK`

正文：

```text
🍥

恭喜你，找回了 wjj 学长的鱼饼！挑战成功。

密钥：d2pq5a2m6ZW/54ix6bG86aW855qE

这段密钥采用了 Base64 编码，有兴趣可以解码看看。
```

🍥是通关标志。密钥为字符串 `wjj学长爱鱼饼的` 的 UTF-8 字节经标准 Base64 编码后的结果，不包含引号、前后空格或换行。得到奖励即通关，不再设置额外的解码提交关卡。

该密钥不是 JWT 签名密钥。签名密钥必须仅保留在服务端，不能出现在任何响应、前端资源或公开题面中。

## 4. 出题方接口与判题规范

### 4.1 接口总表

| 方法    | 路径             | 鉴权     | 功能                                     |
| ------- | ---------------- | -------- | ---------------------------------------- |
| GET     | `/`              | 无       | 展示题目入口，在响应头提供线索           |
| GET     | `/api/ticket`    | 无       | 创建独立会话，签发初始 JWT               |
| GET     | `/api/challenge` | 初始 JWT | 查看说明、进度或取回通关凭证             |
| POST    | `/api/challenge` | 初始 JWT | 使用 JSON 提交一个编号                   |
| GET     | `/api/reward`    | 通关 JWT | 返回🍥和 Base64 密钥                     |
| OPTIONS | 上述路径         | 无       | 处理跨域预检，不创建会话、不修改挑战状态 |

对于已知路径，不支持的方法返回 `405 Method Not Allowed`，设置 `Allow` 并提示正确方法；不存在的路径返回 `404`。已知路径先处理 OPTIONS 和方法限制，再对受保护操作鉴权。

### 4.2 请求校验与错误反馈

受保护操作先校验凭证，再校验 POST 的媒体类型和 JSON，最后处理状态。鉴权或格式校验失败的请求不计数、不启动计时、不清空进度；时间本身仍正常流逝。集齐后触发的顺序拒绝属于整轮判题失败，按下文规则清空本轮进度，不属于格式校验失败。

| 场景                                    | 状态码 | 建议提示                                                                       |
| --------------------------------------- | ------ | ------------------------------------------------------------------------------ |
| 缺少 Authorization                      | 401    | 请先访问 `/api/ticket` 领票，并设置 `Authorization: Bearer <通行证>`           |
| 直接把 JWT 放在 Authorization 中        | 401    | 通行证格式不正确，请在前面加上 `Bearer `，注意中间的空格                       |
| JWT 签名无效、结构错误或过期            | 401    | 通行证无效或已过期，请重新领票；不要修改 JWT 内容                              |
| JWT 有效但 stage 不符合接口要求         | 403    | 当前通行证不能访问此阶段；领取奖励需要挑战通过后得到的新通行证                 |
| 服务重启导致会话不存在                  | 401    | 本轮会话已失效，请重新访问 `/api/ticket`                                       |
| POST 媒体类型不是 application/json      | 415    | 请设置 `Content-Type: application/json`                                        |
| JSON 无效、缺少 seq、seq 类型或范围不符 | 400    | 请求体应为 `{"seq": 1}`，seq 必须为 1～10 的 JSON 整数                         |
| 不支持的方法                            | 405    | 挑战说明使用 GET，提交编号使用 POST JSON；其他路径提示各自允许的方法           |
| 集齐后首次接收顺序为完整升序或完整降序  | 409    | 明确说明被拒绝的顺序，提示打乱编号、并发发送，并说明下一次有效提交会开启新一轮 |

`Content-Type` 应按媒体类型解析，允许 `application/json; charset=utf-8`。`401` 响应同时带有 `WWW-Authenticate: Bearer`。

POST 请求体严格为仅含一个 `seq` 字段的 JSON 对象：拒绝字符串、布尔值、null、小数、指数形式、数组、重复字段、未知字段、多个连续 JSON 值及尾随非空白内容。不接受 URL 参数或旧的 `X-Seq` 请求头替代 JSON。不要仅依赖默认反序列化行为判断重复字段。

所有错误响应都使用可读中文提示，但不回显签名密钥、内部错误栈或完整的用户凭证。

### 4.3 计时与状态机

每个初始 JWT 的 `jti` 对应一个独立挑战会话。最小状态包括：

- 会话过期时间。
- 当前轮次开始时间、截止时间。
- 已收到序号的集合，可用 10 位 bitset 表示。
- 不重复编号的首次接收序列，按会话锁内处理顺序追加；不能从排序后的集合还原。
- 最近一次因完整升序或完整降序被拒绝的原因与序列，供重试前 GET 查询。
- 是否已通过，以及可复用的通关凭证及其过期时间。

状态转换：

```text
未开始 --第一个有效 POST--> 进行中
进行中 --有效新编号且尚未集齐--> 进行中
进行中 --截止前集齐且非完整升序、非完整降序--> 已通过
进行中 --截止前集齐但为完整升序或完整降序--> 本轮被拒绝（409，清空进度）
本轮被拒绝 --下一个有效 POST--> 新一轮进行中（该编号计为第一个）
进行中 --到达截止时间--> 本轮超时
本轮超时 --下一个有效 POST--> 新一轮进行中（该编号计为第一个）
已通过 --GET 或有效 POST--> 返回通关信息，不重开挑战
任意状态 --会话过期或服务重启--> 需要重新领票
```

细则：

1. 时间窗口为 **5 秒**，从第一个有效提交开始。采用半开区间 `[start, start + 5s)`；恰好到达截止时间视为超时。
2. 判题时间取请求完成校验、进入会话锁后的服务端单调时钟读数，以原子状态处理时刻为准，不使用客户端时间或含糊的 TCP 到达时间。
3. 同一会话的“检查是否通过、检查窗口、必要时重置、序号去重、更新集合和接收序列、集齐后判断顺序、标记通过或拒绝并重置”必须在同一临界区完成。不要对全站请求持有一个长时间全局锁。
4. 不要求恰好发送十次请求，但必须在一个窗口内集齐十个不同的合法序号，且首次接收序列既非完整升序、也非完整降序。重复提交不计数、不追加到序列，也不延长截止时间，不能靠插入重复编号破坏升降序判定。
5. 超时后的下一个有效提交启动新轮次，即使这个编号曾在旧轮次出现，也计为新轮次的第一个编号。
6. 超时可以惰性判断，不必精确在截止瞬间启动清理任务。GET 可以展示超时状态，但不启动新轮次。
7. 仅在集齐十个不同编号时判断完整序列：恰好为 `[1,2,3,4,5,6,7,8,9,10]` 或 `[10,9,8,7,6,5,4,3,2,1]` 时，返回 409 并清空本轮集合、序列和计时，保留失败提示供 GET 查询。其他排列可通过，不要求每两个相邻编号都反转方向。不能因前两个编号递增或递减而提前拒绝。判定使用会话锁内的首次处理顺序，不使用客户端循环顺序、响应完成顺序或日志打印顺序。
8. 以服务端受理时刻判定，响应在窗口结束后到达客户端不影响已经完成的判题。
9. 通过后不再重置进度。初始会话过期后不能再取回通关凭证；此前签发的通关 JWT 在自身有效期内仍可用于领奖。
10. 会话和奖励关联状态须保留到所有已签发凭证的有效期结束，之后清理，避免已通过会话泄漏内存或过早失效。

### 4.4 JWT 与奖励权限

- 服务端固定允许的签名算法，例如 HS256；使用独立、高强度的签名密钥，不接受 `alg: none` 或由客户端自由选择算法。
- 初始 JWT 包含 `jti`、`stage: challenge`、`iat`、`exp`，有效期 30 分钟；`jti` 随机生成，不同领票操作不共享进度。
- 通关 JWT 使用新的唯一 `jti`，包含 `sid` 指向初始 JWT 的 `jti`、`stage: reward`、`iat`、`exp`，有效期 10 分钟。
- 初始 JWT 的 `jti` 是挑战状态键；通关 JWT 的 `sid` 用于关联已通过的会话。
- 奖励接口校验签名、有效期、阶段及关联会话的通过状态，不能只判断“这是一个能验签的 JWT”。
- 所有含凭证、进度或奖励的响应设置 `Cache-Control: no-store`，避免共享缓存泄露或复用会话。
- 不在访问日志中记录完整 Authorization 或含 token 的响应正文。
- 本题的 Base64 密钥是固定展示内容，不承担身份验证或安全保密功能；实际领取奖励仍需通过权限校验。

### 4.5 跨域规则

```http
Access-Control-Allow-Origin: *
Access-Control-Allow-Methods: GET, POST, OPTIONS
Access-Control-Allow-Headers: Authorization, Content-Type
Access-Control-Expose-Headers: X-Next-Step
```

- 不启用 Cookie 凭证模式，不设置 `Access-Control-Allow-Credentials: true`。
- Authorization 和 JSON POST 会触发浏览器预检；OPTIONS 返回 `204 No Content`，不要求 JWT，也不计入挑战。
- 实际响应和错误响应都添加适当的 CORS 头，避免新生只能看到浏览器拦截错误而看不到服务端提示。
- `Expose-Headers` 让浏览器脚本能够读取自定义响应头；开发者工具直接查看响应头不依赖它。

## 5. 评审专用解析

本节不面向新生展示，用于评审题目是否闭环、能否实现、判题是否公平。

### 5.1 解题路径与对应考点

| 阶段     | 预期操作                                                                          | 考查内容                                     |
| -------- | --------------------------------------------------------------------------------- | -------------------------------------------- |
| 发现入口 | 查看根请求的 `X-Next-Step`                                                        | HTTP 响应头、开发者工具、正文不等于完整响应  |
| 领取凭证 | GET 领票接口并保存 JWT                                                            | 响应读取、会话意识                           |
| 携带凭证 | 先探索 Authorization 用法，再根据 401 提示修正为 `Bearer ...`，或直接使用正确格式 | 请求头、错误响应中的线索、认证格式、凭证复用 |
| 阅读说明 | GET 挑战接口                                                                      | 先理解接口契约，再编写代码                   |
| 提交序号 | POST JSON，提交 1～10                                                             | HTTP 方法、媒体类型、请求体序列化            |
| 并发组织 | goroutine、等待所有任务、读取结果                                                 | 并发与程序生命周期、响应处理                 |
| 领取奖励 | 换用新 JWT 请求奖励接口                                                           | 阶段授权、凭证更新                           |

### 5.2 手动探索示例

以下命令仅用于评审复现；实际学生应先发现入口，而不是被提前告知全部路径。

```bash
BASE='http://localhost:8080'

# -i 同时显示响应头和正文，找到 X-Next-Step。
curl -i "$BASE/"

# 根据线索领票。不要在后续每次请求前重新领票。
curl -i "$BASE/api/ticket"

# 手动复制本次领票结果中的初始 JWT。
TOKEN='<初始 JWT>'

# 探索：先直接携带 JWT，预期得到 401，并从响应中发现 Bearer 提示。
# 这次鉴权失败不会启动挑战计时，也不需要重新领票。
curl -i -H "Authorization: $TOKEN" "$BASE/api/challenge"

# 根据提示修正请求头，再阅读挑战说明。
curl -i -H "Authorization: Bearer $TOKEN" "$BASE/api/challenge"

# 演示单个 JSON 提交；此操作会启动本轮计时。
curl -i -X POST "$BASE/api/challenge" \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  --data '{"seq":1}'
```

单个请求用于验证接口格式，不构成完整解法。等待窗口结束后，下一个有效提交自动开始新一轮。下面的 Go 示例从领票开始建立自己的独立会话。

### 5.3 Go 并发参考解法

该示例仅依赖标准库。它从响应头逐步发现接口，从领票正文的 `JWT: ...` 中提取初始凭证，从通关正文的 `Authorization: Bearer ...` 中提取通关凭证，共享一个 `http.Client` 并发提交十个编号，并等待所有响应完成。服务端响应须遵循第 3 节约定。

这是供评审使用的最终正确解法，已知 Bearer 写法，因此直接使用它，不刻意制造一次鉴权失败；新生探索错误格式并获得提示的过程由上一节 curl 示例演示。

保存代码为独立的 `main.go`，在该目录运行：

```bash
go run main.go -base http://localhost:8080
```

```go
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
```

参考代码的关键点：

- `wg.Add(1)` 在启动 goroutine 前执行，`defer wg.Done()` 保证每个任务结束时归还计数。
- 通过参数传入 `seq`，清楚表达每个任务持有自己的编号；每个请求都有独立请求体。
- `http.Client` 可以并发复用，不能让多个 goroutine 共享并修改同一个请求对象或请求体读取器。
- 结果通道容量为 10，与任务数相同，因此先等待再读取不会因通道写满而死锁；若改成无缓冲通道，必须同时调整收集逻辑。
- 单次请求超时 10 秒是客户端防止永久等待的措施，不代表服务端窗口被延长到 10 秒。
- HTTP 请求没有返回 Go error，不等于业务成功；还要检查 HTTP 状态码并读取正文。
- 每个响应体都要关闭；等待的是全部任务，不是固定睡眠一段时间。
- 成功响应可能来自任意编号。最后通过 GET 取回凭证，可避免依赖某个请求的完成顺序。
- 启动任务前使用非升序、非降序的编号排列，但启动顺序仍不等于服务端接收顺序，因此不能保证绝不触发顺序拒绝。
- 示例不自动无限重试，不产生持续请求压力。遇到网络异常或 409 时先查看输出，修正后重跑；重跑会领取新会话。服务端也支持沿用原会话重新提交完整一轮。

### 5.4 密钥解析

生成方式：

```go
base64.StdEncoding.EncodeToString([]byte("wjj学长爱鱼饼的"))
```

结果：

```text
d2pq5a2m6ZW/54ix6bG86aW855qE
```

等价命令：

```bash
printf %s 'wjj学长爱鱼饼的' | base64
printf %s 'd2pq5a2m6ZW/54ix6bG86aW855qE' | base64 --decode
```

不能使用默认会附加换行的 `echo` 生成源字节。本题采用标准 Base64，不是 JWT 各段使用的 Base64URL；此结果不带 `=` 是长度恰好不需要补位，不是主动删除 padding。

Base64 是可逆编码，不提供保密性。JWT 的 payload 同样不因编码而保密；JWT 的可信性来自签名验证，而不是“看起来像乱码”。

### 5.5 判题局限与公平性

1. **拒绝完整升序和完整降序是题目规则，不是并发证明。** 串行客户端可以故意按其他排列发送，而真实并发请求也可能恰好按升序或降序被服务端接收。保留这一硬性条件时，必须明确告知失败原因并允许重试，不能提示“你没有使用并发”。
2. **5 秒集齐十个编号且顺序合规，仍不等于强制并发。** 网络较快时串行乱序也能通过。线上结果只验证协议操作、时间窗口收集和接收序列；若评审需要验证 goroutine 的使用，应另行检查学生源码。
3. **不按 IP 共享进度。** 校园网内多人可能共用出口，按独立会话隔离避免串题。
4. **渐进式揭示认证格式，不让新生无依据地猜测。** 领票时只提示 Authorization，暂不告知 Bearer；首次因缺少凭证或认证格式错误而被拒绝时，就给出完整写法。无需反复失败才能解锁提示。HTTP 方法、JSON 字段及后续规则仍明确说明。
5. **窗口需在实际部署环境验收。** 5 秒是当前规则，不是适用于所有网络条件的定理。上线前用目标环境验证；若需修改，应同步修改题面、服务端和验收用例，不能暗中改变判题标准。
6. **这是 HTTP 入门练习，不是强防作弊竞赛。** 固定密钥可以被转发，不能单凭学生报出密钥证明独立完成；如需成绩记录，应另行设计身份绑定或提交源码机制，不在本次范围内。

## 6. 实现验收清单

本清单是后续服务端的验收标准，不表示当前已有实现或已通过测试。

### 6.1 流程与提示

- [ ] 首页正文含适度提示，响应头能发现领票入口。
- [ ] 领票返回合法初始 JWT、Authorization 线索和下一步地址，并提示复用同一张票；首页及领票响应的正文与响应头都不提前泄露 Bearer 写法。
- [ ] 缺少 Authorization、直接携带裸 JWT 或认证格式错误时，首次失败就返回 401、`WWW-Authenticate: Bearer` 和完整写法提示，不启动计时或修改进度。
- [ ] 直接使用正确 Bearer 格式可以通过，不要求先失败一次；鉴权后的说明可明确展示该写法。
- [ ] GET 挑战接口可阅读说明和进度，不启动或延长计时。
- [ ] POST 使用 JSON `{"seq":1}`，不再依赖 `X-Seq`。
- [ ] 成功响应包含新 JWT 和奖励入口；再次 GET 可取回通关信息。
- [ ] 奖励仅展示通关结果与密钥，不公开评审解析。

### 6.2 判题与并发

- [ ] 完整升序 `1→10` 和完整降序 `10→1` 在窗口内集齐时都返回 409，正文指出具体原因和重试方式，不签发通关 JWT。
- [ ] 非升序、非降序排列（例如 `1,2,3,4,5,6,7,8,10,9`）在窗口内集齐可通过，不因局部递增或递减提前拒绝。
- [ ] 在完整升序或降序中插入重复编号，仍会被拒绝；重复请求不能改变首次接收序列。
- [ ] 顺序拒绝后进度与计时清空，GET 可查看失败提示；下一个有效提交开启新一轮，拒绝时的最后一个编号不自动计入新轮次。
- [ ] 重复编号不增加进度，不延长截止时间；多于十次请求不直接判失败。
- [ ] 无效请求不启动计时、不计数、不清空有效进度。
- [ ] 截止前收到最后一个新编号且序列合规时可通过；恰好截止或之后的有效提交启动新轮次。
- [ ] 超时后的第一个有效编号计入新轮次，正文明确提示重置。
- [ ] 并发重复提交不会多计数，并发集齐时不会丢失更新或出现竞争条件。
- [ ] 不同初始 JWT 的进度独立，即使来源 IP 相同。
- [ ] 成功后重复访问不会重开挑战。
- [ ] 服务重启明确提示重新领票；过期状态最终可清理。

### 6.3 鉴权、格式与跨域

- [ ] 缺失 Bearer、错误签名和过期 JWT 返回 401；阶段错误返回 403。
- [ ] 初始 JWT 无法领取奖励，篡改 stage 或 sid 无法绕过校验。
- [ ] 初始会话过期不使尚在有效期内的通关 JWT 提前失效；服务重启则按内存状态丢失处理。
- [ ] 不支持的方法返回 405 与 Allow；错误媒体类型返回 415。
- [ ] 缺失字段、错误类型、范围越界、重复字段和多余 JSON 值均返回 400。
- [ ] OPTIONS 无需鉴权，返回 204，不改变挑战状态。
- [ ] 浏览器跨域可读取 `X-Next-Step`，可发送带 Authorization 的 JSON POST，也能读取错误提示。
- [ ] 凭证和奖励响应不被缓存，JWT 签名密钥不出现在响应或公开资源中。

### 6.4 参考解法与奖励

- [ ] 参考 Go 程序在真实服务上能完成全流程，并正确等待所有请求。
- [ ] 通过 `go test -race` 等手段检查服务端并发状态访问。
- [ ] 密钥解码后恰好为 `wjj学长爱鱼饼的`，没有额外换行或空格。
