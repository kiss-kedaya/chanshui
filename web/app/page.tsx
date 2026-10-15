"use client";
import { useEffect, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { useI18n } from "@/lib/i18n";
import { reportPath, modelFamily } from "@/lib/report-view";
import { loadKey, saveKey, clearKey } from "@/lib/keyvault";
import { startDetect, getBankStats, getModels, ask, BankStats, Metrics } from "@/lib/api";
import { saveJobControl } from "@/lib/job-controls";
import AuditHero from "@/components/AuditHero";
import Hint from "@/components/Hint";
import Icon from "@/components/Icon";
import ErrorFold from "@/components/ErrorFold";
import Select from "@/components/Select";
import Link from "next/link";
export default function DetectPage() {
  const { t: tr } = useI18n();
  const router = useRouter();
  const [baseUrl, setBaseUrl] = useState("");
  const [key, setKey] = useState("");
  const [showKey, setShowKey] = useState(false);
  const [model, setModel] = useState("");
  const [protocol, setProtocol] = useState<
    "auto" | "chat" | "anthropic" | "openai"
  >("auto");
  const [stats, setStats] = useState<BankStats | null>(null);
  const [models, setModels] = useState<string[] | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [modelBusy, setModelBusy] = useState(false);

  const [concurrency, setConcurrency] = useState(0);
  const [timeout, setTimeoutValue] = useState(360);
  const [cacheOn, setCacheOn] = useState(false);
  const [prompt, setPrompt] = useState("");
  const [mode, setMode] = useState("summary");
  const [budget, setBudget] = useState(2048);
  const [effort, setEffort] = useState("");
  const [answer, setAnswer] = useState<Metrics | null>(null);
  const [asking, setAsking] = useState(false);
  const [askError, setAskError] = useState("");
  const alive = useRef(true);
  useEffect(() => {
    alive.current = true;
    getBankStats()
      .then((value) => {
        if (!alive.current) return;
        setStats(value);
      })
      .catch((e) => {
        if (alive.current) setError(e.message);
      })
      .finally(() => {

      });
    try {
      const p = JSON.parse(localStorage.getItem("chanshui-settings") || "{}");
      const storedBaseUrl = typeof p.base_url === "string" ? p.base_url.trim() : "";
      // Remove the retired prefilled endpoint from existing browser settings.
      const cleanedBaseUrl =
        /^https?:\/\/api\.125837\.xyz(?:\/|$)/i.test(storedBaseUrl)
          ? ""
          : storedBaseUrl;
      setBaseUrl(cleanedBaseUrl);
      setModel(p.model || "");
      if (["auto", "chat", "anthropic", "openai"].includes(p.protocol))
        setProtocol(p.protocol);
      if (typeof p.cache === "boolean") setCacheOn(p.cache);
      if (cleanedBaseUrl !== storedBaseUrl) {
        localStorage.setItem(
          "chanshui-settings",
          JSON.stringify({ ...p, base_url: "" }),
        );
      }
    } catch {}
    // 已保存的 Key 从加固存储里异步读回；读不到就保持空，让用户重填。
    loadKey()
      .then((saved) => {
        if (alive.current && saved) setKey(saved);
      })
      .catch(() => {});
    return () => {
      alive.current = false;
    };
  }, []);
  const settings = () => ({
    base_url: baseUrl.trim(),
    key: key.trim(),
    model: model.trim(),
    protocol,
    concurrency,
    timeout,
    cache: cacheOn,
  });
  // 首页只负责“新建检测”：提交成功后直接跳到报告页，结果、重跑都在 /history 处理。
  const run = async () => {
    setError("");
    setBusy(true);
    try {
      try {
        localStorage.setItem(
          "chanshui-settings",
          JSON.stringify({ base_url: baseUrl.trim(), model, protocol, cache: cacheOn }),
        );
      } catch {}
      // Key 单独加固落盘（IndexedDB 里不可提取的 AES 密钥加密），方便回报告页重试。
      await saveKey(key);
      const d = await startDetect(settings());
      if (!saveJobControl({ jobId: d.task_id, token: d.control_token, expiresAt: d.expires_at }))
        setError(tr("浏览器未允许保存操作 key，刷新后将无法重试此任务"));
      router.push(reportPath(d.report_id));
    } catch (e) {
      setError((e as Error).message);
      setBusy(false);
    }
  };
  const loadModels = async () => {
    setModelBusy(true);
    setError("");
    try {
      const d = await getModels({
        base_url: baseUrl.trim(),
        key: key.trim(),
        protocol,
      });
      setModels(d.models);
      if (!model.trim()) setModel(d.models[0] || "");
      if (!d.models.length)
        setError(tr("上游未返回模型，可直接输入实际调用名称。"));
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setModelBusy(false);
    }
  };
  const askOnce = async () => {
    setAsking(true);
    setAskError("");
    setAnswer(null);
    try {
      const thinking = modelFamily(model) !== "claude"
        ? null
        : mode === "omit"
          ? null
          : mode === "disabled"
            ? { type: "disabled" }
            : mode === "highlights"
              ? { type: "adaptive", display: "highlights" }
              : mode === "updates"
                ? { type: "adaptive", display: "updates" }
              : mode === "budget"
              ? {
                  type: "enabled",
                  budget_tokens: budget,
                  display: "summarized",
                }
              : mode === "summary"
                ? { type: "adaptive", display: "summarized" }
                : { type: "adaptive" };
      const m = await ask(settings(), prompt, thinking, effort);
      setAnswer(m);
      if (m.Err) setAskError(m.Err);
    } catch (e) {
      setAskError((e as Error).message);
    } finally {
      setAsking(false);
    }
  };
  const canRun = !!(baseUrl.trim() && key.trim() && model.trim() && !busy);

  return (
    <div className="page-shell">
      <header className="page-top">
        <div className="eyebrow breadcrumb">
          <Icon name="scan" />
          <span>{tr("掺水？")}</span>
          <Icon name="chevron" />
          <b>{tr("模型审计")}</b>
        </div>
        <Link className="report-query-link" href="/history">
          {tr("按 ID 查报告")}
        </Link>
        <span className="local-badge">
          <i />
          {tr("提交即分享 · 报告保留24小时")}
        </span>
      </header>
      <div className="opening">
      <AuditHero />
      <section className="hero-card connection-card" id="connection">
        <div className="section-heading">
          <div>
            <span className="step">01</span>
            <h2>{tr("连接你的网关")}</h2>
          </div>
          <Hint
            label={tr("密钥安全说明")}
            text={tr("密钥仅用于当前任务，不写入历史或浏览器存储。")}
          >
            <Icon name="shield" />
          </Hint>
        </div>
        <form
          onSubmit={(e) => {
            e.preventDefault();
            if (canRun) void run();
          }}
        >
          <div className="field protocol-field">
            <div className="label-row">
              <label htmlFor="protocol">{tr("接口格式")}</label>
              <Hint
                label={tr("接口格式说明")}
                text={tr("自适应按模型选择协议。Chat使用 /v1/chat/completions；Anthropic原生使用 /v1/messages；OpenAI原生使用 /v1/responses。手动选择不会自动换协议。")}
              />
            </div>
            <Select
              id="protocol"
              value={protocol}
              disabled={busy}
              onChange={(e) => {
                setProtocol(e.target.value as typeof protocol);
                setModels(null);
              }}
            >
              <option value="auto">{tr("自适应")}</option>
              <option value="chat">{tr("Chat Completions · OpenAI 兼容")}</option>
              <option value="anthropic">{tr("Anthropic 原生")}</option>
              <option value="openai">{tr("OpenAI 原生")}</option>
            </Select>
          </div>
          <div className="connection-grid">
            <div className="field">
              <div className="label-row">
                <label htmlFor="gateway">{tr("接口地址")}</label>
                <Hint text={tr("支持根地址或带 /v1 的地址。")} />
              </div>
              <input
                id="gateway"
                placeholder={tr("网关地址")}
                value={baseUrl}
                onChange={(e) => {
                  setBaseUrl(e.target.value);
                  setModels(null);
                }}
                disabled={busy}
                spellCheck={false}
              />
            </div>
            <div className="field">
              <label htmlFor="api-key">API Key</label>
              <div className="input-action">
                <input
                  id="api-key"
                  type={showKey ? "text" : "password"}
                  placeholder={tr("输入密钥")}
                  autoComplete="off"
                  value={key}
                  onChange={(e) => {
                    setKey(e.target.value);
                    setModels(null);
                  }}
                  disabled={busy}
                  spellCheck={false}
                />
                <button
                  type="button"
                  onClick={() => setShowKey(!showKey)}
                  aria-label={showKey ? tr("隐藏密钥") : tr("显示密钥")}
                >
                  <Icon name={showKey ? "eyeOff" : "eye"} />
                </button>
              </div>
              {key && (
                <button
                  type="button"
                  className="key-forget"
                  disabled={busy}
                  onClick={() => {
                    setKey("");
                    setModels(null);
                    void clearKey();
                  }}
                >
                  {tr("清除已保存的密钥")}
                </button>
              )}
            </div>
          </div>
          <div className="model-row">
            <div className="field">
              <label htmlFor="model">{tr("目标模型")}</label>
              <input
                id="model"
                value={model}
                disabled={busy}
                list="model-suggestions"
                placeholder="例如 openai/gpt-6.1-sol"
                maxLength={256}
                autoComplete="off"
                aria-describedby="model-help"
                onChange={(e) => setModel(e.target.value)}
              />
              <datalist id="model-suggestions">
                {(models ?? stats?.supported_models ?? []).map((m) => (
                  <option key={m} value={m}>
                    {m}
                  </option>
                ))}
              </datalist>
              <small id="model-help">{tr("可选择或输入自定义名称，请求时原样发送；没有对应基线时不代表已验证型号。")}</small>
            </div>
            <button
              className="btn-secondary icon-button model-refresh"
              type="button"
              title={tr("获取模型列表")}
              aria-label={tr("获取模型列表")}
              aria-busy={modelBusy}
              onClick={loadModels}
              disabled={!baseUrl || !key || busy || modelBusy}
            >
              <Icon
                name={modelBusy ? "spinner" : "refresh"}
                className={modelBusy ? "icon-spin" : ""}
              />
              <span className="sr-only">
                {modelBusy ? tr("正在获取模型") : tr("获取模型列表")}
              </span>
            </button>
          </div>
          <details className="advanced">
            <summary>
              高级设置 <span>{tr("并发 · 超时")}</span>
            </summary>
            <div className="advanced-grid">
              <div className="field">
                <label htmlFor="concurrency">{tr("并发（0 = 全部并行）")}</label>
                <input
                  id="concurrency"
                  type="number"
                  min="0"
                  max="32"
                  value={concurrency}
                  onChange={(e) => setConcurrency(Number(e.target.value))}
                />
              </div>
              <div className="field">
                <label htmlFor="timeout">{tr("单请求超时 / 秒")}</label>
                <input
                  id="timeout"
                  type="number"
                  min="10"
                  max="600"
                  value={timeout}
                  onChange={(e) => setTimeoutValue(Number(e.target.value))}
                />
              </div>
            </div>
          </details>
          <div className="form-bottom">
            <p>
              <span className="usage-note">{tr("会产生接口用量")}</span>
              <Hint
                label={tr("检测用量说明")}
                text={tr("注入大小使用两个独立小请求。缓存读写默认关闭，打开后才跑 6 轮串行对话。每个失败请求独立最多重试 3 次。报告全站可见，有效期 24 小时。")}
              />
            </p>
            <div className="actions">
              <label className="cache-opt cache-beside" title={tr("可选。按普通多轮对话测前缀缓存，默认不跑。")}>
                <input
                  type="checkbox"
                  checked={cacheOn}
                  disabled={busy}
                  onChange={(e) => setCacheOn(e.target.checked)}
                />
                {tr("缓存读写")}
                <Hint
                  label={tr("缓存读写说明")}
                  text={tr("默认关闭。打开后用 6 轮普通对话串行检测：上一条读加写等于下一条读就是 100%。只对勾选的任务计分。")}
                />
              </label>
              <button
                className="btn-primary detect-button"
                type="submit"
                disabled={!canRun}
                aria-busy={busy}
              >
                <Icon
                  name={busy ? "spinner" : "scan"}
                  className={busy ? "icon-spin" : ""}
                />
                {busy ? tr("正在检测…") : tr("一键检测")}
                <Icon name="arrow" />
              </button>
            </div>
          </div>
          {error && <ErrorFold text={error} />}
        </form>
        <div className="bank-strip">
          <span>
            <Icon name="layers" /> <b>{stats?.fingerprint_count ?? "—"}</b>{" "}
            {tr("个可测模型")}
          </span>
          <Hint
            label={tr("模型库说明")}
            text={tr("服务端定期同步模型基线。数字指纹仅对库内候选比较，行为参考不等于型号证明。")}
          />
        </div>
      </section>
      </div>
      <details className="ask-panel">
        <summary>{tr("单问单答")}</summary>
        <div className="advanced-grid">
          {modelFamily(model) === "claude" && (
            <div className="field">
              <label htmlFor="thinking-mode">{tr("思考模式")}</label>
              <Select
                id="thinking-mode"
                value={mode}
                onChange={(e) => setMode(e.target.value)}
              >
                <option value="summary">{tr("自适应 + 摘要")}</option>
                <option value="adaptive">{tr("自适应（不传 display）")}</option>
                <option value="highlights">{tr("自适应 + 要点")}</option>
                <option value="updates">{tr("自适应 + 进度更新（beta）")}</option>
                <option value="budget">{tr("指定预算")}</option>
                <option value="omit">{tr("不传 thinking")}</option>
                <option value="disabled">{tr("明确关闭（部分模型不支持）")}</option>
              </Select>
            </div>
          )}
          {modelFamily(model) === "claude" && mode === "budget" && (
            <div className="field">
              <label htmlFor="budget">{tr("思考预算 token")}</label>
              <input
                id="budget"
                type="number"
                min="1024"
                max="8192"
                value={budget}
                onChange={(e) => setBudget(Number(e.target.value))}
              />
            </div>
          )}
          <div className="field">
            <label htmlFor="effort">{tr("推理努力（按上游支持）")}</label>
            <Select
              id="effort"
              value={effort}
              onChange={(e) => setEffort(e.target.value)}
            >
              {[
                ["", tr("不传")],
                ["low", tr("低")],
                ["medium", tr("中")],
                ["high", tr("高")],
                ["max", tr("最大")],
              ].map(([v, t]) => (
                <option key={v} value={v}>
                  {t}
                </option>
              ))}
            </Select>
          </div>
        </div>
        <div className="field">
          <label htmlFor="question">{tr("向当前选中模型提问")}</label>
          <textarea
            id="question"
            value={prompt}
            onChange={(e) => setPrompt(e.target.value)}
            placeholder={tr("输入问题，Ctrl + Enter 发送")}
            onKeyDown={(e) => {
              if (
                e.ctrlKey &&
                e.key === "Enter" &&
                !asking &&
                canRun &&
                prompt.trim()
              )
                void askOnce();
            }}
          />
        </div>
        <button
          className="btn-primary"
          disabled={!canRun || !prompt.trim() || asking}
          onClick={askOnce}
        >
          {asking ? tr("正在回答…") : tr("发送问题")}
        </button>
        {askError && <ErrorFold text={askError} />}
        {answer && (
          <div className="answer-panel">
            <pre>{answer.Answer || tr("未返回正文")}</pre>
            <p className="muted">
              {tr("输入 {a} · 输出 {b} · 思考 {c} · 缓存写入 {d} · 缓存读取 {e}", {
                a: answer.InputTokens,
                b: answer.OutputTokens,
                c: answer.ThinkingTokens,
                d: answer.CacheCreation5m + answer.CacheCreation1h,
                e: answer.CacheRead,
              })}
            </p>
            {modelFamily(model) === "claude" && (
              <details>
                <summary>{tr("思维链全文与签名")}</summary>
                <pre>{answer.ThinkingText || tr("未回传摘要")}</pre>
                <pre>{answer.Signature || tr("无签名")}</pre>
              </details>
            )}
          </div>
        )}
      </details>
    </div>
  );
}
