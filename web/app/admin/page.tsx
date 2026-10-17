"use client";

import Icon from "@/components/Icon";
import Select from "@/components/Select";
import { FormEvent, useEffect, useState } from "react";
import { useI18n } from "@/lib/i18n";
import { loadKey, saveKey } from "@/lib/keyvault";
import { getModels } from "@/lib/api";

type Item = {
  id: string;
  created_at: number;
  kind: string;
  contact: string;
  message: string;
};

type Reference = {
  id: string;
  display_name: string;
  family: string;
  family_name: string;
  source: string;
  captured_at: string;
  samples: number;
  aliases?: string[];
};

type CaptureGroup = {
  group: number;
  top_model: string;
  top_display: string;
  probability: number;
  pass: boolean;
};

type Capture = {
  id: string;
  status: string;
  started_at: string;
  updated_at: string;
  train_requested: number;
  holdout_requested: number;
  total: number;
  completed: number;
  train_usable: number;
  holdout_usable: number;
  error?: string;
  groups?: CaptureGroup[];
  reference?: Reference;
};

type CaptureForm = {
  base_url: string;
  key: string;
  model: string;
  id: string;
  display_name: string;
  aliases: string;
  protocol: "auto" | "chat" | "anthropic" | "openai";
  train: number;
  holdout: number;
  concurrency: number;
  timeout: number;
};

const emptyCapture: CaptureForm = {
  base_url: "",
  key: "",
  model: "",
  id: "",
  display_name: "",
  aliases: "",
  protocol: "auto",
  train: 36,
  holdout: 12,
  concurrency: 3,
  timeout: 180,
};

const captureLabels: Record<string, string> = {
  running: "采集中",
  validating: "校验归因",
  done: "已完成",
  failed: "失败",
  cancelled: "已取消",
};

export default function AdminPage() {
  const { t: tr } = useI18n();
  const kinds: Record<string, string> = {
    bug: tr("问题反馈"),
    method: tr("检测方法"),
    suggestion: tr("改进建议"),
  };
  const [token, setToken] = useState("");
  const [show, setShow] = useState(false);
  const [items, setItems] = useState<Item[] | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [checking, setChecking] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [refs, setRefs] = useState<Reference[]>([]);
  const [capture, setCapture] = useState<Capture | null>(null);
  const [form, setForm] = useState<CaptureForm>(emptyCapture);
  const [captureBusy, setCaptureBusy] = useState(false);
  const [captureError, setCaptureError] = useState("");
  const [fillBusy, setFillBusy] = useState(false);
  const [modelBusy, setModelBusy] = useState(false);
  const [captureModels, setCaptureModels] = useState<string[]>([]);
  const [showCaptureKey, setShowCaptureKey] = useState(false);
  const [tab, setTab] = useState<"feedback" | "fingerprint">("feedback");

  async function load() {
    const [feedback, fingerprints] = await Promise.all([
      fetch("/api/admin/feedback", { cache: "no-store", credentials: "same-origin" }),
      fetch("/api/admin/fingerprints", { cache: "no-store", credentials: "same-origin" }),
    ]);
    if (feedback.status === 401) {
      setItems(null);
      return;
    }
    const data = await feedback.json().catch(() => ({}));
    if (!feedback.ok) throw Error(data.error || tr("读取失败"));
    setItems(data.items || []);
    if (fingerprints.ok) {
      const fp = await fingerprints.json().catch(() => ({}));
      setRefs(fp.items || []);
      if (fp.capture) setCapture(fp.capture);
    }
  }

  useEffect(() => {
    setChecking(true);
    load()
      .catch(() => setItems(null))
      .finally(() => setChecking(false));
  }, []);

  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      const res = await fetch("/api/admin/login", {
        method: "POST",
        credentials: "same-origin",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ token }),
      });
      const data = await res.json().catch(() => ({}));
      if (!res.ok) throw Error(data.error || tr("登录失败"));
      setToken("");
      await load();
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setBusy(false);
    }
  }

  async function logout() {
    await fetch("/api/admin/logout", { method: "POST", credentials: "same-origin" });
    setItems(null);
    setRefs([]);
    setCapture(null);
    setError("");
    setTab("feedback");
  }

  async function refresh() {
    setRefreshing(true);
    setError("");
    try {
      await load();
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setRefreshing(false);
    }
  }

  async function fillSaved() {
    setFillBusy(true);
    setCaptureError("");
    try {
      let saved: { base_url?: string; model?: string; protocol?: string } = {};
      try {
        saved = JSON.parse(localStorage.getItem("chanshui-settings") || "{}");
      } catch {}
      const key = await loadKey();
      if (!saved.base_url && !key) {
        setCaptureError(tr("本机没有保存过的网关或 Key，请手动填写。"));
        return;
      }
      setForm((f) => ({
        ...f,
        base_url: f.base_url || saved.base_url || "",
        key: f.key || key || "",
        model: f.model || saved.model || "",
        protocol:
          f.protocol === "auto" && ["auto", "chat", "anthropic", "openai"].includes(saved.protocol || "")
            ? (saved.protocol as CaptureForm["protocol"])
            : f.protocol,
      }));
    } catch (err) {
      setCaptureError((err as Error).message);
    } finally {
      setFillBusy(false);
    }
  }

  async function listCaptureModels() {
    if (!form.base_url.trim() || !form.key.trim()) {
      setCaptureError(tr("先填写接口地址和 API Key。"));
      return;
    }
    setModelBusy(true);
    setCaptureError("");
    try {
      const data = await getModels({
        base_url: form.base_url.trim(),
        key: form.key.trim(),
        protocol: form.protocol,
      });
      setCaptureModels(data.models);
      if (!form.model.trim() && data.models.length) {
        setForm((f) => ({ ...f, model: data.models[0], id: f.id || data.models[0] }));
      }
      if (!data.models.length) setCaptureError(tr("上游没有返回模型，仍可手动填写请求名。"));
    } catch (err) {
      setCaptureError((err as Error).message);
    } finally {
      setModelBusy(false);
    }
  }

  async function startCapture(e: FormEvent) {
    e.preventDefault();
    setCaptureBusy(true);
    setCaptureError("");
    try {
      await saveKey(form.key);
      try {
        localStorage.setItem(
          "chanshui-settings",
          JSON.stringify({ base_url: form.base_url.trim(), model: form.model, protocol: form.protocol }),
        );
      } catch {}
      const res = await fetch("/api/admin/fingerprints/capture", {
        method: "POST",
        credentials: "same-origin",
        headers: { "content-type": "application/json" },
        body: JSON.stringify(form),
      });
      const data = await res.json().catch(() => ({}));
      if (!res.ok) throw Error(data.error || tr("采集启动失败"));
      setCapture(data);
      setForm((f) => ({ ...f, key: "" }));
      setTab("fingerprint");
    } catch (err) {
      setCaptureError((err as Error).message);
    } finally {
      setCaptureBusy(false);
    }
  }

  async function cancelCapture() {
    if (!capture) return;
    await fetch(`/api/admin/fingerprints/capture/${encodeURIComponent(capture.id)}/cancel`, {
      method: "POST",
      credentials: "same-origin",
    });
  }

  async function removeReference(id: string) {
    if (!window.confirm(tr("删除这个自定义指纹？已有报告不会重算。"))) return;
    const res = await fetch(`/api/admin/fingerprints/${encodeURIComponent(id)}`, {
      method: "DELETE",
      credentials: "same-origin",
    });
    const data = await res.json().catch(() => ({}));
    if (!res.ok) {
      setError(data.error || tr("删除失败"));
      return;
    }
    await refresh();
  }

  useEffect(() => {
    if (!capture || !["running", "validating"].includes(capture.status)) return;
    const timer = window.setInterval(async () => {
      const res = await fetch(`/api/admin/fingerprints/capture/${encodeURIComponent(capture.id)}`, {
        cache: "no-store",
        credentials: "same-origin",
      });
      if (!res.ok) return;
      const data = await res.json().catch(() => null);
      if (!data) return;
      setCapture(data);
      if (!["running", "validating"].includes(data.status)) await load();
    }, 1200);
    return () => window.clearInterval(timer);
  }, [capture?.id, capture?.status]);

  function download() {
    if (!items) return;
    const blob = new Blob([JSON.stringify(items, null, 2)], { type: "application/json" });
    const url = URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.href = url;
    link.download = `chanshui-feedback-${new Date().toISOString().slice(0, 10)}.json`;
    link.click();
    URL.revokeObjectURL(url);
  }

  const percent = capture?.total
    ? Math.max(0, Math.min(100, Math.round((capture.completed / capture.total) * 100)))
    : 0;

  return (
    <div className="admin-shell">
      <header className="admin-head">
        <div>
          <span className="admin-kicker">{tr("管理员控制台")}</span>
          <h1>{items ? tr("反馈与指纹") : tr("管理员登录")}</h1>
          <p>{tr("查看用户反馈，维护自定义模型指纹。")}</p>
        </div>
        {items && (
          <button className="admin-button ghost" type="button" onClick={() => void logout()}>
            {tr("退出")}
          </button>
        )}
      </header>

      {checking ? (
        <section className="admin-card">
          <p className="muted">{tr("正在检查管理员会话…")}</p>
        </section>
      ) : items === null ? (
        <section className="admin-card admin-login">
          <div className="admin-login-icon" aria-hidden="true">
            <Icon name="shield" width="22" height="22" />
          </div>
          <h2>{tr("进入反馈后台")}</h2>
          <p className="muted">{tr("输入服务端配置的 CHANSHUI_ADMIN_TOKEN。")}</p>
          <form onSubmit={submit}>
            <div className="field">
              <label htmlFor="admin-token">{tr("管理员口令")}</label>
              <div className="input-action">
                <input
                  id="admin-token"
                  type={show ? "text" : "password"}
                  value={token}
                  autoComplete="current-password"
                  onChange={(e) => setToken(e.target.value)}
                  autoFocus
                  spellCheck={false}
                  autoCapitalize="off"
                  required
                />
                <button
                  type="button"
                  onClick={() => setShow((v) => !v)}
                  aria-label={show ? tr("隐藏口令") : tr("显示口令")}
                >
                  <Icon name={show ? "eyeOff" : "eye"} />
                </button>
              </div>
              <small className="admin-hint">{tr("通常是 48 位随机字符串。")}</small>
            </div>
            {error && (
              <div className="error-box" role="alert">
                {error}
              </div>
            )}
            <button className="admin-button primary wide" disabled={busy}>
              {busy ? tr("正在核对…") : tr("进入")}
            </button>
          </form>
        </section>
      ) : (
        <>
          <nav className="admin-tabs" aria-label={tr("后台模块")}>
            <button
              type="button"
              className={tab === "feedback" ? "active" : ""}
              onClick={() => setTab("feedback")}
            >
              {tr("反馈记录")}
              <span>{items.length}</span>
            </button>
            <button
              type="button"
              className={tab === "fingerprint" ? "active" : ""}
              onClick={() => setTab("fingerprint")}
            >
              {tr("指纹采集")}
              <span>{refs.length}</span>
            </button>
          </nav>

          {tab === "feedback" && (
            <section className="admin-card">
              <div className="admin-card-head">
                <div>
                  <h2>{tr("反馈记录")}</h2>
                  <p>{tr("联系方式仅用于反馈处理，不会公开。")}</p>
                </div>
                <div className="admin-actions">
                  <button className="admin-button ghost" type="button" onClick={() => void refresh()} disabled={refreshing}>
                    {refreshing ? tr("刷新中…") : tr("刷新")}
                  </button>
                  <button className="admin-button ghost" type="button" onClick={download}>
                    {tr("导出 JSON")}
                  </button>
                </div>
              </div>
              {error && (
                <div className="error-box" role="alert">
                  {error}
                </div>
              )}
              {items.length === 0 ? (
                <div className="admin-empty">
                  <Icon name="feedback" width="22" height="22" />
                  <b>{tr("还没有反馈")}</b>
                  <span>{tr("用户提交后会出现在这里。")}</span>
                </div>
              ) : (
                <div className="admin-list">
                  {items.map((item) => (
                    <article key={item.id}>
                      <header>
                        <b>{kinds[item.kind] || item.kind}</b>
                        <time>{new Date(item.created_at * 1000).toLocaleString("zh-CN", { hour12: false })}</time>
                      </header>
                      <p className="admin-contact">{item.contact}</p>
                      <p>{item.message}</p>
                    </article>
                  ))}
                </div>
              )}
            </section>
          )}

          {tab === "fingerprint" && (
            <section className="admin-card">
              <div className="admin-card-head">
                <div>
                  <h2>{tr("指纹采集与归因")}</h2>
                  <p>{tr("采集训练与保留样本，全部通过后写入归因库。")}</p>
                </div>
                <button className="admin-button ghost" type="button" onClick={() => void fillSaved()} disabled={fillBusy}>
                  {fillBusy ? tr("读取中…") : tr("填入上次保存")}
                </button>
              </div>

              <form className="admin-capture-form" onSubmit={startCapture}>
                <div className="admin-form-section">
                  <span className="admin-section-label">01 · {tr("连接上游")}</span>
                  <div className="admin-capture-grid">
                    <div className="field">
                      <label htmlFor="capture-base">{tr("接口地址")}</label>
                      <input
                        id="capture-base"
                        value={form.base_url}
                        onChange={(e) => setForm({ ...form, base_url: e.target.value })}
                        placeholder="https://gateway.example"
                        required
                      />
                    </div>
                    <div className="field">
                      <label htmlFor="capture-key">API Key</label>
                      <div className="input-action">
                        <input
                          id="capture-key"
                          type={showCaptureKey ? "text" : "password"}
                          value={form.key}
                          onChange={(e) => setForm({ ...form, key: e.target.value })}
                          placeholder={tr("只用于本次采集")}
                          autoComplete="off"
                          required
                        />
                        <button
                          type="button"
                          onClick={() => setShowCaptureKey((v) => !v)}
                          aria-label={showCaptureKey ? tr("隐藏 Key") : tr("显示 Key")}
                        >
                          <Icon name={showCaptureKey ? "eyeOff" : "eye"} />
                        </button>
                      </div>
                    </div>
                    <div className="field">
                      <label htmlFor="capture-model">{tr("请求模型名")}</label>
                      <input
                        id="capture-model"
                        value={form.model}
                        onChange={(e) => {
                          const value = e.target.value;
                          setForm((f) => ({ ...f, model: value, id: f.id || value.trim() }));
                        }}
                        placeholder="deepseek-flash"
                        list="capture-model-list"
                        required
                      />
                      <datalist id="capture-model-list">
                        {captureModels.map((name) => (
                          <option key={name} value={name} />
                        ))}
                      </datalist>
                      <button
                        className="admin-button ghost mini"
                        type="button"
                        onClick={() => void listCaptureModels()}
                        disabled={modelBusy}
                      >
                        {modelBusy ? tr("拉取中…") : tr("拉取模型")}
                      </button>
                    </div>
                    <div className="field">
                      <label htmlFor="capture-protocol">{tr("协议")}</label>
                      <Select
                        id="capture-protocol"
                        value={form.protocol}
                        onChange={(e) => setForm({ ...form, protocol: e.target.value as CaptureForm["protocol"] })}
                      >
                        <option value="auto">{tr("自适应")}</option>
                        <option value="chat">Chat</option>
                        <option value="anthropic">Anthropic</option>
                        <option value="openai">OpenAI</option>
                      </Select>
                    </div>
                  </div>
                </div>

                <div className="admin-form-section">
                  <span className="admin-section-label">02 · {tr("归因信息")}</span>
                  <div className="admin-capture-grid">
                    <div className="field">
                      <label htmlFor="capture-id">{tr("归因 ID")}</label>
                      <input
                        id="capture-id"
                        value={form.id}
                        onChange={(e) => setForm({ ...form, id: e.target.value })}
                        placeholder="deepseek-v4.1-flash-1017"
                      />
                    </div>
                    <div className="field">
                      <label htmlFor="capture-display">{tr("显示名称")}</label>
                      <input
                        id="capture-display"
                        value={form.display_name}
                        onChange={(e) => setForm({ ...form, display_name: e.target.value })}
                        placeholder={tr("留空则使用归因 ID")}
                      />
                    </div>
                    <div className="field global-field">
                      <label htmlFor="capture-aliases">{tr("别名（逗号分隔）")}</label>
                      <input
                        id="capture-aliases"
                        value={form.aliases}
                        onChange={(e) => setForm({ ...form, aliases: e.target.value })}
                        placeholder="deepseek-flash"
                      />
                    </div>
                  </div>
                </div>

                <div className="admin-form-section">
                  <span className="admin-section-label">03 · {tr("采样参数")}</span>
                  <div className="admin-capture-grid four">
                    <div className="field">
                      <label htmlFor="capture-train">{tr("训练样本")}</label>
                      <input
                        id="capture-train"
                        type="number"
                        min={30}
                        max={40}
                        value={form.train}
                        onChange={(e) => setForm({ ...form, train: Number(e.target.value) })}
                      />
                    </div>
                    <div className="field">
                      <label htmlFor="capture-holdout">{tr("保留样本")}</label>
                      <input
                        id="capture-holdout"
                        type="number"
                        min={9}
                        max={12}
                        step={3}
                        value={form.holdout}
                        onChange={(e) => setForm({ ...form, holdout: Number(e.target.value) })}
                      />
                    </div>
                    <div className="field">
                      <label htmlFor="capture-concurrency">{tr("并发")}</label>
                      <input
                        id="capture-concurrency"
                        type="number"
                        min={1}
                        max={4}
                        value={form.concurrency}
                        onChange={(e) => setForm({ ...form, concurrency: Number(e.target.value) })}
                      />
                    </div>
                    <div className="field">
                      <label htmlFor="capture-timeout">{tr("超时 / 秒")}</label>
                      <input
                        id="capture-timeout"
                        type="number"
                        min={10}
                        max={600}
                        value={form.timeout}
                        onChange={(e) => setForm({ ...form, timeout: Number(e.target.value) })}
                      />
                    </div>
                  </div>
                </div>

                {captureError && (
                  <div className="error-box" role="alert">
                    {captureError}
                  </div>
                )}
                <div className="admin-form-actions">
                  <button
                    className="admin-button primary"
                    disabled={captureBusy || (!!capture && ["running", "validating"].includes(capture.status))}
                  >
                    {captureBusy ? tr("正在启动…") : tr("开始采集")}
                  </button>
                  {capture && ["running", "validating"].includes(capture.status) && (
                    <button className="admin-button ghost" type="button" onClick={() => void cancelCapture()}>
                      {tr("取消")}
                    </button>
                  )}
                  <span className="admin-form-note">
                    {tr("保留样本全部命中新 ID 才会写入归因库。")}
                  </span>
                </div>
              </form>

              {capture && (
                <div className={`admin-progress ${capture.status}`}>
                  <div className="admin-progress-head">
                    <b>{tr(captureLabels[capture.status] || capture.status)}</b>
                    <span>
                      {capture.completed}/{capture.total} · {percent}%
                    </span>
                  </div>
                  <div className="admin-progress-bar">
                    <i style={{ width: `${percent}%` }} />
                  </div>
                  <div className="admin-progress-meta">
                    <span>
                      {tr("训练可用")} {capture.train_usable}/{capture.train_requested}
                    </span>
                    <span>
                      {tr("保留可用")} {capture.holdout_usable}/{capture.holdout_requested}
                    </span>
                  </div>
                  {capture.error && <p className="admin-error-text">{capture.error}</p>}
                  {capture.status === "done" && capture.reference && (
                    <p className="admin-success-text">
                      {tr("已加入归因库")}：{capture.reference.id}
                    </p>
                  )}
                  {!!capture.groups?.length && (
                    <ol className="admin-capture-groups">
                      {capture.groups.map((g) => (
                        <li key={g.group}>
                          <b className={g.pass ? "good" : "bad"}>{g.pass ? tr("通过") : tr("未通过")}</b>
                          <span>{g.top_display || g.top_model}</span>
                          <span>{(g.probability * 100).toFixed(2)}%</span>
                        </li>
                      ))}
                    </ol>
                  )}
                </div>
              )}

              <div className="admin-ref-list">
                <div className="admin-subhead">
                  <b>{tr("自定义指纹")}</b>
                  <span>{refs.length}</span>
                </div>
                {refs.length === 0 ? (
                  <p className="muted admin-empty-line">{tr("还没有自定义指纹。")}</p>
                ) : (
                  refs.map((ref) => (
                    <article key={ref.id}>
                      <div>
                        <b>{ref.display_name || ref.id}</b>
                        <small>
                          {ref.family_name} · {ref.samples} {tr("训练样本")}
                        </small>
                      </div>
                      <button className="admin-button ghost mini" type="button" onClick={() => void removeReference(ref.id)}>
                        {tr("删除")}
                      </button>
                    </article>
                  ))
                )}
              </div>
            </section>
          )}
        </>
      )}
    </div>
  );
}
