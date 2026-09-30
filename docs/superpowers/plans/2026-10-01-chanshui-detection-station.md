# 掺水？检测站 实施计划 v2

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** SaaS 级单页检测站「掺水？」——填 URL/Key/模型，一键检测四项：注入 token 数、模型家族+具体型号归因（数字指纹为主 + 行为指纹补充）、summarized 思考文本回传、IQ 评分（满分 100，含色盲/糖果变题/袜子配对）；侧边栏历史记录存库；所有鉴定逻辑在 Go 后端；前端 Next.js。

**Architecture:** 双进程架构。**Go 后端**（`server/`，net/http + SQLite，端口 8770）承载全部鉴定逻辑并暴露 `/api/*`；**Next.js 14 前端**（`web/`，App Router，`next build` 产静态导出由 Go embed 或独立 `next start`）只做表单/轮询/渲染。前端不含任何判分/回归/归因逻辑，也**不直接访问网关**——所有探测请求都从 Go 后端发出（黑盒、防反查、可加代理）。

**Tech Stack:** Go 1.22（net/http、embed、modernc.org/sqlite 纯 Go）；Next.js 14（App Router、TypeScript、Tailwind CSS v4、静态导出模式）；无其它运行时依赖。

---

## 已核对的素材（子代理直接用，不用再调研）

### 指纹库（已下载到仓库 `data_ref/`，勿删）

| 文件 | 内容 | 用途 |
|---|---|---|
| `data_ref/unified_bank.json`（834KB） | ModelTrace 统一指纹库，16 模型 × 数字指纹：models[]（id/display_name/family/counts[355]）、robust.hellinger（feature_mean[355]、feature_scale[355]、nuisance_basis[2×355]、centroids[16×355]）、robust.ordered_blocks（weight=0.25、feature_mean[74]、feature_scale[74]、nuisance_basis、centroids[16×74]、environment_centroids[12×74]）、calibration{1,2,3}（3→beta=12.0, cv_accuracy=1.0）、method{name, range:[1,355], alpha:0.5, ordered_block_weight:0.25} | **数字指纹主库**，完整照抄算法 |
| `data_ref/probe_v3e_snapshot.json`（32KB） | LLMprobe-engine v0.10.1 行为基线，34 模型（含 27 个数字库没有的：claude 4.5~4.8/haiku4.5/sonnet4.5/4.6/fable-5、deepseek-v3.2/v4-flash/pro、gemini 2.5~3.1、gpt-4.1~5.6、grok-4.3/4.5、glm-5.2），每模型 {refusalLadder(8 维向量+拒答长度/首轮档位), formatting(bullet 模式/标题深度/em-dash 率/代码语言标签), uncertainty(值均值/取整率/标准差), sampleSize, updatedAt} | **行为指纹补充库**：对数字指纹低置信（top1 概率 < 0.6）的待测模型，追加 3 发行为探针（拒答梯度轻量版 2 问 + 格式偏好 1 问），与该库 34 模型做余弦比对，只输出"行为指纹最像的 3 个模型"作为参考证据（不进归因概率，避免两库分布空间不可比污染） |

### ModelTrace 算法（照抄语义，Go 重写）

数字解析：`parse_numbers` = 全文正则 `\d+`，按字母分隔断 run，取**最长 run**，保留 1..355；有效数 ≥80（或 0.55×expected）才算有效输出。
特征：Hellinger = sqrt((counts+0.5)/sum)（355 维）；有序块 = 4 段×16 bin 直方图 + 末位数字 10 bin（74 维，sqrt 平滑）。
评分：hellinger 侧标准化→投影去 nuisance basis→与 centroids 余弦→standardize；ordered 侧同流程 + environment_centroids 取 max 模板分；融合 = 0.75×hellinger + 0.25×ordered（0.25 来自 bank.robust.ordered_blocks.weight）。
归因：三发分数平均 → `P(model)=softmax(beta×score)`（beta 按 valid 数取 calibration[str(min(n,3))].beta）→ `P(family)=Σ P(models∈family)`；另报 js_similarity 作参考。
挑战：3 发，长度 rng 292..333，文案模板照抄（开句 5 选/动作 5 选/结尾 5 选/分隔提示 4 选），禁工具声明 + 直接输出声明。

### iqbench 题库与评分（照抄规则）

5 题、卷面 58、总分 `round(100 × 通过题数满分和/卷面满分)`，**半分只进标记不进分子**：

| id | 题 | 卷面 | 限秒 | 判分 |
|---|---|---:|---:|---|
| Q1 | 安眠药（静态） | 6 | 55 | 命中「不叫醒/不喂/暂缓给药/持药观察」且无「叫醒」强动作 → 6 |
| Q4 | 红绿色盲（参数化 38↔88 / 27↔72 / 45↔54 三组随机） | 14 | 90 | 「非亲生/不是亲生」14；只答遗传链（伴X/隔代/外公）7；「亲生/正常」0 |
| Q6 | 经典糖果题（圆 7/9/8 星 7/6/4 固定） | 16 | 120 | 21→16；29→8+「替代建模」；其它 0 |
| Q8 | 糖果变体（库存随机：三类各 4..10） | 16 | 120 | 命中求解器答案→16；答公式值→8+「替代建模」；答 21→标「疑似套用」；盲取 25→8+「替代建模」 |
| Q7 | 袜子配对（3-4 色×2-7 只×p=2..4 双，**只出真实≠公式值的实例**） | 16 | 120 | 命中穷举最坏值→16；答 2p+c−1→8+「替代建模」 |

答案抽取：去 Markdown → 末尾「最终答案/final answer/the answer is/答案:」槽 → 正文空时用思维链末尾 2000 字 → 数值等价（0.10==0.1）。
系统提示：`禁止使用任何工具。回答最后单独一行"最终答案: <短结论>"。`（Q4 恒定答案不变，参数化只换表观素材）

### LLMprobe-engine 精华（并入）

1. **token 膨胀侦测**：prompt_tokens 与内容 BPE 估算对比 → 已并入注入测量的"对照通道"
2. **缓存侦测**：响应头 `x-cache`/`cf-cache-status` 采集 → 附加诊断字段
3. **SSE 流格式探针**：`stream:true` 一发，校验 `data:` 帧格式与 `[DONE]` → 附加诊断字段（不计分）
4. **CLI 式逐行报告样式**：`[1/19] ✓ 探针名 12.7s 说明` → 前端检测过程时间线渲染
5. **探针逐项隔离**：单探针失败不影响其余 → 后端编排模式

### UI（cctest.ai 首屏 + manxue 侧边栏）

- 顶部 hero 卡片：三输入（接口地址/API Key/目标模型）+ 「一键检测」大按钮 + 可折叠高级项（并发/超时）
- 检测中：探针时间线（逐行 ✓/✗/时长，LLMprobe 风格）
- 结果区：四张卡片（注入 token 大数字 / 家族徽章+top 模型概率条 / 思考文本 Pass-Fail 徽章 / IQ 大分数+每题展开）
- 左侧固定侧边栏：logo「掺水？」+ 历史记录（时间/站点/模型/分数徽章），点击回看完整报告
- **无任何广告、无推荐站点、无定价、无 footer 友情链接**
- 首页顶部显示两条只读统计：`已收录指纹模型 N 个`、`支持家族：GPT / Claude / Gemini / DeepSeek / Grok / GLM`（来自 GET /api/bank/stats，用户可感知库容；模型下拉同样由 /api/bank/stats 提供的可测模型清单驱动）

---

## 文件结构

```
chanzhushui/
├── go.mod                        # module chanzhushui（Go 侧在 server/ 目录作子 module？否——单 module 根目录）
├── main.go                       # Go 入口：/api/* + 静态服务（embed web/out 若存在）
├── internal/
│   ├── gateway/client.go         # /v1/messages 客户端（UA、超时、usage/block 全解析、SSE）
│   ├── inject/inject.go          # 注入回归测量（主通道）+ 内容长度对照（次通道）+ 缓存头采集
│   ├── fingerprint/bank.go       # 双库加载（内嵌 generated JSON）
│   ├── fingerprint/feature.go    # 数字解析 + Hellinger + 有序块
│   ├── fingerprint/attr.go       # 归因评分 + 家族聚合 + challenge 生成
│   ├── fingerprint/behavior.go   # 行为指纹比对（v3e 库，低置信时启用）
│   ├── thinking/thinking.go      # summarized 思考文本回传检查
│   ├── iq/generate.go            # 参数化生成器 + 求解器（色盲/糖果/糖果变体/袜子）
│   ├── iq/judge.go               # 答案抽取 + 每题判分
│   ├── iq/score.go               # 加权汇总 100 分制
│   ├── history/store.go          # SQLite
│   └── api/handlers.go           # /api/detect /api/detect/{id} /api/history /api/bank/stats
├── data_ref/                     # 原始参考库（unified_bank.json、probe_v3e_snapshot.json）
├── server/data/bank.generated.json   # 由 data_ref 生成的 Go 内嵌库（转换脚本产物）
├── scripts/convert_bank.py       # 一次性转换脚本（提交，便于库更新）
├── web/                          # Next.js 14 App Router（独立 package.json）
│   ├── package.json
│   ├── next.config.mjs           # output: 'export'、distDir、images.unoptimized
│   ├── app/layout.tsx            # 深色侧边栏 + 主区骨架
│   ├── app/page.tsx              # 检测主页面（输入卡片+进度+四结果卡）
│   ├── app/history/[id]/page.tsx # 历史详情页
│   ├── lib/api.ts                # fetch 封装（/api/* 透传到 Go :8770）
│   └── components/…              # ResultCards/Badge/Timeline/ScoreRing
└── docs/superpowers/plans/
```

**worktree 分工（全在 F:\GO\chanzhushui\ 下，文件域不重叠）：**

| worktree | 分支 | 拥有文件 |
|---|---|---|
| 主检出 | main | 计划、README、合并 |
| dev | feature/core | main.go、go.mod、收口 |
| dev-inject | wt-inject | internal/gateway/**、internal/inject/** |
| dev-fingerprint | wt-fingerprint | internal/fingerprint/**、scripts/convert_bank.py、server/data/bank.generated.json、data_ref 转换产物 |
| dev-thinking | wt-thinking | internal/thinking/** |
| dev-iq | wt-iq | internal/iq/** |
| dev-history | wt-history | internal/history/** |
| dev-web | wt-web | web/** |

**依赖顺序：** dev-inject 先建 go.mod+gateway（其它 Go 模块 worktree 从 wt-inject 分支 rebase 或等 main merge 后 rebase——简单化：**go.mod 与 go.sum 由主会话先在 main 建好**（Task 0），各模块 push 后 main 逐个 merge，模块 worktree 不需要再建 go.mod）。

---

## Task 0: main 分支放好 go.mod 与目录骨架（主会话自己干，5 分钟）

- [ ] go.mod（module chanzhushui、go 1.22、require modernc.org/sqlite v1.29.10）+ `go mod tidy`
- [ ] 空 internal/{gateway,inject,fingerprint,thinking,iq,history,api}/ 目录占位（.gitkeep）
- [ ] .gitignore（*.exe、*.db、web/node_modules、web/.next）
- [ ] commit + push：`chore: module skeleton and package scaffolding`

## Chunk 1: 四个独立 Go 模块（可并行，四个子代理）

### Task 1 [dev-inject]: gateway 客户端

**Files:** Create `internal/gateway/client.go`, `internal/gateway/client_test.go`
**依赖：** main 的 go.mod（worktree 已含）。

- [ ] 失败测试：httptest 假网关回标准 messages JSON（含 usage.output_tokens_details.thinking_tokens、content[]含 thinking{thinking,signature} 与 text、response headers x-cache）→ 断言 Metrics 全字段（InputTokens/ThinkingTokens/CacheCreation5m/CacheCreation1h/CacheRead/ThinkLen/SigLen/ThinkingText/Signature/CacheHeader）与请求头（x-api-key、anthropic-version、user-agent=claude-cli/2.1.284…）
- [ ] 失败 → 实现 client.go（结构见计划 v2 素材段；支持 ThinkingConfig 指针、system 数组带 cache_control、SSE 流式原始帧采集备用）
- [ ] 测试通过 → commit `feat(gateway): messages client with full metrics and cache headers`
- [ ] push wt-inject

### Task 2 [dev-inject] 注入测量

**Files:** Create `internal/inject/inject.go`, `internal/inject/inject_test.go`

- [ ] 失败测试：mock 按 0.738×n+44 → Inj∈[43,45]、Slope∈[0.735,0.741]；某点带 cache_creation → total 口径仍线性；429 → 退避重试后成功
- [ ] 实现：Measure(ctx, gw, model, ua) 5 点（1/200/500/800/1000）最小二乘；同时记录 800/1000 两点的 cache 头与 SSE 探针（stream:true 一发 200 字符）作诊断字段；429 退避 30s×4
- [ ] 通过 → commit `feat(inject): regression measurement with cache-header diagnostics`
- [ ] push

### Task 3 [dev-thinking] 思考文本检查

**Files:** Create `internal/thinking/thinking.go`, `internal/thinking/thinking_test.go`

- [ ] 失败测试：三形态（文本+签名→Pass；仅签名→Fail"思考被清空，只回签名"；无块→Fail"无思考块"）
- [ ] 实现 Check：一发 adaptive+display=summarized；Verdict{Pass bool, ThinkLen, SigLen, Reason}
- [ ] 通过 → commit `feat(thinking): summarized passthrough check`
- [ ] push

### Task 4 [dev-history] SQLite

**Files:** Create `internal/history/store.go`, `internal/history/store_test.go`

- [ ] 失败测试：内存库 Save→List 倒序→Get 完整字段
- [ ] 实现：detections 表（id/created_at/base_url/model/family/family_prob/top_model/top_prob/thinking_pass/thinking_note/injection/iq_score/verdict_json）；List 限 50
- [ ] 通过 → commit `feat(history): sqlite store with summary list and detail`
- [ ] push

### Task 5 [dev-iq] 生成器 + 求解器

**Files:** Create `internal/iq/generate.go`, `internal/iq/generate_test.go`

- [ ] 失败测试：
  - GenColorBlind：三组数字对随机；题面含色盲叙述与「崩溃」；正确答案恒「非亲生」
  - GenCandy：随机库存→SolveCandy 穷举最坏保证（圆取 k₁+星取 k₂ 最小总数）；两次调用可能不同
  - GenSocks：随机色数 3..4、每色 2..7、目标 p 2..4；SolveSocks 穷举；**过滤掉解=2p+c−1 的实例**
- [ ] 实现（算法照 iqbench spec Q8 堵四类对手的推导泛化）
- [ ] 通过 → commit `feat(iq): parameterized generators with worst-case solvers`
- [ ] push

### Task 6 [dev-iq] 判分器

**Files:** Create `internal/iq/judge.go`, `internal/iq/judge_test.go`

- [ ] 失败测试：Q1 安眠药关键词；Q4 满分/遗传链半分；Q6 21 满分、29 半分+「替代建模」；Q8 求解器值满分、公式值半分、21→「疑似套用」、25→半分；Q7 求解器值满分、2p+c−1 半分；抽取链（Markdown 剥离→答案槽→思维链兜底→数值等价）
- [ ] 实现 judge.go + 题面文案常量（含参数化插值）
- [ ] 通过 → commit `feat(iq): answer extraction and judges with half-credit tags`
- [ ] push

### Task 7 [dev-iq] 100 分制汇总

**Files:** Create `internal/iq/score.go`, `internal/iq/score_test.go`

- [ ] 失败测试：全对 100；袜子半分 ≈86（16×0.5/58 → round(100×(50/58))）；标签透传
- [ ] 实现 score.go（通过才进分子；输出 PerQuestion 明细）
- [ ] 通过 → commit `feat(iq): weighted scoring to 0-100`
- [ ] push

## Chunk 2: 指纹归因（一个子代理串行三步）

### Task 8 [dev-fingerprint] 库转换

**Files:** Create `scripts/convert_bank.py`, `server/data/bank.generated.json`；Test: 转换后 Unmarshal 冒烟

- [ ] 写转换脚本：读 data_ref/unified_bank.json + data_ref/probe_v3e_snapshot.json → 输出 bank.generated.json {numeric: {models, robust, calibration, method}, behavioral: {models:[{modelId, family, displayName, refusalLadder, formatting, uncertainty, sampleSize}]}}；**字段语义 1:1，不重算任何矩阵**
- [ ] 跑脚本 → 生成的 JSON 16+34 模型、矩阵维度断言（355/74/16×355/16×74/2×355）
- [ ] commit `feat(data): generated bank from ModelTrace + v3e snapshot` + push

### Task 9 [dev-fingerprint] 特征与归因

**Files:** Create `internal/fingerprint/{bank.go,feature.go,attr.go}` + 三个测试

- [ ] 失败测试 feature：ParseNumbers 边界（356/0 剔除、跨字母断 run、最长 run）；HellingerFeature 归一；OrderedBlockFeature 74 维
- [ ] 实现 feature.go（纯 Go，自带 matVec/dot/standardize 工具）
- [ ] 通过 → commit `feat(fingerprint): number parsing and feature extraction`
- [ ] 失败测试 attr：bank 模型 counts 采样自检 → 归因回自身或同家族 top2；GenerateChallenges(3) 文案与 expected_count 校验
- [ ] 实现 attr.go（robust_score_numbers 语义 + softmax(beta×score) + 家族聚合 + challenge 模板）
- [ ] 通过 → commit `feat(fingerprint): attribution with family aggregation and challenges` + push

### Task 10 [dev-fingerprint] 行为指纹补充

**Files:** Create `internal/fingerprint/behavior.go`, `internal/fingerprint/behavior_test.go`

- [ ] 失败测试：给定 mock 网关（拒答梯度回「无法确认」、格式偏好回带 em-dash 长文）→ CompareBehavior 返回 3 个最像模型 + 相似度；库无该 family 时返回空不报错
- [ ] 实现：3 发轻量探针（拒答梯度 2 问：金融建议过度承诺/自我推理评分，**照 v0.10.1 安全改版文案，绝不复刻有害版本**；格式偏好 1 问：请用 markdown 列 5 条建议）；特征：拒答长度均值/首轮档位 + bullet 模式/标题深度/em-dash 率 → 与 v3e 库 34 模型余弦比对；仅在数字归因 top1 概率 <0.6 时调用
- [ ] 通过 → commit `feat(fingerprint): behavioral fingerprint cross-check for low-confidence attribution` + push

## Chunk 3: API 编排 + Next.js 前端

### Task 11 [dev-web] API handlers（Go 侧）

**Files:** Create `internal/api/handlers.go`, `internal/api/handlers_test.go`

- [ ] 失败测试：
  - POST /api/detect {base_url,key,model} → {task_id}；mock 四模块接口全成功 → GET /api/detect/{id} 返回四项聚合 verdict；单模块错误隔离（inject 挂了其它三项照出）
  - GET /api/bank/stats → {supportedModels:[…34+16 去重 id 清单], fingerprintCount:16+34, families:[…]}
  - GET /api/history → 最近 50 摘要；Key 不落盘（detect 请求体只进内存，日志脱敏）
- [ ] 实现：内存任务表+goroutine 编排（inject/fingerprint/thinking 并行，iq 5 题并发，全完聚合作 verdict 存 history）；`/api/detect/{id}` 支持轮询（running/partial/done 状态）
- [ ] 通过 → commit `feat(api): detect orchestration, bank stats, history endpoints` + push

### Task 12 [dev-web] Next.js 前端

**Files:** Create `web/package.json`, `web/next.config.mjs`, `web/tsconfig.json`, `web/tailwind.config.ts`(v4 用 CSS 配置可省), `web/app/{layout.tsx,page.tsx,globals.css,history/[id]/page.tsx}`, `web/lib/api.ts`, `web/components/{DetectForm.tsx,ResultCards.tsx,Timeline.tsx,ScoreRing.tsx,Sidebar.tsx}`

- [ ] **Step 1:** `package.json`（next@14、react@18、typescript、tailwindcss@4）+ `next.config.mjs`（`output:'export'`、images.unoptimized、api 代理不用 rewrites——前端 `lib/api.ts` 直连 `NEXT_PUBLIC_API_BASE` 环境变量默认 `http://127.0.0.1:8770`）
- [ ] **Step 2:** `layout.tsx`：左侧固定侧边栏（w-64，logo「掺水？」+ `GET /api/bank/stats` 渲染「指纹 N 模型 · M 家族」+ 历史列表 GET /api/history，条目：时间/站点域名/模型/分数徽章，点击路由到 `/history/[id]`）；主区 `overflow-y-auto`
- [ ] **Step 3:** `page.tsx`：首屏 hero 卡片（cctest 风格：三输入+「一键检测」按钮+可选高级折叠）→ 点击后时间线组件（轮询 /api/detect/{id}，逐模块状态行 ✓/✗/耗时）→ 完成渲染四结果卡：
  - 注入卡：大数字 `≈516 tok` + 副文案「每次请求额外系统提示」+ 斜率与采样点折叠
  - 归因卡：家族徽章（GPT/Claude/Gemini/DeepSeek/Grok/GLM 各配色）+ top 模型概率条 top3 + 「行为指纹佐证」折叠区（低置信时才显示）
  - 思考卡：Pass 绿徽章 / Fail 红徽章 + 文本长度/签名长度数字 + Reason 一句话
  - IQ 卡：ScoreRing 大分数（0-100）+ 每题行（分值/半分标签/展开看答案对比）
  - 顶部两条只读统计条：`已收录指纹模型 N 个`（来自 bank stats）
- [ ] **Step 4:** `app/history/[id]/page.tsx`：GET /api/history/{id} 渲染完整 verdict（同结果卡复用组件）
- [ ] **Step 5:** 样式基调：深色侧栏（#11161d）+ 浅主区（#f4f5f7）、卡片圆角 10px 阴影、等宽数字、绿/红/黄徽章；**模板里不写任何广告/推荐/定价组件**
- [ ] **Step 6:** `npm install && npm run build`（静态导出）成功；`next dev` 手验表单交互（填假 key → 错误态正常渲染）
- [ ] commit `feat(web): next.js ui — hero card, timeline, four result cards, history sidebar` + push

### Task 13 [dev] 收口

**Files:** Modify `main.go`（路由注册、embed web/out）、`README.md`

- [ ] main merge 六个 wt-* 分支（文件域不重叠，理论无冲突；冲突以各模块为准手工解）
- [ ] main.go：注册 /api/* + `//go:embed web/out` 静态服务（web/out 存在时）；`go vet ./... && go test ./...` 全绿
- [ ] 真实网关冒烟：起服务 → next build 静态托管 → 浏览器一键检测真实 key → 四项全出数（注入≈已知、归因合理、thinking 判定对、IQ 出分、历史进侧栏）
- [ ] README：使用说明 + 截图占位
- [ ] commit `chore: integration, embed static web, e2e smoke` → push main → tag v0.1.0

---

## 子代理分派矩阵

| 子代理 | worktree | Tasks | 前置 |
|---|---|---|---|
| A | dev-inject | 1, 2 | 无（go.mod 已在 main） |
| B | dev-thinking | 3 | 无 |
| C | dev-history | 4 | 无 |
| D | dev-iq | 5, 6, 7 | 无 |
| E | dev-fingerprint | 8, 9, 10 | 无（data_ref 已在 main 提交） |
| F | dev-web | 11, 12 | Task A 的 gateway 接口签名（给它 client.go 的接口描述即可，不必等实现） |
| 主会话 | main+dev | 0, 13 | 全部完成后 |

**纪律（写给每个子代理）：**
1. 只动自己 worktree 的拥有文件；**绝不动其它目录**
2. 每完成一个 Task：`go test ./internal/<pkg>/ -v` 全绿 → `git add <该pkg> && git commit -m "..."` → push
3. TDD 顺序：先写失败测试、跑确认失败、再实现、再跑通过
4. 禁止在前端写任何鉴定逻辑；禁止在仓库出现广告/推荐站点/定价 UI；UA 常量 `claude-cli/2.1.284 (external, claude-desktop-3p, agent-sdk/0.3.284)`
5. 错误隔离：任何探测失败都要在结果 JSON 里带 reason，不许整任务 500
6. 提交信息格式：`feat(<pkg>): <行为描述>`
