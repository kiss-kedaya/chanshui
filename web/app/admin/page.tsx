"use client";
import Icon from "@/components/Icon";
import { FormEvent, useEffect, useState } from "react";

type Item = {
  id: string;
  created_at: number;
  kind: string;
  contact: string;
  message: string;
};

const kinds: Record<string, string> = {
  bug: "问题反馈",
  method: "检测方法",
  suggestion: "改进建议",
};

export default function AdminFeedbackPage() {
  const [token, setToken] = useState(""),
    [show, setShow] = useState(false),
    [items, setItems] = useState<Item[] | null>(null),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false);

  async function load() {
    const res = await fetch("/api/admin/feedback", { cache: "no-store", credentials: "same-origin" });
    if (res.status === 401) {
      setItems(null);
      return;
    }
    const data = await res.json().catch(() => ({}));
    if (!res.ok) throw Error(data.error || "读取失败");
    setItems(data.items || []);
  }

  useEffect(() => {
    load().catch(() => setItems(null));
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
      if (!res.ok) throw Error(data.error || "登录失败");
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
  }

  return (
    <section className="result-card admin-feedback">
      <div className="rc-head">
        <h3>反馈记录</h3>
        {items && (
          <button className="btn-secondary" type="button" onClick={logout}>
            退出
          </button>
        )}
      </div>
      {items === null ? (
        <form onSubmit={submit}>
          <div className="field">
            <label htmlFor="admin-token">管理员口令</label>
            <div className="input-action">
              <input
                id="admin-token"
                type={show ? "text" : "password"}
                value={token}
                autoComplete="current-password"
                onChange={(e) => setToken(e.target.value)}
                required
              />
              <button type="button" onClick={() => setShow((v) => !v)} aria-label={show ? "隐藏口令" : "显示口令"}>
                <Icon name={show ? "eyeOff" : "eye"} />
              </button>
            </div>
          </div>
          {error && <div className="error-box">{error}</div>}
          <button className="btn-primary" disabled={busy || token.length < 24}>
            {busy ? "正在核对…" : "进入"}
          </button>
        </form>
      ) : items.length === 0 ? (
        <p className="muted">还没有反馈</p>
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
