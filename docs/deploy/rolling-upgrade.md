# 滚动升级与 Job 操作密钥

## 默认：双实例 + nginx 切流

升级时不再先排空。新版本先在空闲端口启动，nginx 把新请求切过去；旧实例只负责跑完手上已有的 Job，新检测从切换那一刻起就可以继续提交。

端口约定：

- 蓝：`127.0.0.1:8770`
- 绿：`127.0.0.1:8771`
- nginx 只把 `/api/` 和页面反代到当前颜色，另一个实例保持待命或排空。

仓库里的入口模板：

- `docs/deploy/chanshui-bluegreen@.service`
- `docs/deploy/chanshui-run`
- `docs/deploy/chanshui-upstream.conf`
- `docs/deploy/chanshui-switch.sh`

升级顺序：

1. 新包解到新的版本目录，校验 `checksums.txt`，不覆盖当前目录。
2. 在空闲端口启动新版本；模板 wrapper 会在另一端口健康时自动加 `-skip-recover`。否则新实例启动时会把旧实例正在跑的 Job 标成 `interrupted`。两个实例共用 `/opt/chanshui-data/jobs.db`，SQLite 继续使用 WAL 和 `busy_timeout`。
3. 检查新端口的 `/api/health`、`/api/ready` 和首页；确认 `draining=false`、`queued_jobs=0`，并能提交一条新检测。
4. 修改 nginx 的 upstream/`proxy_pass` 指向新端口，`nginx -t` 后 `systemctl reload nginx`。reload 只影响新连接，旧连接继续走旧实例。
5. 观察旧实例 `/api/health`，等 `active_jobs=0` 后停止旧实例；不要给旧实例发 `SIGUSR1`，它已经不再接收新请求。
6. 更新 `/opt/chanshui-current` 指向新版本。下一次升级反向使用另一个端口。

具体命令示例（当前绿在 8771，新版本切到 8770）：

```bash
# 1. 启动新的蓝色实例（root-only admin.env 已存在）
sudo tee /etc/chanshui/bluegreen-next2.env >/dev/null <<'EOF'
CHANSHUI_ROOT=/opt/chanshui-0.1.46
CHANSHUI_PORT=8770
CHANSHUI_OTHER_PORT=8771
EOF
sudo systemctl start chanshui-bluegreen@next2
curl -fsS http://127.0.0.1:8770/api/ready

# 2. 切流并等绿色实例跑完
sudo /usr/local/bin/chanshui-switch 8770 chanshui-bluegreen@next2 chanshui-bluegreen@next

# 3. 绿色停稳后再更新 current
sudo ln -sfn /opt/chanshui-0.1.46 /opt/chanshui-current
```

下一次升级再反向：用 `@next3` 和 `8771`，切流脚本参数改成 `8771 chanshui-bluegreen@next3 chanshui-bluegreen@next2`。不要直接用 `chanshui.service` 做备用实例，它的 `ExecStart` 没有 `-skip-recover`；模板 unit 的 wrapper 才会根据另一端口是否健康自动加这个参数。

回滚：nginx 切回旧端口即可；旧实例仍在时不再启动第三份。旧实例已经停止时，先按旧版本目录在旧端口启动，再切回。

注意事项：

- 过渡期最多两个实例；不要让两个实例同时执行同一个内存队列。
- 旧实例的 Job 不在新实例内存里，但报告读取和失败重试会回落到 SQLite；切换后不要依赖旧实例的停止接口。
- 公共接口的每分钟限速和公益计数在进程内，过渡期两个实例各自计数；正式长跑前应把限速移到共享存储。
- 两个实例都会做模型库自动同步；过渡期可接受，后续可加 `-no-sync` 让备用实例只读。
- `TimeoutStopSec=1300` 仍是停止旧实例时的保护窗口。默认每个上游请求超时 360 秒、每个请求独立最多重试 3 次。

## 兜底：单实例排空切换

只有在新实例无法启动、或数据库锁无法接受时，才退回旧流程：

1. 对当前服务进程发送 `SIGUSR1`，进入 draining。
2. 轮询 `/api/health`，直到 `queued_jobs=0` 且 `active_jobs=0`；确认 `/api/ready` 为 `503`。
3. 停止旧服务，切换 `/opt/chanshui-current`，启动新服务。
4. 验证 `/api/health`、`/api/ready`、首页和 `/history`。

## 操作密钥边界

创建 Job 的 `202` 响应一次性返回随机 128 bit `control_token`。服务端只保存 SHA-256 摘要；报告、历史、分享 URL、日志和普通 GET 响应都不包含明文 token。浏览器按 `job_id` 将该操作key持久化到本地，刷新页面仍可恢复；Job 全部探针完成或24小时到期后删除对应记录。

失败项重试必须使用对应Job的 `X-Job-Token` 请求头，比较采用常量时间；操作key不能用于其他Job、停止接口或读取权限，公开 Job ID 只有只读权限。重试仍要求重新提交 API Key，服务端不会把 API Key 写入数据库、浏览器或 URL；操作key本身也不进URL、分享链接、报告和日志。

生产入口保持 HTTPS。运行中、排队中和可重试的报告使用 `no-store`；已完成且没有失败项的报告使用 `private, max-age=300, stale-while-revalidate=60`，由浏览器复用。公开读取接口继续用 ETag 返回 304；不要把 `X-Job-Token` 放进查询参数或分享链接。
