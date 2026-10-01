"use client";
import Feedback from "@/components/Feedback";
import Icon from "@/components/Icon";
import Link from "next/link";
import { useCallback, useEffect, useRef, useState } from "react";
import { usePathname } from "next/navigation";
import { reportPath } from "@/lib/report-view";
import { getHistory, HistorySummary } from "@/lib/api";

const HISTORY_REFRESH_MS = 45000;

export default function Sidebar() {
  const [items, setItems] = useState<HistorySummary[]>([]),
    [error, setError] = useState("");
  const pathname = usePathname();
  const [mobileOpen, setMobileOpen] = useState(false),
    [hasMore, setHasMore] = useState(false),
    [loadingMore, setLoadingMore] = useState(false);
  const alive = useRef(true),
    busy = useRef(false),
    lastRefresh = useRef(0);
  const refresh = useCallback(async (force = false) => {
    if (
      busy.current ||
      document.visibilityState === "hidden" ||
      navigator.onLine === false ||
      (!force && Date.now() - lastRefresh.current < HISTORY_REFRESH_MS)
    ) return;
    busy.current = true;
    lastRefresh.current = Date.now();
    try {
      const page = await getHistory(50);
      if (!alive.current) return;
      setItems((prev) => {
        const older =
          page.length === 50
            ? prev.filter((x) => x.id < page[page.length - 1].id)
            : [];
        const next = [...page, ...older];
        return JSON.stringify(prev) === JSON.stringify(next) ? prev : next;
      });
      setHasMore(page.length === 50);
      setError("");
    } catch (e) {
      if (alive.current) setError((e as Error).message);
    } finally {
      busy.current = false;
    }
  }, []);
  async function more() {
    if (busy.current || !items.length) return;
    busy.current = true;
    setLoadingMore(true);
    try {
      const page = await getHistory(50, items[items.length - 1].id);
      if (!alive.current) return;
      setItems((prev) => {
        const ids = new Set(prev.map((x) => x.id));
        return [...prev, ...page.filter((x) => !ids.has(x.id))];
      });
      setHasMore(page.length === 50);
      setError("");
    } catch (e) {
      if (alive.current) setError((e as Error).message);
    } finally {
      busy.current = false;
      if (alive.current) setLoadingMore(false);
    }
  }
  useEffect(() => {
    alive.current = true;
    void refresh();
    const load = () => { void refresh(); };
    const updated = () => { void refresh(true); };
    const timer = window.setInterval(load, HISTORY_REFRESH_MS);
    window.addEventListener("history-updated", updated);
    window.addEventListener("focus", load);
    window.addEventListener("online", load);
    document.addEventListener("visibilitychange", load);
    return () => {
      alive.current = false;
      window.clearInterval(timer);
      window.removeEventListener("history-updated", updated);
      window.removeEventListener("focus", load);
      window.removeEventListener("online", load);
      document.removeEventListener("visibilitychange", load);
    };
  }, [refresh]);
  useEffect(() => {
    void refresh();
  }, [pathname, refresh]);
  return (
    <aside className={"sidebar " + (mobileOpen ? "mobile-open" : "")}>
      <Link href="/" className="sidebar-logo">
        <span className="logo-symbol">?</span>
        <span>
          掺水<span className="logo-punct">？</span>
        </span>
      </Link>
      <div className="sidebar-sub">模型检测台</div>
      <Link
        className={"nav-home " + (pathname === "/" ? "active" : "")}
        href="/"
      >
        <Icon name="scan" /> 新建检测
      </Link>
      <button
        className="mobile-history"
        aria-expanded={mobileOpen}
        onClick={() => setMobileOpen(!mobileOpen)}
      >
        {mobileOpen ? "收起历史" : "历史记录"}
      </button>
      <div className="sidebar-title">
        <span>全站检测历史</span>
        <span>{items.length}</span>
      </div>
      <div className="sidebar-list">
        {error ? (
          <p className="sidebar-empty">{error}</p>
        ) : !items.length ? (
          <div className="sidebar-empty">
            <div className="empty-lines">
              <i />
              <i />
              <i />
            </div>
            还没有检测记录
            <br />
            <small>所有访客的检测报告都会出现在这里</small>
          </div>
        ) : (
          items.map((x) => (
            <Link
              className="hist-item"
              key={x.id}
              href={reportPath(x.report_id)}
              prefetch={false}
            >
              <div className="history-model">{x.model}</div>
              <div className="history-meta">
                <b className="hist-badge" title={x.total_score == null ? "旧报告只有推理分" : "总分"}>
                  {(x.total_score ?? x.iq_score) == null
                    ? "待完成"
                    : (x.total_score ?? x.iq_score) + " 分"}
                </b>
                <time className="history-time" dateTime={x.created_at}>
                  {new Date(x.created_at).toLocaleString("zh-CN", {
                    month: "2-digit",
                    day: "2-digit",
                    hour: "2-digit",
                    minute: "2-digit",
                  })}
                </time>
              </div>
              <div className="history-report-id" title={x.report_id}>
                ID {x.report_id.slice(0, 12)}…
              </div>
            </Link>
          ))
        )}
        {hasMore && (
          <button
            className="history-more"
            onClick={more}
            disabled={loadingMore}
          >
            {loadingMore ? "正在加载…" : "加载更多全站记录"}
          </button>
        )}
      </div>
      <div className="sidebar-bottom">
        <Feedback />
        <span className="sidebar-retention">
          <Icon name="clock" width="14" height="14" />
          报告保留 24 小时
        </span>
      </div>
    </aside>
  );
}
