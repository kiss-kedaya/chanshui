# 掺水？（chanzhushui）

Claude 系网关黑盒检测站。填 URL + Key + 模型名，一键出四项结论：

1. **注入 token 数** —— 网关每次请求额外塞进去多少系统提示词（回归测量，同侧差分消 tokenizer 差异）
2. **模型家族 / 具体型号** —— 数字指纹归因（三发长整数序列 → Hellinger + 有序块融合评分，参考 ModelTrace）
3. **summarized 思考文本是否回传** —— 不能只回签名
4. **智商评分（满分 100）** —— 认知反射 / 色盲遗传 / 糖果题（含变题防背题）/ 袜子配对等加权判分，参考 iqbench

## 状态

开发中。技术栈：Go 后端（所有鉴定逻辑在服务端，前端零逻辑）+ 单页前端 + SQLite 历史记录。

实施计划：`docs/superpowers/plans/`。

## License

MIT
