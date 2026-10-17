# 掺水？ · 模型检测站

> Self-hosted audit station for LLM gateways and resellers. Check whether a claimed model is actually served, and collect protocol, cache, tool, web, thinking, knowledge-cutoff, IQ and fingerprint evidence.

一个可自托管的模型中转站审计工具。填入网关地址、API Key 和模型名，提交一次检测，得到一份带证据链的报告。Go 服务端 + Next.js 静态前端 + SQLite，单机即可运行。

示例部署：<https://chanshui.dev>（不依赖该站点即可自托管）

源码：<https://github.com/kiss-kedaya/chanshui>

## 能查什么

- **模型指纹归因**：数字挑战 + 行为参考，比较声称模型与实际响应的候选；置信度不足时明确显示“证据不足”，不伪造结论。
- **注入大小**：两次独立小请求估算协议包装和上下文注入量。
- **缓存读写**：可选的串行多轮对话，核对缓存读/写衔接、命中率和末轮缓存率；缺字段显示未知，不把缺失当 0。
- **工具调用**：核对结构化调用、`tool_use_id`、参数和结果回传，不执行任意外部工具。
- **联网证据**：DeepSeek 走 `POST /v1/tools/search`；Messages/Responses 走原生搜索工具，只认实际搜索执行和来源 URL。
- **思维链回传**：仅 Claude 前缀模型检测摘要/全文/签名是否回传，只占思维链板块 5 分。
- **知识截止**：用可验证的时事问题串行二分，只展示模型自报或探测结果，不混入 IQ。
- **推理表现**：Q1～Q8 可验证题型，报告保留评分版本，旧报告不重算。
- **接口完整性**：独立统计协议、缓存、工具、联网、流式和思维链证据，未知项显示覆盖率。

## 架构

```text
Browser / Agent
      │
      ▼
Go HTTP server ── SQLite (jobs, reports, feedback)
      │
      ├── Next.js static export in web/out-build
      ├── /api/v1 public audit API
      └── upstream gateway (Chat / Messages / Responses / DeepSeek Search)
```

- 单个 Go 二进制同时托管静态页面和 API。
- API Key 只在当前请求中使用，不写入数据库、报告、日志或分享链接。
- Job ID 即分享 ID；公开报告只读，失败项重试需要创建时返回的一次性操作 key。
- 报告默认保留 24 小时；反馈写入私有表，不跟随报告清理。
- 单实例 SQLite 是默认部署；升级使用双实例 + nginx 切流，见 `docs/deploy/rolling-upgrade.md`。

## 快速开始

要求：Go 1.22+、Node.js 20/22、npm。

### Windows

```powershell
./scripts/start-local.ps1
```

打开 <http://127.0.0.1:8770>。已有构建可加 `-SkipBuild`；停止用 `scripts/stop-local.ps1`。

### 手动运行

```bash
cd web
npm ci
npm run build
cd ..

export CHANSHUI_ADMIN_TOKEN="$(openssl rand -hex 24)"
go run . \
  -addr 127.0.0.1:8770 \
  -web web/out-build \
  -db .local/history.db
```

打开 <http://127.0.0.1:8770>。反馈管理在 `/admin`，登录时使用同一个 `CHANSHUI_ADMIN_TOKEN`。

### Docker

准备 `.env`：

```bash
cp .env.example .env
# 填入：CHANSHUI_ADMIN_TOKEN=$(openssl rand -hex 24)
docker compose up -d --build
```

默认只绑定 `127.0.0.1:8770`，由外层 nginx/Caddy 提供 TLS；数据在 `chanshui-data` 卷。不要把 `8770` 直接暴露公网。

### 反馈管理

- 浏览器打开 `/admin`，输入 `CHANSHUI_ADMIN_TOKEN`。
- 页面支持刷新、退出和导出当前反馈 JSON；导出文件含联系方式，按敏感文件保管。
- 也可以离线导出：

```bash
feedback-export \
  -db ./data/jobs.db \
  -out ./private-feedback.json
```

`feedback-export` 只创建新文件（`O_EXCL`，权限 0600）；不要把导出文件放进静态 Web 目录，也不要提交到 Git。

## 运行参数

| 参数 | 默认值 | 说明 |
| --- | --- | --- |
| `-addr` | `127.0.0.1:8770` | 监听地址；生产环境保持回环，由 nginx 反代 |
| `-db` | `.local/history.db` | SQLite 文件；目录必须可写并持久化 |
| `-web` | `web/out-build` | Next.js 静态导出目录 |
| `-workers` | `16` | 同时执行的检测 Job 上限 |
| `-queue` | `128` | 有界等待队列 |
| `-upstream-concurrency` | `64` | 全局上游请求并发上限 |
| `-skip-recover` | `false` | 双实例切换时使用，避免把另一实例的 Job 标成中断 |
| `-allow-private` | `false` | 仅本地测试；允许私网上游，生产不要打开 |

环境变量：

| 变量 | 用途 |
| --- | --- |
| `CHANSHUI_ADMIN_TOKEN` | `/admin` 反馈管理口令；通常为 48 位 hex，可用 `openssl rand -hex 24` 生成 |
| `CHANSHUI_CAPTURE_KEY` | 仅 `cmd/fingerprint-capture` 使用；也可从 stdin 读取 |

`.env.example` 是配置模板；`.env*` 默认不进入 Git。

## 检测与数据边界

- 注入大小是含协议包装的近似值，不展示虚假的线性置信。
- 数字指纹只对已建立的候选库比较；DeepSeek/Grok 等基线是采集时的版本参考，不是官方型号身份的独立证明。
- 缓存命中受上游 TTL、路由和缓存策略影响；缺字段、空结果和重试导致的不可靠样本不会被伪判为通过。
- 公共 API 结果不写入全站历史，只在内存保留 24 小时，重启即丢。
- 站点只应测试你拥有或已获授权的网关。模型名称和第三方数据归各自权利人所有。

更细的采集、评分和协议说明见 `docs/`，公共 API 规范见 `/docs` 和 `GET /api/v1/openapi.json`。

## 公共 API

`POST /api/v1/audits` 面向 Agent 和脚本调用：

- 每个客户端 IP 每分钟 1 次，轮询不计数。
- `sections` 可省略或传 `all`，也可以只选要测的板块。
- 结果不进入全站历史；只选思维链或非 Claude 模型时按实际适用板块处理。
- 文档页：`/docs`；OpenAPI：`/api/v1/openapi.json`。

## 测试与构建

```bash
go test -race ./...
go vet ./...

cd web
npm ci
npm test
npm run typecheck
npm run build
```

Linux 部署包：

```powershell
./scripts/build-server.ps1
```

只构建到 `.artifacts/`，不上传、不部署。发布包和校验说明见 `docs/server-preparation.md`。

## 部署

- `docs/deploy/chanshui.service`：单实例 systemd 模板。
- `docs/deploy/chanshui-bluegreen@.service`、`chanshui-run`、`chanshui-switch.sh`：双实例 + nginx 切流。
- `docs/deploy/chanshui.nginx.http`：nginx 反代模板。
- `docs/deploy/rolling-upgrade.md`：排空与切流顺序。

生产至少要做：TLS、反代 Host、`/api` 不进 SPA 兜底、入口限速/WAF、独立数据目录、管理员口令只放在环境文件、不要把 SQLite/反馈导出放进 Web 根目录。

## 安全与隐私

细节见 `SECURITY.md`。重点：

- 上游 API Key 不落盘、不进报告、不进日志；操作 key 只保存在创建者浏览器，服务端只存摘要。
- 反馈联系方式只通过 `/admin` 或离线导出读取，没有公开反馈接口。
- 默认报告 24 小时过期；过期清理不删除反馈。
- 发现安全问题请走 GitHub Security Advisories，不要开公开 Issue。

## 数据与第三方说明

数字/行为基线包含 ModelTrace 数据和本地采集样本；DeepSeek/Grok 基线有独立采集记录。第三方模型名称、数据和依赖分别遵循其自身条款，详见 `THIRD_PARTY_NOTICES.md`。

## 贡献

见 `CONTRIBUTING.md`。提交前请跑完整测试，不要提交密钥、数据库、反馈导出或本地 `.env`。准备公开仓库前按 `docs/open-source-checklist.md` 检查许可证、Git 历史、Security 设置和 release 资产。

## License

代码默认按 MIT 发布，见 `LICENSE`。第三方数据、模型名称和依赖不自动适用 MIT，见 `THIRD_PARTY_NOTICES.md`。

