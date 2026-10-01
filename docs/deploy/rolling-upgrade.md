# 滚动升级与 Job 操作密钥

## 升级顺序

单实例 SQLite 不做两个 worker 实例同时抢同一队列。上线时先排空旧实例，再切换二进制，避免正在运行的分享链接被中断：

1. 确认新包已通过 `checksums.txt` 校验，放入新的版本目录，不覆盖当前目录。
2. 对当前服务进程发送 `SIGUSR1`，进入 draining。此时 `/api/detect` 返回 `503 + Retry-After`，已有 queued/running Job 继续运行。
3. 轮询 `GET http://127.0.0.1:8770/api/health`，直到 `queued_jobs=0` 且 `active_jobs=0`。同时确认 `GET /api/ready` 返回 `503`，证明不会再接收新 Job。
4. 停止旧服务，切换 `/opt/chanshui-current` 到新版本目录，启动新服务；新服务默认会把异常中断的残留 Job 标为 `interrupted`，但不会碰已排空任务。
5. 依次验证本机 `/api/health`、nginx/Cloudflare `/api/ready`、首页和 `/history`，确认健康后恢复入口。

`TimeoutStopSec=1300` 是 systemd 停止服务时的保护窗口，不替代任务级总耗时预算。默认每个上游请求超时360秒、每个请求独立最多重试3次；任务总耗时由探针波次、并发等待和4次尝试预算共同决定。不要直接 `systemctl restart` 代替排空流程。

## 操作密钥边界

创建 Job 的 `202` 响应一次性返回随机 128 bit `control_token`。服务端只保存 SHA-256 摘要；报告、历史、分享 URL、日志和普通 GET 响应都不包含明文 token。浏览器按 `job_id` 将该操作key持久化到本地，刷新页面仍可恢复；Job 全部探针完成或24小时到期后删除对应记录。

失败项重试必须使用对应Job的 `X-Job-Token` 请求头，比较采用常量时间；操作key不能用于其他Job、停止接口或读取权限，公开 Job ID 只有只读权限。重试仍要求重新提交 API Key，服务端不会把 API Key 写入数据库、浏览器或 URL；操作key本身也不进URL、分享链接、报告和日志。

生产入口应保持 HTTPS、`/api/*` `no-store`，Cloudflare 对 API 路径绕过缓存；公开读取接口用 ETag 返回 304 降低重复包体；不要把 `X-Job-Token` 放进查询参数或分享链接。
