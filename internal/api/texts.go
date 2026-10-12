package api

// 本文件集中存放所有面向新生的中文文案，与 project.md 第 3 节保持一致。
// 正文统一 text/plain; charset=utf-8，由 gin 的 c.String 保证。

// 3.1 首页
const textHome = `这里好像什么都没有……
但你目前能看到的并不是真相。

提示：一次网络响应不只有正文，看看它的“头”部。
浏览器开发者工具的 Network（网络）面板，也许能帮到你。`

// 3.2 领票。占位符：有效期分钟数、初始 JWT。
const textTicketFmt = `你找到了一张通行证，有效期为 %d 分钟。

JWT: %s

下一站：/api/challenge
先使用 GET 尝试访问，再沿着响应中的线索继续探索。

提示：这张通行证似乎应该放进 Authorization 请求头。
如果网站没有放行，记得查看响应头和正文中的提示。
每次领票都会生成独立会话。请只领取一次，并在本轮挑战中复用同一张通行证。`

// 3.3 挑战说明。占位符：窗口秒数。
const textInstructionsFmt = `你已经找到鱼饼保险箱，但还需要凑齐 10 个编号才能打开。

请向当前地址 /api/challenge 发送 POST 请求：
- 继续携带 Authorization: Bearer <你的通行证>
- 设置 Content-Type: application/json
- 请求体示例：{"seq": 1}

从第一个有效编号被收到开始，你有 %d 秒时间提交 1～10。
每个请求只提交一个编号，编号不能重复计数。
阅读本说明不会启动计时。

提示：
- 所有请求必须复用同一张通行证，不要每次重新领票。
- 可以使用 go 关键字并发发送这 10 个请求。
- 主程序需要等待请求完成，并读取每个请求的响应。
- 集齐编号且顺序符合要求时，那次响应会给出新的通行证和奖励入口。
- 可以先打乱编号再并发发送；并发并不保证服务端收到的顺序一定符合要求。
- 超时或因完整升序、完整降序被拒绝后，下一次有效提交会自动开始新一轮。
- 如果错过通关响应，再用原通行证 GET 本接口即可取回通关信息。`

// GET 附加的进度提示。
const (
	hintNotStartedFmt = `

当前没有进行中的挑战。第一个有效提交将开始 %d 秒计时。`
	hintActiveFmt = `

当前进度：%d/10。
首次接收顺序：%s。`
	hintTimedOut = `

上一轮已超时，下一次有效提交将重新开始。`
	hintRejectedFmt = `

上一轮 10 个编号已集齐，但接收顺序为完整%s，本轮未通过。
首次接收顺序：%s。
本轮进度已清空。请继续使用同一张通行证，重新提交全部 10 个编号。
下一次有效提交会启动新的 %d 秒计时。`
)

// POST 响应文案。占位符按各自 fmt 说明。
const (
	bodyNewRoundFmt = `已开启新一轮挑战，计时 %d 秒。
已收到编号 %d，当前进度：%d/10。
请在时间窗口内继续提交其余编号。`
	bodyProgressFmt = `已收到编号 %d，当前进度：%d/10。
首次接收顺序：%s。`
	bodyDuplicateFmt = `编号 %d 已收到，本次不重复计数。
当前进度：%d/10。计时不会延长。`
	bodyRoundAfterTimeoutFmt = `上一轮已超时，旧进度已清空。
已开启新一轮挑战，计时 %d 秒。
已收到编号 %d，当前进度：%d/10。
建议准备好全部编号后并发发送，并等待所有响应完成。`
	bodyRoundAfterRejectFmt = `上一轮因完整%s被拒绝，旧进度已清空。
已开启新一轮挑战，计时 %d 秒。
已收到编号 %d，当前进度：%d/10。
请先打乱编号再提交，避免再次出现完整升序或完整降序。`
)

// 3.4 集齐但顺序不符合要求（409）。占位符：升序/降序、本轮首次接收顺序、窗口秒数。
const textRejectedFmt = `10 个编号已集齐，但本轮未通过：接收顺序为完整%s。
本轮首次接收顺序：%s。

完整升序和完整降序都会被拒绝。
提示：可以先打乱 1～10，再使用 go 关键字并发发送。
并发不保证接收顺序一定符合要求；请检查每个响应，必要时重新尝试。

本轮进度已清空。请继续使用同一张通行证，重新提交全部 10 个编号。
下一次有效提交会启动新的 %d 秒计时，阅读说明不会启动计时。`

// 3.4 成功集齐。占位符：通关 JWT、有效期分钟数。
const textPassFmt = `10 个编号已集齐，且接收顺序既不是完整升序，也不是完整降序。鱼饼保险箱已打开！

Authorization: Bearer %s

奖励入口：/api/reward
请使用 GET 访问，并把 Authorization 请求头替换为上面的新通行证。
新通行证有效期为 %d 分钟，旧通行证不能领取奖励。`

// 3.5 奖励
const textReward = `🍥

恭喜你，找回了 wjj 学长的鱼饼！挑战成功。

密钥：d2pq5a2m6ZW/54ix6bG86aW855qE

这段密钥采用了 Base64 编码，有兴趣可以解码看看。`

// 4.2 错误提示。401 场景均附完整 Bearer 写法（渐进式揭示在首次失败时释放）。
const (
	msgMissingAuth = `请先访问 /api/ticket 领票，并设置请求头：
Authorization: Bearer <你的通行证>

如果还没有通行证，请先访问 /api/ticket。`
	msgNeedsBearer = `通行证格式不正确，请在前面加上 Bearer，注意中间的空格：
Authorization: Bearer <你的通行证>

请使用刚才领取的 JWT，不要把它放进 URL 或 JSON 请求体。`
	msgInvalidToken     = `通行证无效或已过期，请重新领票；不要修改 JWT 内容。`
	msgSessionGone      = `本轮会话已失效，请重新访问 /api/ticket。`
	msgStageChallenge   = `当前通行证不能访问此阶段；挑战接口需要 /api/ticket 发放的初始通行证。`
	msgStageReward      = `当前通行证不能访问此阶段；领取奖励需要挑战通过后得到的新通行证。`
	msgUnsupportedMedia = `请设置 Content-Type: application/json`
	msgBadBody          = `请求体应为 {"seq": 1}，seq 必须为 1～10 的 JSON 整数。`
	msgInternal         = `服务开小差了，请稍后重试。`
)

// 405 / 404 提示。
const (
	msgMethodGetOnly   = `该路径只支持 GET。请使用 GET 访问，并留意响应头中的线索。`
	msgMethodChallenge = `挑战接口支持 GET 和 POST。
查看说明与进度使用 GET；提交编号使用 POST，并设置 Content-Type: application/json。`
	msgNotFound = `这里没有你要找的东西。
请从首页 / 出发，沿着响应头中的线索探索。`
)
