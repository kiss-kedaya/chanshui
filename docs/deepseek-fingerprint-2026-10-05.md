# DeepSeek Flash 数字基线（2026-10-05）

使用用户提供的官方接口 `https://api.deepseek.com` 采集，实际 API 名称为 `deepseek-flash`。模型列表不提供 `deepseek-v4.1-flash`，直接使用该名称返回400；用户明确确认两者对应，因此将后者登记为别名。此对应关系来自用户确认，不是模型列表证明。访问官方域名时自动将该别名转换为真实 API 名称，其他网关保持用户选择的名称。

## 数据与验证

- 49次数字题采样：36个有效训练样本、12个独立验证样本、1个无效样本。
- 训练集含11227个数字，验证集含3709个数字。使用项目原有中文数字题生成器，范围1～355，题目长度292～332。
- 流式Chat Completions，保持默认推理和默认温度，max_tokens=16384，并发3。一个训练请求将预算全部用于推理且没有最终答案，予以剔除，并用独立新题补足。
- 训练与验证按请求预先划分，验证数据不参与质心构建。验证12题分为4组三题检测，4/4的首位候选均为deepseek-flash。
- 使用现有355维Hellinger及74维顺序特征的标准化、干扰投影和融合评分。单一采集环境只生成一个环境模板，在现有环境槽中共享；不声称拥有12个独立环境。
- 相对候选权重沿用原库温度参数，未作全候选或跨供应商重校准。保留测试不能证明同家族版本区分能力；旧DeepSeek行为库不等于数字对照样本。

原始采集文件：`data_ref/captures/deepseek-flash-20261005.json`，SHA256：`d9f36fb58af6f711972334f514a89caf52ede6a5df1fa83753a55c64f958e5ff`。记录题目、最终回答、结束原因、token用量、请求次数、耗时和失败样本；不保存密钥或推理全文。

## 重建与维护

采集工具从stdin或`CHANSHUI_CAPTURE_KEY`读取密钥，不接受密钥命令行参数：

```powershell
go run ./cmd/fingerprint-capture -base-url https://api.deepseek.com -model deepseek-flash -train 36 -holdout 12 -out .local/capture.json
```

重建本地基线并输出验证报告（当前工具针对DeepSeek）：

```powershell
go run ./cmd/fingerprint-reference -capture data_ref/captures/deepseek-flash-20261005.json -out internal/fingerprint/local.references.json
go test -race ./...
```

当前原库保持原样，本地参考单独嵌入；启动、加载SQLite缓存和远程库发布时都按当时的标准化参数重新投影并合并。别名不新增候选，不分割候选权重。原Python oracle继续验证原候选集的数学一致性，新基线由独立保留集测试验证。
