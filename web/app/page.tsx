"use client";
import { useEffect, useRef, useState } from "react";
import {
  startDetect,
  getDetect,
  retryDetect,
  cancelDetect,
  getBankStats,
  getModels,
  ask,
  DetectTask,
  BankStats,
  Metrics,
} from "@/lib/api";
import AuditHero from "@/components/AuditHero";
import Hint from "@/components/Hint";
import ProbeTimeline from "@/components/ProbeTimeline";
import Icon from "@/components/Icon";
import Select from "@/components/Select";
import Link from "next/link";
import ReportShare from "@/components/ReportShare";
import ResultCards from "@/components/ResultCards";
export default function DetectPage() {
  const [baseUrl, setBaseUrl] = useState("");
  const [key, setKey] = useState("");
  const [showKey, setShowKey] = useState(false);
  const [model, setModel] = useState("");
  const [protocol, setProtocol] = useState<
    "auto" | "chat" | "anthropic" | "openai"
  >("auto");
  const [stats, setStats] = useState<BankStats | null>(null);
  const [models, setModels] = useState<string[] | null>(null);
  const [task, setTask] = useState<DetectTask | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [modelBusy, setModelBusy] = useState(false);
  const [concurrency, setConcurrency] = useState(0);
  const [timeout, setTimeoutValue] = useState(180);
  const controlToken = useRef("");
  const [prompt, setPrompt] = useState("");
  const [mode, setMode] = useState("summary");
  const [budget, setBudget] = useState(2048);
  const [effort, setEffort] = useState("");
  const [answer, setAnswer] = useState<Metrics | null>(null);
  const [asking, setAsking] = useState(false);
  const [askError, setAskError] = useState("");
  const alive = useRef(true);
  const session = useRef(0);
  useEffect(() => {
    alive.current = true;
    getBankStats()
      .then(setStats)
      .catch((e) => setError(e.message));
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
      if (cleanedBaseUrl !== storedBaseUrl) {
        localStorage.setItem(
          "chanshui-settings",
          JSON.stringify({ ...p, base_url: "" }),
        );
      }
    } catch {}
    return () => {
      alive.current = false;
      session.current++;
    };
  }, []);
  const settings = () => ({
    base_url: baseUrl.trim(),
    key: key.trim(),
    model: model.trim(),
    protocol,
    concurrency,
    timeout,
  });
  const poll = async (id: string) => {
    const token = ++session.current;
    try {
      for (;;) {
        const t = await getDetect(id);
        if (!alive.current || token !== session.current) return;
        setTask(t);
        if (!["running", "queued"].includes(t.status)) {
          window.dispatchEvent(new Event("history-updated"));
          break;
        }
        await new Promise((r) => window.setTimeout(r, 900));
      }
    } catch (e) {
      if (alive.current) setError((e as Error).message);
    } finally {
      if (alive.current) setBusy(false);
    }
  };
  const run = async (retry = false) => {
    setError("");
    setBusy(true);
    if (!retry) setTask(null);
    try {
      localStorage.setItem(
        "chanshui-settings",
        JSON.stringify({ base_url: baseUrl.trim(), model, protocol }),
      );
      const d =
        retry && task
          ? await retryDetect(task.id, settings(), controlToken.current)
          : await startDetect(settings());
      if ("control_token" in d)
        controlToken.current = d.control_token as string;
      setTask(await getDetect(d.task_id));
      await poll(d.task_id);
    } catch (e) {
      setError((e as Error).message);
      setBusy(false);
    }
  };
  const loadModels = async () => {
    setModelBusy(true);
    setError("");
    try {
      const d = await getModels(settings());
      setModels(d.models);
      if (!d.models.includes(model)) setModel(d.models[0] || "");
      if (!d.models.length)
        setError("上游模型与可测清单没有交集。未发送检测请求。");
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
      const thinking = !model.toLowerCase().startsWith("claude-")
        ? null
        : mode === "omit"
          ? null
          : mode === "disabled"
            ? { type: "disabled" }
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
  const canRun = !!(baseUrl.trim() && key.trim() && model && !busy);
  const failed =
    task?.probes.filter((p) => p.status === "error" || p.status === "cancelled")
      .length || 0;

  return (
    <div className="page-shell">
      <header className="page-top">
        <div className="eyebrow breadcrumb">
          <Icon name="scan" />
          <span>掺水？</span>
          <Icon name="chevron" />
          <b>模型审计</b>
        </div>
        <Link className="report-query-link" href="/history">
          按 ID 查报告
        </Link>
        <span className="local-badge">
          <i /> 提交即分享 · 报告保留24小时
        </span>
      </header>
      <AuditHero />
      <section className="hero-card connection-card" id="connection">
        <div className="section-heading">
          <div>
            <span className="step">01</span>
            <h2>连接你的网关</h2>
          </div>
          <Hint
            label="密钥安全说明"
            text="密钥仅用于当前任务，不写入历史或浏览器存储。"
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
              <label htmlFor="protocol">接口格式</label>
              <Hint
                label="接口格式说明"
                text="自适应按模型选择协议。Chat使用 /v1/chat/completions；Anthropic原生使用 /v1/messages；OpenAI原生使用 /v1/responses。手动选择不会自动换协议。"
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
              <option value="auto">自适应</option>
              <option value="chat">Chat Completions · OpenAI 兼容</option>
              <option value="anthropic">Anthropic 原生</option>
              <option value="openai">OpenAI 原生</option>
            </Select>
          </div>
          <div className="connection-grid">
            <div className="field">
              <div className="label-row">
                <label htmlFor="gateway">接口地址</label>
                <Hint text="支持根地址或带 /v1 的地址。" />
              </div>
              <input
                id="gateway"
                placeholder="网关地址"
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
                  placeholder="输入密钥"
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
                  aria-label={showKey ? "隐藏密钥" : "显示密钥"}
                >
                  <Icon name={showKey ? "eyeOff" : "eye"} />
                </button>
              </div>
            </div>
          </div>
          <div className="model-row">
            <div className="field">
              <label htmlFor="model">目标模型</label>
              <Select
                id="model"
                value={model}
                disabled={busy}
                loading={modelBusy}
                onChange={(e) => setModel(e.target.value)}
              >
                <option value="">选择可测模型</option>
                {(models ?? stats?.supported_models ?? []).map((m) => (
                  <option key={m} value={m}>
                    {m}
                  </option>
                ))}
              </Select>
            </div>
            <button
              className="btn-secondary icon-button model-refresh"
              type="button"
              title="获取模型列表"
              aria-label="获取模型列表"
              aria-busy={modelBusy}
              onClick={loadModels}
              disabled={!baseUrl || !key || busy || modelBusy}
            >
              <Icon
                name={modelBusy ? "spinner" : "refresh"}
                className={modelBusy ? "icon-spin" : ""}
              />
              <span className="sr-only">
                {modelBusy ? "正在获取模型" : "获取模型列表"}
              </span>
            </button>
          </div>
          <details className="advanced">
            <summary>
              高级设置 <span>并发 · 超时</span>
            </summary>
            <div className="advanced-grid">
              <div className="field">
                <label htmlFor="concurrency">并发（0 = 全部并行）</label>
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
                <label htmlFor="timeout">单请求超时 / 秒</label>
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
              <span className="usage-note">会产生接口用量</span>
              <Hint
                label="检测用量说明"
                text="一般请求约 15～18 次。缓存用三个小输出请求：Claude 约 5 千 token 前缀，其他模型约 2 千；以实际返回用量为准。失败最多重试 3 次。报告全站可见，有效期 24 小时。"
              />
            </p>
            <div className="actions">
              {busy && task && (
                <button
                  type="button"
                  className="btn-secondary"
                  onClick={() =>
                    cancelDetect(task.id, controlToken.current).catch((e) =>
                      setError(e.message),
                    )
                  }
                >
                  停止检测
                </button>
              )}
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
                {busy ? "正在检测…" : "一键检测"}
                <Icon name="arrow" />
              </button>
            </div>
          </div>
          {error && (
            <div role="alert" className="error-box">
              {error}
            </div>
          )}
        </form>
      </section>
      <div className="bank-strip">
        <span>
          <Icon name="layers" /> <b>{stats?.fingerprint_count ?? "—"}</b>{" "}
          个可测模型
        </span>
        <Hint
          label="模型库说明"
          text="服务端定期同步模型基线。数字指纹仅对库内候选比较，行为参考不等于型号证明。"
        />
      </div>
      {task && (
        <ProbeTimeline probes={task.probes} status={task.status}>
          {!busy && (
            <div className="report-actions">
              <span className="muted">{task.warning || ""}</span>
              {failed > 0 && (
                <button
                  className="btn-secondary"
                  onClick={() => run(true)}
                  disabled={!key}
                >
                  重跑失败项（{failed}）
                </button>
              )}
              <button
                className="btn-secondary"
                onClick={() => {
                  const blob = new Blob(
                    [
                      JSON.stringify(
                        {
                          report_id: task.report_id,
                          created_at: task.created_at,
                          model: task.model,
                          verdict: task.verdict,
                        },
                        null,
                        2,
                      ),
                    ],
                    { type: "application/json" },
                  );
                  const u = URL.createObjectURL(blob);
                  const a = document.createElement("a");
                  a.href = u;
                  a.download =
                    "chanshui-report-" + (task.report_id || task.id) + ".json";
                  a.click();
                  URL.revokeObjectURL(u);
                }}
              >
                导出报告
              </button>
            </div>
          )}
        </ProbeTimeline>
      )}
      {task?.report_id && (
        <ReportShare
          key={task.report_id}
          id={task.report_id}
          expiresAt={task.expires_at}
        />
      )}
      {task && (
        <ResultCards
          verdict={task.verdict}
          model={task.model}
          loading={["queued", "running"].includes(task.status)}
        />
      )}
      <details className="ask-panel">
        <summary>单问单答</summary>
        <div className="advanced-grid">
          {model.toLowerCase().startsWith("claude-") && (
            <div className="field">
              <label htmlFor="thinking-mode">思考模式</label>
              <Select
                id="thinking-mode"
                value={mode}
                onChange={(e) => setMode(e.target.value)}
              >
                <option value="summary">自适应 + 摘要</option>
                <option value="adaptive">自适应（不传 display）</option>
                <option value="budget">指定预算</option>
                <option value="omit">不传 thinking</option>
                <option value="disabled">明确关闭</option>
              </Select>
            </div>
          )}
          {model.toLowerCase().startsWith("claude-") && mode === "budget" && (
            <div className="field">
              <label htmlFor="budget">思考预算 token</label>
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
            <label htmlFor="effort">推理努力（按上游支持）</label>
            <Select
              id="effort"
              value={effort}
              onChange={(e) => setEffort(e.target.value)}
            >
              {[
                ["", "不传"],
                ["low", "低"],
                ["medium", "中"],
                ["high", "高"],
                ["max", "最大"],
              ].map(([v, t]) => (
                <option key={v} value={v}>
                  {t}
                </option>
              ))}
            </Select>
          </div>
        </div>
        <div className="field">
          <label htmlFor="question">向当前选中模型提问</label>
          <textarea
            id="question"
            value={prompt}
            onChange={(e) => setPrompt(e.target.value)}
            placeholder="输入问题，Ctrl + Enter 发送"
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
          {asking ? "正在回答…" : "发送问题"}
        </button>
        {askError && <p className="error-box">{askError}</p>}
        {answer && (
          <div className="answer-panel">
            <pre>{answer.Answer || "未返回正文"}</pre>
            <p className="muted">
              输入 {answer.InputTokens} · 输出 {answer.OutputTokens} · 思考{" "}
              {answer.ThinkingTokens} · 缓存写入{" "}
              {answer.CacheCreation5m + answer.CacheCreation1h} · 缓存读取{" "}
              {answer.CacheRead}
            </p>
            {model.toLowerCase().startsWith("claude-") && (
              <details>
                <summary>思维链全文与签名</summary>
                <pre>{answer.ThinkingText || "未回传摘要"}</pre>
                <pre>{answer.Signature || "无签名"}</pre>
              </details>
            )}
          </div>
        )}
      </details>
    </div>
  );
}
