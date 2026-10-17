"use client";
import Icon from "@/components/Icon";
import { FormEvent, useEffect, useState } from "react";
import { useI18n } from "@/lib/i18n";

type Item = {
  id: string;
  created_at: number;
  kind: string;
  contact: string;
  message: string;
};

export default function AdminFeedbackPage() {
  const { t: tr } = useI18n();
  const kinds: Record<string, string> = {
    bug: tr("问题反馈"),
    method: tr("检测方法"),
    suggestion: tr("改进建议"),
  };
  const [token, setToken] = useState(""),
    [show, setShow] = useState(false),
    [items, setItems] = useState<Item[] | null>(null),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false),
    [checking, setChecking] = useState(true),
    [refreshing, setRefreshing] = useState(false);

  async function load() {
    const res = await fetch("/api/admin/feedback", { cache: "no-store", credentials: "same-origin" });
    if (res.status === 401) {
      setItems(null);
      return;
    }
    const data = await res.json().catch(() => ({}));
    if (!res.ok) throw Error(data.error || tr("读取失败"));
    setItems(data.items || []);
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
    setError("");
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

  function download() {
    if (!items) return;
    const blob = new Blob([JSON.stringify(items, null, 2)], {
      type: "application/json",
    });
    const url = URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.href = url;
    link.download = `chanshui-feedback-${new Date().toISOString().slice(0, 10)}.json`;
    link.click();
    URL.revokeObjectURL(url);
  }

  return (
    <section className="result-card admin-feedback">
      <div className="rc-head">
        <h3>
          {tr("反馈记录")}
          {items ? ` (${items.length})` : ""}
        </h3>
        {items && (
          <div className="admin-actions">
            <button className="btn-secondary" type="button" onClick={() => void refresh()} disabled={refreshing}>
              {refreshing ? tr("刷新中…") : tr("刷新")}
            </button>
            <button className="btn-secondary" type="button" onClick={download}>
              {tr("导出 JSON")}
            </button>
            <button className="btn-secondary" type="button" onClick={() => void logout()}>
              {tr("退出")}
            </button>
          </div>
        )}
      </div>
      {checking ? (
        <p className="muted">{tr("正在检查管理员会话…")}</p>
      ) : items === null ? (
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
              <button type="button" onClick={() => setShow((v) => !v)} aria-label={show ? tr("隐藏口令") : tr("显示口令")}>
                <Icon name={show ? "eyeOff" : "eye"} />
              </button>
            </div>
            <small className="admin-hint">
              {tr("填写服务端配置的 CHANSHUI_ADMIN_TOKEN，通常为 48 位。")}
            </small>
          </div>
          {error && <div className="error-box" role="alert">{error}</div>}
          <button className="btn-primary" disabled={busy}>
            {busy ? tr("正在核对…") : tr("进入")}
          </button>
        </form>
      ) : items.length === 0 ? (
        <p className="muted">{tr("还没有反馈")}</p>
      ) : (
        <div className="admin-list">
          {items.map((item) => (
            <article key={item.id}>
              <header>
                <b>{kinds[item.kind] || item.kind}</b>
                <time>
                  {new Date(item.created_at * 1000).toLocaleString("zh-CN", { hour12: false })}
                </time>
              </header>
              <p className="admin-contact">{item.contact}</p>
              <p>{item.message}</p>
            </article>
          ))}
        </div>
      )}
    </section>
  );
}
