# 「掺水？」项目需求原文汇总（供转交下个 Agent）

> 本文为旧会话需求档案，旧上传/部署/进度已被 v3 本地计划覆盖。当前禁止上传、打标签、部署；实际验收见 docs/local-verification-2026-10-01.md。

> 以下是老板在 2026-10-01 会话里对本项目说过的所有要求，按主题整理，原话口径保留。

## 项目定位

- 网站改名 **「掺水？」**——检测 API 中转站有没有掺假/降智/注水。
- 定位：**简单、快速、高效的检测站**，不需要询问用户太多东西，填 URL + Key + 模型名就能一键出结果。
- 目标是做出**顶级 SaaS 和行业独角兽级别的功能**。
- **不要任何广告**（无广告位、无推荐站点、无定价页、无友情链接）。

## 四项核心检测（一键直测）

1. **注入 token 数量**——网关每次请求额外注入多少系统提示词 token（比如发 "Hi" 实际计费多少 token，多出来的就是注入量）。
2. **模型家族 + 最大可能的具体型号**——模型指纹归因。
3. **summarized 后思考文本是否真的返回**——不能只返回 signature（签名），必须能看到思考文本内容。
4. **智商评分，满分 100 分**——参考 iqbench 的评分标准。

## 一键测试

- 要有**一键测试功能**：点一下按钮，四项检测全部并行跑完出结果。

## 技术要求

- **一切鉴定走后端**，主要逻辑思维走后端，前端不要暴露太多功能。
- **后端用 Go 程序，不要 Python**。
- **前端用 Next.js**（他明确说喜欢这个）。
- UI 风格参考 **https://cctest.ai/zh**（首屏输入卡片 + 一键开始 + 结果徽章风格）。
- 侧边栏要加**历史检测记录**，参考 **https://manxue.ai/** 的布局。

## 页面要显示

- **限制用户能选择的模型**（可测模型清单）。
- **当前存储模型指纹数量**（库容量展示）。

## 参考项目（抄精华，注意别抄错文件）

| 项目 | 地址 | 取什么 |
|---|---|---|
| ModelTrace | https://xqy2006.github.io/ModelTrace/ <br>https://github.com/xqy2006/ModelTrace | **核心逻辑**：模型家族鉴定。三发长整数挑战（1..355，292~333 个）→ 数字指纹 → Hellinger + 有序块融合评分 → softmax 概率 + 家族聚合 |
| LLMprobe-engine | https://github.com/Bazaarlinkorg/LLMprobe-engine | **补充更多指纹**：34 个模型的行为指纹基线（拒答梯度/格式偏好/不确定性），ModelTrace 指纹不够全时补充；另有 token 膨胀侦测、缓存侦测、SSE 流格式探针思路 |
| iqbench | https://github.com/yclenove/iqbench | **评分标准** + **红绿色盲题、糖果题、糖果题变题（参数化防背题）、袜子配对题**，最后汇总评分满分 100 |

- 指纹库现状：ModelTrace 库 16 模型（GPT 5.4~6 全系 + Claude haiku4.5~opus5.5）；LLMprobe v3e 基线 34 模型（补齐 claude 4.5~4.8/fable-5、deepseek、gemini、grok、glm 等 27 个）。两库都已在仓库 `data_ref/` 落盘。
- 行为指纹只作低置信时的佐证（不进归因概率，避免两库分布空间不可比污染）。
- 抄东西时**务必确认没抄错、没改错文件**。

## 工程与仓库要求

- GitHub **私有仓库**：https://github.com/kiss-kedaya/chanzhushui
- **使用 worktree 多功能多模块同时干活**；所有 worktree 收在**一个入口文件夹**里（`F:\GO\chanzhushui\`，不要在 F:\GO 下散落一堆文件夹）。
- 完成一个功能/模块就 **commit + 说明**，推到 GitHub。
- 实施计划：`docs/superpowers/plans/2026-10-01-chanshui-detection-station.md`（v2）。

## 当前进度（截至交接）

- 6 个 Go 模块全部完成并合入 `feature/core` 分支（构建+全测试绿）：gateway/inject/fingerprint/thinking/iq/history
- Next.js 前端（静态导出）已完成并合入：侧边栏+首屏卡片+四结果卡+历史详情
- **待做**：`internal/api/handlers.go`（四模块编排 + /api/detect 轮询 + /api/bank/stats + /api/history）、main.go 收口（路由注册 + embed 前端 + 真实网关冒烟）→ tag v0.1.0
- 各模块 API 签名已勘察完毕（详见 git log 里的各 feat commit）

## 说话方式提醒

- 全程中文、短句、先结果后动作。
- 修完按「修前/修后/怎么验/生不生效」汇报。
- 口令、key、token 不写进 md、不进 git、不贴回复。
