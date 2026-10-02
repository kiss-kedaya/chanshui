"use client";
import { useEffect, useMemo, useState } from "react";
import { useI18n } from "@/lib/i18n";

const sample = `{
  "base_url": "https://example.com/v1",
  "api_key": "sk-replace-me",
  "model": "claude-sonnet-5",
  "protocol": "auto",
  "timeout": 360,
  "sections": ["fingerprint", "injection"]
}`;

type Param = { name: string; type: string; required: boolean; note: string };
type Resp = { code: string; title: string; note: string; example: string; fields: Param[] };
type Item = {
  id: string;
  method?: "GET" | "POST";
  name: string;
  path?: string;
  summary: string;
  paramsLabel?: string;
  params: Param[];
  example?: string;
  responses: Resp[];
};

const items: Item[] = [
  {
    id: "guide",
    name: "约定",
    summary: "密钥放在请求体里。只限制创建。结果不进全站历史。",
    params: [
      { name: "api_key", type: "string", required: true, note: "上游密钥放在 JSON 里，不要放进 Authorization 头" },
      { name: "频率", type: "string", required: true, note: "每个 IP 每 60 秒只能创建 1 次。轮询和规范不计数" },
      { name: "sections", type: "string | array", required: false, note: "不传、all，或板块数组。all 包含缓存" },
      { name: "thinking", type: "string", required: false, note: "只适用于 claude- 前缀。全量检测会跳过其他模型；只选它会返回 400" },
      { name: "保留", type: "string", required: true, note: "内存保留 24 小时。重启后不能再查，也没有失败重试" },
      { name: "总分", type: "number", required: true, note: "只按选中板块重算到 100。知识截止日期不进总分" },
    ],
    responses: [],
  },
  {
    id: "meta",
    method: "GET",
    name: "服务信息",
    path: "/api/v1",
    summary: "看频率、板块和公益检测次数。不消耗创建额度。",
    paramsLabel: "无请求参数",
    params: [],
    responses: [
      {
        code: "200",
        title: "服务信息",
        note: "",
        example: `{
  "name": "chanshui",
  "version": "v1",
  "public_audits": 128,
  "rate_limit": {"limit": 1, "window_seconds": 60, "scope": "client_ip", "counts": "POST /api/v1/audits"},
  "sections": ["fingerprint", "injection", "cache", "tools", "web", "knowledge", "thinking", "iq"],
  "docs": "/docs",
  "openapi": "/api/v1/openapi.json",
  "history": false,
  "retention": "24h memory, dropped on restart"
}`,
        fields: [
          { name: "public_audits", type: "integer", required: true, note: "已完成的公共检测次数" },
          { name: "rate_limit", type: "object", required: true, note: "只约束创建请求" },
          { name: "sections", type: "string[]", required: true, note: "可检测板块，顺序固定" },
          { name: "history", type: "boolean", required: true, note: "固定为 false，不进全站历史" },
        ],
      },
      {
        code: "503",
        title: "次数暂时读不到",
        note: "",
        example: `{ "error": "公益次数暂时读不到" }`,
        fields: [{ name: "error", type: "string", required: true, note: "人类可读的错误原因" }],
      },
    ],
  },
  {
    id: "spec",
    method: "GET",
    name: "OpenAPI",
    path: "/api/v1/openapi.json",
    summary: "OpenAPI 3.1。给 Agent 读取，不消耗创建额度。",
    paramsLabel: "无请求参数",
    params: [],
    responses: [
      {
        code: "200",
        title: "OpenAPI 3.1",
        note: "",
        example: `{ "openapi": "3.1.0", "info": {"title": "掺水？公共检测 API", "version": "v1"}, "paths": {} }`,
        fields: [
          { name: "openapi", type: "string", required: true, note: "固定为 3.1.0" },
          { name: "paths", type: "object", required: true, note: "与本页四个接口一致" },
        ],
      },
    ],
  },
  {
    id: "create",
    method: "POST",
    name: "创建检测",
    path: "/api/v1/audits",
    summary: "提交一次检测。202 之后用 poll_url 轮询。",
    paramsLabel: "Body",
    params: [
      { name: "base_url", type: "string", required: true, note: "上游地址，要带协议。末尾可以有 /v1，也可以没有" },
      { name: "api_key", type: "string", required: true, note: "上游密钥。只放在这个 JSON 里" },
      { name: "model", type: "string", required: true, note: "要检测的模型名" },
      { name: "protocol", type: "string", required: false, note: "auto、chat、anthropic、openai。不传就是 auto" },
      { name: "timeout", type: "integer", required: false, note: "单请求超时，10 到 600 秒。不传是 360" },
      { name: "sections", type: "string | array", required: false, note: "不传、all，或板块数组。可选 fingerprint、injection、cache、tools、web、knowledge、thinking、iq" },
    ],
    example: sample,
    responses: [
      {
        code: "202",
        title: "已接受，去轮询",
        note: "",
        example: `{
  "id": "b6c1d0e29f4a4780a1b2c3d4e5f60718",
  "status": "queued",
  "sections": ["fingerprint", "injection"],
  "skipped": [],
  "poll_url": "/api/v1/audits/b6c1d0e29f4a4780a1b2c3d4e5f60718",
  "expires_at": 1710000000
}`,
        fields: [
          { name: "id", type: "string", required: true, note: "检测编号" },
          { name: "status", type: "string", required: true, note: "queued、running 或 done" },
          { name: "sections", type: "string[]", required: true, note: "实际会跑的板块" },
          { name: "skipped", type: "object[]", required: true, note: "被跳过的板块和原因" },
          { name: "poll_url", type: "string", required: true, note: "轮询路径" },
          { name: "expires_at", type: "integer", required: true, note: "过期时间，Unix 秒" },
        ],
      },
      {
        code: "400",
        title: "参数不合法",
        note: "",
        example: `{ "error": "至少选择一个检测板块" }`,
        fields: [{ name: "error", type: "string", required: true, note: "人类可读的错误原因" }],
      },
      {
        code: "429",
        title: "超过频率或队列已满",
        note: "响应头 Retry-After 是秒",
        example: `{ "error": "请稍后再提交" }`,
        fields: [{ name: "error", type: "string", required: true, note: "人类可读的错误原因" }],
      },
      {
        code: "503",
        title: "服务暂不接收",
        note: "平滑切换或计数失败时返回，响应头可能带 Retry-After",
        example: `{ "error": "服务正在平滑切换，当前不接收新检测" }`,
        fields: [{ name: "error", type: "string", required: true, note: "人类可读的错误原因" }],
      },
    ],
  },
  {
    id: "poll",
    method: "GET",
    name: "查询检测",
    path: "/api/v1/audits/{id}",
    summary: "用创建时返回的 id 取结果。不消耗创建额度。",
    paramsLabel: "Path",
    params: [{ name: "id", type: "string", required: true, note: "创建检测返回的检测编号" }],
    responses: [
      {
        code: "200",
        title: "当前结果",
        note: "",
        example: `{
  "id": "b6c1d0e29f4a4780a1b2c3d4e5f60718",
  "status": "done",
  "model": "claude-sonnet-5",
  "base_url": "https://example.com/v1",
  "host": "example.com",
  "created_at": 1710000000,
  "expires_at": 1710086400,
  "probes": {},
  "verdict": {}
}`,
        fields: [
          { name: "status", type: "string", required: true, note: "queued、running 或 done" },
          { name: "host", type: "string", required: true, note: "上游主机，不含路径" },
          { name: "probes", type: "object", required: true, note: "各板块结果" },
          { name: "verdict", type: "object", required: true, note: "总分和结论" },
        ],
      },
      {
        code: "404",
        title: "不存在或已丢弃",
        note: "",
        example: `{ "error": "检测不存在或已过期（有效期24小时，重启后不保留）" }`,
        fields: [{ name: "error", type: "string", required: true, note: "人类可读的错误原因" }],
      },
      {
        code: "410",
        title: "超过 24 小时",
        note: "",
        example: `{ "error": "检测已超过24小时有效期" }`,
        fields: [{ name: "error", type: "string", required: true, note: "人类可读的错误原因" }],
      },
    ],
  },
];

function ClaudeIcon() {
  return (
    <svg viewBox="0 0 256 257" aria-hidden="true">
      <path fill="#D97757" d="m50.228 170.321 50.357-28.257.843-2.463-.843-1.361h-2.462l-8.426-.518-28.775-.778-24.952-1.037-24.175-1.296-6.092-1.297L0 125.796l.583-3.759 5.12-3.434 7.324.648 16.202 1.101 24.304 1.685 17.629 1.037 26.118 2.722h4.148l.583-1.685-1.426-1.037-1.101-1.037-25.147-17.045-27.22-18.017-14.258-10.37-7.713-5.25-3.888-4.925-1.685-10.758 7-7.713 9.397.649 2.398.648 9.527 7.323 20.35 15.75L94.817 91.9l3.889 3.24 1.555-1.102.195-.777-1.75-2.917-14.453-26.118-15.425-26.572-6.87-11.018-1.814-6.61c-.648-2.723-1.102-4.991-1.102-7.778l7.972-10.823L71.42 0 82.05 1.426l4.472 3.888 6.61 15.101 10.694 23.786 16.591 32.34 4.861 9.592 2.592 8.879.973 2.722h1.685v-1.556l1.36-18.211 2.528-22.36 2.463-28.776.843-8.1 4.018-9.722 7.971-5.25 6.222 2.981 5.12 7.324-.713 4.73-3.046 19.768-5.962 30.98-3.889 20.739h2.268l2.593-2.593 10.499-13.934 17.628-22.036 7.778-8.749 9.073-9.657 5.833-4.601h11.018l8.1 12.055-3.628 12.443-11.342 14.388-9.398 12.184-13.48 18.147-8.426 14.518.778 1.166 2.01-.194 30.46-6.481 16.462-2.982 19.637-3.37 8.88 4.148.971 4.213-3.5 8.62-20.998 5.184-24.628 4.926-36.682 8.685-.454.324.519.648 16.526 1.555 7.065.389h17.304l32.21 2.398 8.426 5.574 5.055 6.805-.843 5.184-12.962 6.611-17.498-4.148-40.83-9.721-14-3.5h-1.944v1.167l11.666 11.406 21.387 19.314 26.767 24.887 1.36 6.157-3.434 4.86-3.63-.518-23.526-17.693-9.073-7.972-20.545-17.304h-1.36v1.814l4.73 6.935 25.017 37.59 1.296 11.536-1.814 3.76-6.481 2.268-7.13-1.297-14.647-20.544-15.1-23.138-12.185-20.739-1.49.843-7.194 77.448-3.37 3.953-7.778 2.981-6.48-4.925-3.436-7.972 3.435-15.749 4.148-20.544 3.37-16.333 3.046-20.285 1.815-6.74-.13-.454-1.49.194-15.295 20.999-23.267 31.433-18.406 19.702-4.407 1.75-7.648-3.954.713-7.064 4.277-6.286 25.47-32.405 15.36-20.092 9.917-11.6-.065-1.686h-.583L44.07 198.125l-12.055 1.555-5.185-4.86.648-7.972 2.463-2.593 20.35-13.999-.064.065Z" />
    </svg>
  );
}
function CodexIcon() {
  return (
    <svg viewBox="0 0 24 24" aria-hidden="true">
      <path fill="currentColor" fillRule="evenodd" d="M8.086.457a6.105 6.105 0 013.046-.415c1.333.153 2.521.72 3.564 1.7a.117.117 0 00.107.029c1.408-.346 2.762-.224 4.061.366l.063.03.154.076c1.357.703 2.33 1.77 2.918 3.198.278.679.418 1.388.421 2.126a5.655 5.655 0 01-.18 1.631.167.167 0 00.04.155 5.982 5.982 0 011.578 2.891c.385 1.901-.01 3.615-1.183 5.14l-.182.22a6.063 6.063 0 01-2.934 1.851.162.162 0 00-.108.102c-.255.736-.511 1.364-.987 1.992-1.199 1.582-2.962 2.462-4.948 2.451-1.583-.008-2.986-.587-4.21-1.736a.145.145 0 00-.14-.032c-.518.167-1.04.191-1.604.185a5.924 5.924 0 01-2.595-.622 6.058 6.058 0 01-2.146-1.781c-.203-.269-.404-.522-.551-.821a7.74 7.74 0 01-.495-1.283 6.11 6.11 0 01-.017-3.064.166.166 0 00.008-.074.115.115 0 00-.037-.064 5.958 5.958 0 01-1.38-2.202 5.196 5.196 0 01-.333-1.589 6.915 6.915 0 01.188-2.132c.45-1.484 1.309-2.648 2.577-3.493.282-.188.55-.334.802-.438.286-.12.573-.22.861-.304a.129.129 0 00.087-.087A6.016 6.016 0 015.635 2.31C6.315 1.464 7.132.846 8.086.457zm-.804 7.85a.848.848 0 00-1.473.842l1.694 2.965-1.688 2.848a.849.849 0 001.46.864l1.94-3.272a.849.849 0 00.007-.854l-1.94-3.393zm5.446 6.24a.849.849 0 000 1.695h4.848a.849.849 0 000-1.696h-4.848z" />
    </svg>
  );
}
function CursorIcon() {
  return (
    <svg viewBox="0 0 466.73 532.09" aria-hidden="true">
      <path fill="currentColor" d="M457.43,125.94L244.42,2.96c-6.84-3.95-15.28-3.95-22.12,0L9.3,125.94c-5.75,3.32-9.3,9.46-9.3,16.11v247.99c0,6.65,3.55,12.79,9.3,16.11l213.01,122.98c6.84,3.95,15.28,3.95,22.12,0l213.01-122.98c5.75-3.32,9.3-9.46,9.3-16.11v-247.99c0-6.65-3.55-12.79-9.3-16.11h-.01ZM444.05,151.99l-205.63,356.16c-1.39,2.4-5.06,1.42-5.06-1.36v-233.21c0-4.66-2.49-8.97-6.53-11.31L24.87,145.67c-2.4-1.39-1.42-5.06,1.36-5.06h411.26c5.84,0,9.49,6.33,6.57,11.39h-.01Z" />
    </svg>
  );
}
function OpenCodeIcon() {
  return (
    <svg viewBox="0 0 32 40" aria-hidden="true">
      <path d="M24 32H8V16H24V32Z" fill="currentColor" fillOpacity="0.35" />
      <path d="M24 8H8V32H24V8ZM32 40H0V0H32V40Z" fill="currentColor" />
    </svg>
  );
}

async function copyText(value: string) {
  try {
    await navigator.clipboard.writeText(value);
    return true;
  } catch {
    const area = document.createElement("textarea");
    area.value = value;
    area.setAttribute("readonly", "");
    area.style.position = "fixed";
    area.style.left = "-9999px";
    document.body.appendChild(area);
    area.select();
    const ok = document.execCommand("copy");
    area.remove();
    return ok;
  }
}

export default function DocsPage() {
  const { t } = useI18n();
  const [current, setCurrent] = useState("create");
  const [status, setStatus] = useState("202");
  const [count, setCount] = useState<number | null>(null);
  const [copied, setCopied] = useState("");
  const item = items.find((row) => row.id === current) || items[3];
  const resp = item.responses.find((row) => row.code === status) || item.responses[0];
  const sections = useMemo(
    () => [
      ["fingerprint", "模型指纹归因"],
      ["injection", "注入大小"],
      ["cache", "缓存读写"],
      ["tools", "工具调用"],
      ["web", "联网检测"],
      ["knowledge", "知识截止日期"],
      ["thinking", "思维链回传"],
      ["iq", "推理表现"],
    ],
    [],
  );

  useEffect(() => {
    const id = window.location.hash.replace("#", "");
    if (items.some((row) => row.id === id)) setCurrent(id);
  }, []);
  useEffect(() => {
    setStatus(item.responses[0]?.code || "");
  }, [item.id]);
  useEffect(() => {
    let alive = true;
    fetch("/api/v1")
      .then((r) => (r.ok ? r.json() : Promise.reject()))
      .then((body) => {
        if (alive && typeof body.public_audits === "number") setCount(body.public_audits);
      })
      .catch(() => {
        if (alive) setCount(null);
      });
    return () => {
      alive = false;
    };
  }, []);

  function flash(key: string) {
    setCopied(key);
    window.setTimeout(() => setCopied(""), 1400);
  }
  function choose(id: string) {
    setCurrent(id);
    history.replaceState(null, "", "#" + id);
  }
  async function copyAgent() {
    const origin = window.location.origin;
    const text = [
      t("你正在接入掺水？公共检测 API。按契约调用，不要猜字段，不要做在线调试。"),
      "",
      "Base: " + origin,
      "OpenAPI: " + origin + "/api/v1/openapi.json",
      "Docs: " + origin + "/docs",
      "",
      t("鉴权：没有请求头令牌。把上游密钥放进 JSON 的 api_key。不要把密钥写进仓库、日志或回复。"),
      t("频率：每个 IP 每 60 秒只能 POST /api/v1/audits 一次。轮询和 OpenAPI 不计数。429 时读取 Retry-After，单位是秒。"),
      t("创建成功返回 202，然后 GET poll_url。没有操作 key，不能重试失败项，不进全站历史。结果只在内存保留 24 小时，重启后查不到。"),
      t("sections 可省略、传 all，或传板块数组。可选 fingerprint、injection、cache、tools、web、knowledge、thinking、iq。all 包含缓存。"),
      t("thinking 只适用于 claude- 前缀。全部检测里遇到其他模型会跳过；只选这一项会返回 400。"),
      t("protocol 取 auto、chat、anthropic、openai，默认 auto。timeout 为 10 到 600 秒，默认 360。"),
      t("总分只按本次选中板块重算到 100。知识截止日期不进总分。"),
      t("轮询到结束后，回报检测 id、总分和各板块结论，不要复述 api_key。"),
      "",
      "POST /api/v1/audits",
      sample,
    ].join("\n");
    if (await copyText(text)) flash("agent");
  }

  return (
    <div className="api-doc">
      <header className="api-doc-head">
        <div>
          <p className="api-doc-kicker">{t("公共接口")}</p>
          <h1>{t("接口文档")}</h1>
        </div>
        <button type="button" className="agent-onboard" data-on={copied === "agent" ? "1" : "0"} onClick={copyAgent} aria-describedby="agent-onboard-tip">
          <span className="agent-onboard-tip" id="agent-onboard-tip" role="tooltip">{t("复制一段给编程 Agent 的接入说明")}</span>
          <span key={copied === "agent" ? "on" : "off"} className="agent-onboard-label">{copied === "agent" ? t("已复制") : t("让 Agent 接入掺水？")}</span>
          <span className="agent-onboard-icons" aria-hidden="true">
            <ClaudeIcon />
            <CodexIcon />
            <CursorIcon />
            <OpenCodeIcon />
          </span>
          <span className="sr-only">{t("适用于 Claude、Codex、Cursor 和 OpenCode")}</span>
        </button>
        <p className="api-doc-count" aria-live="polite">
          {count == null ? t("次数暂不可用") : t("已公益提供检测 {n} 次", { n: count.toLocaleString("zh-CN") })}
        </p>
      </header>
      <div className="api-doc-layout">
        <nav className="api-tree" aria-label={t("接口列表")}>
          <p>{t("接口列表")}</p>
          {items.map((row) => (
            <button key={row.id} type="button" aria-current={row.id === item.id ? "page" : undefined} onClick={() => choose(row.id)}>
              {row.method ? <b className={"verb " + row.method.toLowerCase()}>{row.method}</b> : <b className="verb doc">DOC</b>}
              <span>{t(row.name)}</span>
            </button>
          ))}
        </nav>
        <article className="api-detail" id={item.id}>
          <h2>{t(item.name)}</h2>
          {item.path ? (
            <div className="api-url">
              <b className={"verb-pill " + (item.method || "").toLowerCase()}>{item.method}</b>
              <code>{item.path}</code>
              <button type="button" onClick={() => copyText(item.path || "").then((ok) => ok && flash("path"))}>
                {copied === "path" ? t("已复制") : t("复制路径")}
              </button>
            </div>
          ) : null}
          <p className="api-summary">{t(item.summary)}</p>
          {item.id === "guide" ? (
            <ul className="api-sections">
              {sections.map(([id, label]) => (
                <li key={id}>
                  <code>{id}</code>
                  <span>{t(label)}</span>
                </li>
              ))}
            </ul>
          ) : null}
          <section>
            <h3>{item.paramsLabel ? t(item.paramsLabel) : t("请求参数")}</h3>
            {item.params.length ? (
              <div className="api-table-wrap">
                <table className="api-table">
                  <thead>
                    <tr>
                      <th>{t("参数名")}</th>
                      <th>{t("类型")}</th>
                      <th>{t("必填")}</th>
                      <th>{t("说明")}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {item.params.map((param) => (
                      <tr key={param.name}>
                        <td>
                          <code>{param.name}</code>
                          {param.required ? <i aria-hidden="true">*</i> : null}
                        </td>
                        <td>{param.type}</td>
                        <td>{param.required ? t("是") : t("否")}</td>
                        <td>{t(param.note)}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            ) : (
              <p className="api-empty">{t("这个接口没有参数。")}</p>
            )}
          </section>
          {item.example ? (
            <section>
              <div className="api-block-head">
                <h3>{t("请求示例")}</h3>
                <button type="button" onClick={() => copyText(item.example || "").then((ok) => ok && flash("req"))}>
                  {copied === "req" ? t("已复制") : t("复制")}
                </button>
              </div>
              <pre>{item.example}</pre>
            </section>
          ) : null}
          {item.responses.length ? (
            <section>
              <h3>{t("返回响应")}</h3>
              <div className="api-status-tabs" role="tablist">
                {item.responses.map((row) => (
                  <button key={row.code} type="button" role="tab" aria-selected={resp?.code === row.code} onClick={() => setStatus(row.code)}>
                    <b className={row.code.startsWith("2") ? "ok" : "bad"}>{row.code}</b>
                    <span>{t(row.title)}</span>
                  </button>
                ))}
              </div>
              {resp ? (
                <div className="api-response">
                  {resp.note ? <p className="api-note">{t(resp.note)}</p> : null}
                  <div className="api-table-wrap">
                    <table className="api-table">
                      <thead>
                        <tr>
                          <th>{t("参数名")}</th>
                          <th>{t("类型")}</th>
                          <th>{t("说明")}</th>
                        </tr>
                      </thead>
                      <tbody>
                        {resp.fields.map((field) => (
                          <tr key={field.name}>
                            <td><code>{field.name}</code></td>
                            <td>{field.type}</td>
                            <td>{t(field.note)}</td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                  <div className="api-block-head">
                    <h3>{t("示例")}</h3>
                    <button type="button" onClick={() => copyText(resp.example).then((ok) => ok && flash("res"))}>
                      {copied === "res" ? t("已复制") : t("复制")}
                    </button>
                  </div>
                  <pre>{resp.example}</pre>
                </div>
              ) : null}
            </section>
          ) : null}
          <p className="api-spec-link">
            <a href="/api/v1/openapi.json">{t("机器可读规范")}</a>
          </p>
        </article>
      </div>
      <span className="sr-only" aria-live="polite">{copied ? t("已复制") : ""}</span>
    </div>
  );
}
