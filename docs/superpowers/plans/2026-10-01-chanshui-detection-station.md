# 掺水？检测站 实施计划

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 单页检测站「掺水？」——填 URL/Key/模型名一键检测：注入 token 数、模型家族归因、summarized 思考文本回传、IQ 评分（满分 100），历史记录存 SQLite；所有鉴定逻辑在 Go 后端，前端只做展示。

**Architecture:** Go 单二进制（net/http + embed 前端 + SQLite）。前端一个页面：顶部输入卡片（URL/Key/模型 → 一键检测）+ 左侧边栏历史记录 + 右侧结果区。后端按模块拆包：probe（HTTP 客户端）、inject（注入测量）、fingerprint（归因）、thinking（思考文本检查）、iq（题库+评分）、history（SQLite）。前端不出现任何鉴定逻辑，只发 `/api/detect` 收结构化结果。

**Tech Stack:** Go 1.22+（net/http、embed、database/sql + modernc.org/sqlite 纯 Go 驱动）、原生 HTML/CSS/JS（无构建步骤）。

**参考项目精华（已实地核对，抄的是方法不是文件）：**
- ModelTrace（xqy2006）：三发长整数挑战（1..355，292–333 个）→ 解析数字序列 → Hellinger 特征 + 有序块特征 → 与指纹库比对 → softmax 概率 + 家族聚合。库里有 GPT 5.4–6、Claude haiku4.5–opus5.5 共 16 模型。**移植其 challenge 文案与评分公式，指纹库先内置 ModelTrace 的 unified_bank.json（下载后转 Go 内嵌 JSON）。**
- iqbench（yclenove）：评分公式 `score = 100 × 加权通过率`（满分 100，比 IQ 55–145 直观）；题库取 **Q1 安眠药、Q4 红绿色盲（参数化 38→88 随机）、Q6 经典糖果题、Q8 糖果题变体（参数化库存数字）、Q7 袜子配对（参数化）**，参数化题每次换实例、答案由求解器现算；判分规则照抄 spec（含半分与「替代建模/背原题」标记）。
- LLMprobe-engine（Bazaarlink）：**token 膨胀侦测探针**思路（prompt_tokens 与实际内容 token 数对比）→ 我们的注入测量升级为双通道：回包 usage 差分回归（主）+ 内容长度外推（对照）；**缓存侦测**（x-cache 头）与 **SSE 流格式**探针作为附加诊断字段；探针逐条出结果行的 CLI 报告样式。

**UI 参考：** cctest.ai 的"输入卡片 + 一键开始 + 结果徽章"式首屏；manxue.ai 的左侧固定侧边栏（历史记录列表）；无任何广告/推荐站点位。

---

## 文件结构

```
chanzhushui/
├── go.mod                     # module chanzhushui, go 1.22
├── main.go                    # 入口：路由注册、embed 前端、启动
├── internal/
│   ├── gateway/client.go      # 网关 HTTP 客户端（/v1/messages 封装，UA、超时、代理）
│   ├── gateway/client_test.go
│   ├── inject/inject.go       # 注入 token 回归测量（5 点小区间）
│   ├── inject/inject_test.go
│   ├── fingerprint/bank.go    # 指纹库加载（内嵌 JSON）
│   ├── fingerprint/feature.go # 数字解析/Hellinger/有序块特征
│   ├── fingerprint/feature_test.go
│   ├── fingerprint/attr.go    # 归因评分 + 家族聚合
│   ├── fingerprint/attr_test.go
│   ├── thinking/check.go      # summarized 思考文本回传检查
│   ├── thinking/thinking_test.go
│   ├── iq/spec.go             # 题库结构 + 参数化生成器（色盲/糖果变体/袜子）
│   ├── iq/spec_test.go
│   ├── iq/grade.go            # 判分器（关键词/数值等价/半分）
│   ├── iq/iq_test.go
│   ├── iq/score.go            # 加权汇总 → 100 分制
│   ├── api/handlers.go        # /api/detect、/api/history、/api/detect/{id}
│   ├── api/handlers_test.go
│   └── history/store.go       # SQLite 历史（表 + 增查）
│   └── history/store_test.go
├── data/fingerprint_bank.json # 从 ModelTrace unified_bank.json 转换
├── web/
│   ├── index.html             # 单页：输入卡片 + 一键检测
│   ├── app.js                 # 仅 fetch + 渲染，零鉴定逻辑
│   └── style.css              # cctest 风格（卡片、徽章、无广告）
└── docs/superpowers/plans/…   # 本计划
```

**worktree 分工（多模块并行）：**

| worktree | 分支 | 模块 | 无共享文件冲突 |
|---|---|---|---|
| main | main | 仓库骨架、路由、embed、CI | — |
| wt-inject | wt-inject | inject 包 + gateway 包 | 只碰 internal/inject、internal/gateway |
| wt-fingerprint | wt-fingerprint | fingerprint 包 + bank 转换 | 只碰 internal/fingerprint、data/ |
| wt-thinking | wt-thinking | thinking 包 | 只碰 internal/thinking |
| wt-iq | wt-iq | iq 包（题库+判分+评分） | 只碰 internal/iq |
| wt-web | wt-web | 前端三件 + api handlers 骨架 | 只碰 web/、internal/api |
| wt-history | wt-history | history 包 | 只碰 internal/history |

各 worktree 完成即 commit + push，main 上逐个 merge（文件不重叠，无冲突）。收口（main）：路由注册全模块 + embed + 端到端验证 + tag v0.1.0。

---

## Chunk 1: 仓库骨架 + Go 后端核心

### Task 1: 仓库骨架与 Go module

**Files:**
- Create: `go.mod`, `main.go`, `.gitignore`

- [ ] **Step 1: 写 go.mod**

```
module chanzhushui
go 1.22
require modernc.org/sqlite v1.29.10
```

（`go mod tidy` 会补齐间接依赖。）

- [ ] **Step 2: 写 .gitignore**

```
*.exe
chanzhushui
chanzhushui.exe
*.db
```

- [ ] **Step 3: main.go 骨架（暂只起 :8777 HTTP 服务 + /healthz）**

```go
package main

import (
	"net/http"
	"log"
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
	http.ListenAndServe("127.0.0.1:8770", mux)
}
```

- [ ] **Step 4: 构建验证** `go build ./...` → 无错误
- [ ] **Step 5: Commit** `git add -A && git commit -m "chore: repo skeleton, go module, healthz"`

### Task 2: gateway 包（网关客户端）

**Files:**
- Create: `internal/gateway/client.go`, `internal/gateway/client_test.go`

- [ ] **Step 1: 写失败测试**（httptest 假网关，验证 headers/body 解析）

```go
func TestCallMessages_ParsesUsageAndBlocks(t *testing.T) {
	// httptest server 回固定 JSON（含 usage.output_tokens_details.thinking_tokens、
	// content[].type=thinking 带 thinking/signature）
	// 断言: Call 返回的 Metrics.InputTokens/ThinkingTokens/ThinkLen/SigLen 正确
	//       请求头带 x-api-key / anthropic-version / user-agent
}
```

- [ ] **Step 2: 跑测试确认失败** `go test ./internal/gateway/ -v` → FAIL
- [ ] **Step 3: 实现 client.go**

结构：`Client{BaseURL, Key, HTTP *http.Client}`；方法 `CallMessages(ctx, CallRequest) (Metrics, error)`。Metrics 字段：Status、Ms、Err、Answer、Blocks、StopReason、InputTokens、OutputTokens、ThinkingTokens、CacheCreation5m、CacheCreation1h、CacheRead、ThinkLen、SigLen、ThinkingText、Signature、Via（direct/proxy）。默认 UA 常量 `claude-cli/2.1.284 (external, claude-desktop-3p, agent-sdk/0.3.284)`（可被调用方覆盖）。thinking 参数由调用方给 `*ThinkingConfig`（nil=不传），与 cache_control system 数组支持。

- [ ] **Step 4: 跑测试通过** `go test ./internal/gateway/ -v` → PASS
- [ ] **Step 5: Commit** `feat(gateway): messages client with full usage/block metrics`

### Task 3: inject 包（注入测量）

**Files:**
- Create: `internal/inject/inject.go`, `internal/inject/inject_test.go`

- [ ] **Step 1: 写失败测试**（mock 网关：input = n×0.74 + 44 → 回归截距 ≈44，斜率 ≈0.74）

```go
func TestMeasure_RecoversInterceptAndSlope(t *testing.T) {
	// 5 点 (1/200/500/800/1000)，mock 服务器按 0.738*n+44 计算 input_tokens
	// 断言: Inj 在 43..45，Slope 在 0.735..0.741
}
func TestMeasure_TreatsCacheAsTotal(t *testing.T) {
	// 某点回 cache_creation>0，total=input+cc+cr 口径仍线性
}
```

- [ ] **Step 2: 跑测试失败** → FAIL
- [ ] **Step 3: 实现**：`Measure(ctx, gw gateway.Client, model string) (Result, error)`；5 发递增 A 串（1/200/500/800/1000 字符）；每点 total=input+cc5m+cc1h+cr；最小二乘回归；429 时 30s 退避重试×4；返回 Inj（round(intercept)）、Slope、Points。
- [ ] **Step 4: 测试通过** → PASS
- [ ] **Step 5: Commit** `feat(inject): regression-based injection token measurement`

### Task 4: thinking 包（summarized 回传检查）

**Files:**
- Create: `internal/thinking/thinking.go`, `internal/thinking/thinking_test.go`

- [ ] **Step 1: 失败测试**：假网关回 (a) thinking_text 有内容+签名 (b) 只签名无文本 (c) 无 thinking block → Check 分别判 Pass / Fail("只回签名") / Fail("无思考块")

- [ ] **Step 2: 失败** → FAIL
- [ ] **Step 3: 实现** `Check(ctx, gw, model) (Verdict, error)`：发 adaptive+display=summarized 一发；verdict 字段 Pass/Fail、ThinkLen、SigLen、Reason（人话）。**判据：ThinkLen>0 即 Pass；ThinkLen==0 && SigLen>0 → Fail 文案"思考被清空，只回签名"**。
- [ ] **Step 4: 通过** → PASS
- [ ] **Step 5: Commit** `feat(thinking): summarized thinking-text passthrough check`

### Task 5: history 包（SQLite）

**Files:**
- Create: `internal/history/store.go`, `internal/history/store_test.go`

- [ ] **Step 1: 失败测试**：Open 内存库 → Save 一条 → List 返回含该条 → Get(id) 字段完整 → List 按时间倒序
- [ ] **Step 2: 失败** → FAIL
- [ ] **Step 3: 实现**：表 `detections(id INTEGER PK, created_at TEXT, base_url TEXT, model TEXT, injection INTEGER, family TEXT, family_prob REAL, top_model TEXT, top_prob REAL, thinking_pass INTEGER, thinking_note TEXT, iq_score INTEGER, verdict_json TEXT)`；Verdict 整体 JSON 存 `verdict_json`。modernc.org/sqlite 纯 Go 驱动（免 CGO）。
- [ ] **Step 4: 通过** → PASS
- [ ] **Step 5: Commit** `feat(history): sqlite detection history store`

## Chunk 2: 指纹归因模块

### Task 6: 下载并转换 ModelTrace 指纹库

**Files:**
- Create: `data/fingerprint_bank.json`, `scripts/fetch_bank.ps1`

- [ ] **Step 1: 下载 unified_bank.json**（`https://raw.githubusercontent.com/xqy2006/ModelTrace/main/codex-plugin/modeltrace-guard/assets/unified_bank.json`），存 `data/raw_unified_bank.json`
- [ ] **Step 2: 写转换脚本**：抽 models[]（id/display_name/family/counts）、robust（hellinger/ordered_blocks）、calibration → 扁平化成我们自己的 bank 格式（字段语义 1:1 保留，只换文件组织）。**不引入 numpy 依赖：JSON 里存的是已算好的矩阵，Go 侧只需线性代数（点积/矩阵乘），转换只是格式整理。**
- [ ] **Step 3: 校验**：转换后 JSON 可被 json.Unmarshal 到 Task 7 的 Bank 结构，16 模型、2 家族
- [ ] **Step 4: Commit** `feat(data): port ModelTrace unified fingerprint bank`

### Task 7: fingerprint 特征与归因

**Files:**
- Create: `internal/fingerprint/feature.go`, `internal/fingerprint/attr.go`, `internal/fingerprint/feature_test.go`, `internal/fingerprint/attr_test.go`

- [ ] **Step 1: 失败测试 feature**：
  - `ParseNumbers("3, 17, 42 100")` → [3,17,42,100]；356/0 被剔除；跨字母断 run 取最长
  - `HellingerFeature(counts)` L2 归一验证
  - `OrderedBlockFeature` 维度 = 4×16+10=74
- [ ] **Step 2: 失败** → FAIL
- [ ] **Step 3: 实现 feature.go**：照抄 ref_fingerprint.py 的 parse_numbers/count_numbers/hellinger_feature/ordered_block_feature 语义（Go 手写，不依赖 numpy——矩阵运算用标准库实现小工具函数 matVec/vecDot）
- [ ] **Step 4: feature 测试通过** → PASS
- [ ] **Step 5: 失败测试 attr**：用 bank 里某模型 self-consistency——造一组「该模型典型分布」的假 counts（从 bank 的 counts 直接采样 200 个数）→ 归因应回到该模型或同家族（概率 top2 内）
- [ ] **Step 6: 失败** → FAIL
- [ ] **Step 7: 实现 attr.go**：robust_score_numbers 语义移植（hellinger 投影去 nuisance + ordered blocks + 0.75/0.25 融合 + 全局 softmax + 家族聚合）；输出 `Attribution{Family, FamilyName, FamilyProb, TopModel, TopModelProb, Results []{Model,Prob}, UsedOutputs}`
- [ ] **Step 8: attr 测试通过** → PASS
- [ ] **Step 9: Commit** `feat(fingerprint): hellinger+ordered-block attribution ported from ModelTrace`

### Task 8: challenge 生成

**Files:**
- Modify: `internal/fingerprint/attr.go`（加 GenerateChallenges）
- Test: `internal/fingerprint/attr_test.go`

- [ ] **Step 1: 失败测试**：GenerateChallenges(3) → 3 条，prompt 含「1 到 355」「355」数量在 292..333，expected_count 与 prompt 中数字一致
- [ ] **Step 2: 失败** → FAIL
- [ ] **Step 3: 实现**：照抄 ref_fingerprint.py generate_challenges 文案（开句/动作/结尾/分隔随机组合）
- [ ] **Step 4: 通过** → PASS
- [ ] **Step 5: Commit** `feat(fingerprint): long-integer challenge generator`

## Chunk 3: IQ 题库与评分

### Task 9: iq 题库结构与参数化生成器

**Files:**
- Create: `internal/iq/spec.go`, `internal/iq/spec_test.go`

- [ ] **Step 1: 失败测试**：
  - `GenColorBlind()`：女儿色盲+父亲色觉正常+伴X隐性随机素材 → 每次调用生成不同父辈表型数字（38↔88 位置随机）→ 正确结论恒为「非亲生」
  - `GenCandy()`：随机库存（每类 4..10）→ `SolveCandy(stock)` 用最坏保证求解器算出 (圆取, 星取, 总数)；同参数两次生成可不同
  - `GenSocks()`：3-4 色 × 2-7 只 × 目标 2-4 双 → `SolveSocks` 穷举最坏情况，且**只出真实答案 ≠ 2p+c-1 公式值的实例**（库存约束生效）
- [ ] **Step 2: 失败** → FAIL
- [ ] **Step 3: 实现 spec.go**：题目模板 + 求解器
  - Candy 求解器：对「(形状取法只辨形状) 最坏保证 (圆苹∧星桃)∨(圆桃∧星苹)」逐类枚举堵住四类对手（照抄 iqbench spec Q8 推导逻辑，泛化到任意库存数字）
  - Socks 求解器：对每种「抽 k 只保证 p 双同色」穷举库存组合最坏值
  - 色盲题：模板「父亲说数字 8 的[左半/右半]一半[红/绿]一半[红/绿]，随后崩溃/说是 88 不是 38」随机数字对（38/88、27/72、45/54 中选）——答案恒定「非亲生」（遗传链固定），判分点在关键词
- [ ] **Step 4: 通过** → PASS
- [ ] **Step 5: Commit** `feat(iq): parameterized question generators with solvers`

### Task 10: 静态题 + 判分器

**Files:**
- Create: `internal/iq/judge.go`, `internal/iq/judge_test.go`

- [ ] **Step 1: 失败测试**：Q1 安眠药（命中「不叫醒/不喂/暂缓给药」且无强制叫醒）；Q6 经典糖果题（21 满分、29 半分标替代建模、其它 0）；色盲题（「非亲生」满分 14、「不是亲生」也中、只答遗传链 7 分）；糖果变体（命中求解器总数满分、公式值 2p+c-1 半分、答经典 21 标疑似套用）
- [ ] **Step 2: 失败** → FAIL
- [ ] **Step 3: 实现 judge.go**：答案抽取（去 Markdown → 最终答案槽：`最终答案/final answer/the answer is` → 正文空用思维链末尾 2000 字 → 数值等价匹配 0.10==0.1）+ 每题 keyword 正则
- [ ] **Step 4: 通过** → PASS
- [ ] **Step 5: Commit** `feat(iq): answer extraction and per-question judges`

### Task 11: 加权评分汇总（100 分制）

**Files:**
- Create: `internal/iq/score.go`, `internal/iq/score_test.go`

- [ ] **Step 1: 失败测试**：5 题（安眠药 6、色盲 14、糖果变体 16、袜子 16、经典糖果 6 = 卷面 58）全对 → 总分 `round(58/58×100)=100`；袜子答公式值半分 8 → 加权 96 左右；标签集合正确传递
- [ ] **Step 2: 失败** → FAIL
- [ ] **Step 3: 实现 score.go**：`score = round(100 × Σ earned/Σ max)`，只计通过题进分子（半分进卷面标记、不进总分分子，遵循 iqbench「半分不进 IQ」）；输出 `IQResult{Score, Max=100, PerQuestion []{ID,Title,Weight,Max,Earned,Half,Tags}, PassAll}`
- [ ] **Step 4: 通过** → PASS
- [ ] **Step 5: Commit** `feat(iq): weighted 0-100 scoring with half-credit tags`

## Chunk 4: API + 前端 + 收口

### Task 12: api handlers

**Files:**
- Create: `internal/api/handlers.go`, `internal/api/handlers_test.go`

- [ ] **Step 1: 失败测试**：
  - `POST /api/detect` {base_url, key, model} → 202 {task_id}；后台跑四模块（inject/thinking/fingerprint 3 发/iq 5 题）；`GET /api/detect/{id}` 完成后返回完整 verdict JSON（四项 + 总评）；Key 不落盘不记日志
  - `GET /api/history` → 最近 50 条摘要列表
  - `GET /api/history/{id}` → 单条详情
- [ ] **Step 2: 失败** → FAIL
- [ ] **Step 3: 实现**：内存任务表 + goroutine 执行器（并发 4 模块并行，inject 与 fingerprint 并行、iq 5 题并发）；结果聚合写 history；错误逐模块隔离（一个模块挂不影响其它三项出结果）
- [ ] **Step 4: 通过** → PASS
- [ ] **Step 5: Commit** `feat(api): detect orchestration and history endpoints`

### Task 13: 前端（cctest 风格 + manxue 侧边栏）

**Files:**
- Create: `web/index.html`, `web/app.js`, `web/style.css`

- [ ] **Step 1: index.html**：左侧固定侧边栏（logo「掺水？」+ 历史记录列表，点击加载详情）；主区顶部一张输入卡片（URL/Key/模型 三输入 + 大按钮「一键检测」）；下方结果区四张卡片：注入 token（大数字 + 说明）、模型归因（家族徽章 + top 模型概率条）、思考文本（Pass/Fail 徽章 + 签名/文本长度）、IQ 评分（100 分制大分数 + 每题展开）；无任何广告位、无推荐站点、无定价页
- [ ] **Step 2: app.js**：只做三件事——读输入发 POST /api/detect、轮询 GET /api/detect/{id} 渲染结果、侧边栏拉 /api/history。**零鉴定逻辑**（所有判分/回归/归因都在后端）
- [ ] **Step 3: style.css**：深色侧边栏 + 浅色主区、卡片圆角阴影、结果徽章（绿 Pass/红 Fail/黄 半分）、等宽数字；无 emoji、无广告样式
- [ ] **Step 4: main.go 加 embed**：`//go:embed web` → `/` 静态服务；`go build` 后 `curl localhost:8770/` 返回 HTML
- [ ] **Step 5: 浏览器手验**：起服务 → 打开页面 → 填假 key 点检测 → 看到错误态渲染正常
- [ ] **Step 6: Commit** `feat(web): single-page UI with sidebar history, zero frontend logic`

### Task 14: 端到端联调 + 真实网关验证

**Files:**
- Modify: 各处小修

- [ ] **Step 1: go build + go vet ./... 全绿**
- [ ] **Step 2: go test ./... 全绿**
- [ ] **Step 3: 真实网关冒烟**：用测试 key 打一次真实检测（kedaya 或任一可用网关），四项结果都出数：注入≈已知值、家族归因合理、thinking 判定正确、IQ 出分
- [ ] **Step 4: README 更新使用说明**
- [ ] **Step 5: Commit** `chore: e2e smoke and docs`
- [ ] **Step 6: push main → tag v0.1.0 → GitHub Private 仓库 kiss-kedaya/chanzhushui**

---

## 执行说明（给子代理）

- 每个 Task 一个子代理；Task 2/3/4/5 并行（worktree 隔离，文件不重叠）；Task 6→7→8 串行；Task 9/10 串行后 11；Task 12/13 收口。
- **禁改其它 worktree 的文件**。merge 由主会话做。
- 后端禁 CGO（modernc.org/sqlite）；**全项目不得出现任何广告位、推荐站点、定价 UI**。
- 前端**只**有：输入卡片、进度、结果区、侧边栏历史。其余（题目生成、判分、回归、归因）全部在 Go。
- 指纹库 JSON 是数据文件，转换脚本只整理字段不重算数值。
- 提交纪律：一个功能一个 commit，说明写清测了什么。
