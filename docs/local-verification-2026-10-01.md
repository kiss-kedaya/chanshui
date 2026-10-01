# 本地验收记录 · 2026-10-01

## 范围和基线

当前项目是chanzhushui，不是sub2api正式网关。工作分支keen-weaving-heron；原HEAD 0e36c25，复用feature/core快进到aebce8a。
会话971909c9-b5e2-4861-a4a5-3b19350b2234.jsonl按消息/工具记录读取，最后待办为API编排和入口。没有上传、没有部署、没有改远程或生产数据库。

## 真实接口验证（当次临时授权凭据）

模型claude-opus-5，Messages协议。测试地址仅保存在本地历史/本地报告，不硬编码到源码。

| 路径/能力 | 实际结果 |
|---|---|
| 根地址拉模型 | HTTP200，上游16条，与可测库交集11条 |
| 根地址四项检测 | 所有请求完成；固定开销6token、10/10；摘要回传；归因首选claude-opus-5；IQ首轮91 |
| IQ首轮91根因 | Q1正确的“暂缓发放、无必要叫醒”被关键词漏判；其它4题满分。已用完整答复复现并新增回归，不篡改这条历史 |
| /v1地址浏览器一键检测 | 所有8个可视探针完成；IQ100/100，5题全对；注入6token，10/10；R²=1，残差0 |
| 五个注入点 | 1→7、200→206、500→506、800→806、1000→1006 |
| 模型归因 | 3份有效序列；首选claude-opus-5。只表示库内候选排名 |
| summarized | 非空摘要，完整signature可展开，页面内容与后端返回逐字核对 |
| 浏览器单问单答 | 不传thinking，HTTP200，回复“本地验证通过”，thinking_tokens=0 |
| 历史 | 自动刷新侧栏，点击详情显示同一结果；首轮91与复测100都保留 |

第二轮各项耗时：注入7.6秒、摘要18.7秒、Q1 19.9秒、Q4 122.3秒、经典糖果110.2秒、随机糖果34.9秒、袜子18.9秒、数字指纹97.4秒。全并行，总耗时约122秒。不能把这一轮时间当稳定SLA。

## 自动测试

- gateway：根地址/v1、非法200 JSON、完整缓存统计、data无空格、正文/思考/签名delta、结束事件、截断错误。
- inject：回归、缓存修正、429有限退避、SSE辅助诊断、49/50/1000/1001等边界。
- fingerprint：原版Python源码生成6份数值oracle（80/81/292/303/320/332长度），74维特征与16维评分逐维误差<1e-9。
- IQ：Markdown保留答案、否定守卫、自然语言同义、答案总数在分解数字前、随机库存求解、半分规则、首轮真实Q1证据回放。
- API：单项503隔离、失败重跑只增加1次上游调用、成功项不重复；参数校验、跨Origin拒绝、默认私网阻断、取消、thinking模式、密钥脱敏、历史。
- Go全套与race、vet通过；Next.js静态构建和TypeScript检查通过。

## 浏览器验收

Mirasim托管CDP端点报socket hang up；已换本机Chrome + Playwright，不把MCP连接成功当页面成功。
已检查：首屏加载、模型拉取、浏览器一键真实检测、进度、摘要/签名展开、逐题结果、历史点击、单问单答。
失败重跑/停止使用浏览器网络夹具（非真实上游）：两个按钮各发出1次正确端点请求；后端另外有实际httptest验证，夹具结果不冒充真实上游结果。
390px窄屏检查scrollWidth=390，无横向溢出；桌面1440px同样无溢出。测试页面pageerror为空。
截图在.artifacts/home-desktop.png、live-desktop.png、mobile-live.png；实测完整报告在live-first-report.json、live-final-report.json。均不进git。

## 保留限制

- 数字库GPT/Claude两族，库外模型识别没有校准拒识阈值；高概率不能保证真实型号。
- 行为参考探针并非原基线等价实验，不参与计分。
- 固定开销包含包装，不能等价为精确提示词注入量。
- 旧工具A/B全矩阵、可编辑矩阵列和主动缓存写→读链未完整迁移，本轮不报全功能迁移完成。
- Next.js14现有依赖需在公开上线前另做升级审计。本轮仅本地静态托管。

## 追加：字号与最终版本

全站71处CSS字号统一+2px，历史页内联字号同步。浏览器computedStyle实测：body16px、标签14px、输入15px、主标题34px、侧栏模型14px。1440/1024/760/390/360五档均scrollWidth等于viewport，无横向溢出；390px历史按钮展开正常。最终截图desktop-font-plus2.png、mobile-font-plus2.png。
最终执行scripts/test-local.ps1通过；源码追加评分版本字段、thinking空白文本保护、数值总答案优先抽取后，race与vet复验通过。

## v4：全站分享报告与两次采样（本轮最新）

- 报告持久化32位report_id；旧4条报告补齐ID且保留内容，原数字URL可继续打开。重跑生成新报告ID，旧链接不变。
- 新浏览器空Cookie/无API Key可按ID或报告链接查看；复制ID和复制链接均读回剪贴板核对。
- 标题host使用URL解析，不含/v1；概率显示为0.0%的候选隐藏，本次页面仅显示非零首选。
- 全站历史无需浏览器本地数据，另一已打开浏览器自动出现新报告；55条历史分页夹具验证加载50+5，末页没有继续按钮。
- 本轮真实注入只有1/800两个输入：1→7、800→806，估计6token，10/10，耗时5.2秒。未增加第三次SSE探针；R²和残差为null，两点不伪装成已验证线性。
- 真实完整检测claude-opus-5：IQ100/100，五题全对；指纹首选claude-opus-5，摘要正常。新报告ID f6e3f7591eafad5e5ad635dc26bef1a3。
- 重启本地服务后同ID仍返回同报告，历史共5条。无密钥落盘，页面pageerror为空。1440/760/390/360四档无横向溢出。
- 自动验证：scripts/test-local.ps1通过（Go race+vet、前端3组单测、Next静态构建、TypeScript）。新增Go回归包括旧库迁移、唯一ID、分页、匿名读取、重跑快照保护、两次采样无第三请求、缺一采样不得计分。
- 备份：.artifacts/report-id-two-sample-before（源码、完整SQLite和旧二进制）；验收：report-sharing-browser-verification.json、live-two-samples-shared-report.json、shared-report-final.png。均忽略，不上传。
- 仍只监听127.0.0.1:8770，不部署、不暴露公网。分享功能已实现，同站访问者可以看；其他机器不能直接打开localhost地址。

## v5：Job队列、多协议、最新基线和缓存链（最新）

- 指定会话重新完整解析1947记录。ModelTrace主库最新commit d4131b30243dfa05e70180b5eedde742103f1d73，17数字基线，新增claude-sonnet-5-5；去重39模型。17维oracle重新与原Python参考对照通过。
- LLMprobe-engine commit 5c41136741ca52b5637879cca7bd0cae07404646，读取README.zh-CN.md和dist/runner.js核对OpenAI兼容协议。
- 真实Claude完整Job fb7dac43959774b2305142b130212593：提交202即job_id=report_id=task_id，24h TTL精确86400秒；另一个空会话浏览器在运行中可读进度；完成IQ100/100，思维链通过。
- 缓存实测：首次read0 + write12815，第二次read12815、write0，差值0，通过；请求无重试。
- 新模型搜索下拉、无HTTP代理控件、非Claude隐藏思维链/签名及提问thinking控件，390px无横向溢出；浏览器pageerror为空。域名夹具复制URL为https://report-check.invalid/history?id=<jobid>，无localhost硬编码。
- 本次密钥模型清单实际只提供Claude。GPT真实调用Responses和Chat均404，按规则总共4次后给明确错误；没有把这条标成非Claude实测成功。GPT/Chat各家字段和SSE适配通过本地mock测试，需有对应权限的密钥继续线上协议验收。
- TTL测试覆盖到期查询排除、行物理删除、重启interrupted；内存移除后失败重跑仍使用同一Job ID。独立操作token防止分享者取消/重跑。
- 并发模拟：worker1/queue2/upstream2，40同时提交，2入队38被429拒绝，上游峰值2；race无竞争。非生产容量测试。
- 本机i5-13600KF，空报告并行只读微基准99739次/2.96秒，24826ns/op、11511B/op（不含真实网络和完整报告）；仅作回归基准，不能外推公网吞吐。
- scripts/build-server.ps1已实际完成：Go race/vet、前端单测/类型/静态构建、Linux amd64交叉构建，本地包.artifacts/release-linux-amd64。尚未上传、未在Linux服务器运行。
- 最新本地服务重启后仍能按Job ID读同一份IQ100与缓存通过报告。截图v5-select-desktop.png、v5-select-mobile.png、v5-report-final.png；结果v5-live-job.json、v5-browser-check.json。
- 当前单实例便携服务，不支持多副本共享队列；上线前需资源压测、入口防滥用、依赖升级审查。

## UI/主题、反馈与新增能力检测（最新）

- 参考https://lmspeed.net/zh/audit/token-injection实际页面，使用其黑色主视觉、暖白背景、橙色重点与克制反馈思路；无广告/追踪脚本引入。
- 橙/薄荷×日/夜四套组合均经过1440/760/390/360四档实浏览器检查，无横向溢出；刷新记住主题。密钥眼睛只有图标，重复说明/模型同步commit与时间不再显示。
- 模型加载mock门控验证spinner的transform确实变化；运行中有旋转和骨架、结束停转，prefers-reduced-motion下活动动画0；没有为UI验收浪费上游额度。
- 反馈界面真正POST成功201，后台导出核对1条本地fixture；响应不含联系方式，公共GET /api/feedback不可读。主题夜间反馈/关闭焦点/手机入口通过。
- 服务日志确认后台自动同步已检查；模拟最新commit下载、校验失败不替换、重启读已验证缓存、并发读取和发布均有race测试。
- 原生Anthropic真实Job 4b8846b5f5ca1c27bc707abcc43fe026：全部探针done，IQ100/100；工具返回audit_echo及随机nonce、sum42；搜索返回执行完成+来源证据。仅称上游工具回传支持，不由自述判定。
- 报告页面Q1安眠药/Q2色盲/Q3经典糖果/Q4糖果变体/Q5袜子，各分值不变；分享复制链接正常。
- 四协议本地负例保证手动模式404后仍不降级；三协议工具schema/响应结构测试通过，Chat联网unknown而不伪装通过。
- 截图及证据在.artifacts/ui-*、theme-*、final-orange-report.png。Key扫描仍需最后确认，私有反馈导出不纳入git。

## 视觉重排与三请求缓存严谨验证

- 参考 lmspeed.net token-injection 实际渲染页：深色审计Hero、编号证据过程、暖白细边框、克制橙色动作，不复制广告/登录/分析脚本。
- 结果卡由CSS grid行对齐改为两列独立瀑布流，卡片间距14px；右侧长卡不再让左侧出现大白块。390/360/760/1440宽度无横向溢出。
- 缓存验证现在固定三个串行小请求：首条写入，第二/第三条读取；返回每条read/write/input/total/hit_rate，两段expected_read/actual_read/delta，以及warm/overall hit rate。字段为null、缺写字段或全0时unknown，不判假通过。缓存前缀约6.4k Claude文本，避免截图中超大无关内容。
- 根据 claude-code-best/claude-code main @77a7934e15d69da13879112ed7db695c9ee7a52a 审查：缓存命中率使用 read/(input+creation+read)，缓存变化按read token与缓存策略观察；工具结果必须按tool_use_id配对，孤立/嵌套伪块不计数。相关严格解析和重复流样本测试已加入。
- 工具调用从“看到tool_use即通过”升级为：唯一结构化调用、名称/随机nonce/参数校验、固定调用ID返回tool_result/function_call_output、收到确认文本后才通过；联网只接受服务端搜索块+来源URL，不认普通文本声称。
- 新增反馈入口和私有表，成功POST201；管理员导出工具不暴露HTTP接口。新增背景模型同步：启动检查、正常6小时、失败5分钟重试，校验后缓存替换，前端不展示commit/同步时间。
- 本轮UI真实验收：主题四组合、工具/联网Claude真实Job均完成；Job工具通过、联网通过、IQ100；控件、下拉动画、feedback、动画减少偏好无pageerror。

## 最终视觉收口

- 参考站页面结构落成：深色审计Hero、编号检测流程、结果独立瀑布流卡片、悬浮说明、反馈对话框和克制动画。
- 默认暖橙日间，薄荷绿日间、暖橙夜间、薄荷绿夜间均可切换并持久化；结果页和首页四档视口无横向溢出。
- Q1～Q5显示顺序最终核对为：Q1安眠药、Q2红绿色盲、Q3经典糖果、Q4糖果变体、Q5袜子配对；内部判题ID仍保留Q1/Q4/Q6/Q8/Q7，分值没有变化。
- 结果页三次缓存样本字段、每次命中率、warm/overall命中率、两段过渡；卡片改为两列独立columns，截图大空白消失。
- 手动主题、工具/联网和反馈交互的浏览器验收均无pageerror；prefers-reduced-motion生效。
