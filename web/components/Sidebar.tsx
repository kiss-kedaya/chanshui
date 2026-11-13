"use client";
import Feedback from "@/components/Feedback";
import Icon from "@/components/Icon";
import Link from "next/link";
import { useCallback, useEffect, useRef, useState } from "react";
import { usePathname } from "next/navigation";
import { reportPath } from "@/lib/report-view";
import { getHistory, HistorySummary } from "@/lib/api";
import { useI18n } from "@/lib/i18n";

const HISTORY_REFRESH_MS = 45000;
const HISTORY_PAGE = 10;

export default function Sidebar() {
  const { t: tr, locale } = useI18n();
  const [items, setItems] = useState<HistorySummary[]>([]),
    [error, setError] = useState("");
  const pathname = usePathname();
  const [mobileOpen, setMobileOpen] = useState(false),
    [hasMore, setHasMore] = useState(false),
    [loadingMore, setLoadingMore] = useState(false),
    [ready, setReady] = useState(false);
  const alive = useRef(true),
    busy = useRef(false),
    lastRefresh = useRef(0),
    sentinel = useRef<HTMLDivElement | null>(null);
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
      const page = await getHistory(HISTORY_PAGE);
      if (!alive.current) return;
      setItems((prev) => {
        const older =
          page.length === HISTORY_PAGE
            ? prev.filter((x) => x.id < page[page.length - 1].id)
            : [];
        const next = [...page, ...older];
        if (
          prev.length === next.length &&
          prev.every(
            (x, i) =>
              x.id === next[i].id &&
              x.model === next[i].model &&
              x.total_score === next[i].total_score &&
              x.iq_score === next[i].iq_score &&
              x.created_at === next[i].created_at,
          )
        )
          return prev;
        return next;
      });
      setHasMore(page.length === HISTORY_PAGE);
      setError("");
    } catch (e) {
      if (alive.current) setError((e as Error).message);
    } finally {
      busy.current = false;
      if (alive.current) setReady(true);
    }
  }, []);
  const more = useCallback(async () => {
    if (busy.current || !items.length) return;
    busy.current = true;
    setLoadingMore(true);
    try {
      const page = await getHistory(HISTORY_PAGE, items[items.length - 1].id);
      if (!alive.current) return;
      setItems((prev) => {
        const ids = new Set(prev.map((x) => x.id));
        return [...prev, ...page.filter((x) => !ids.has(x.id))];
      });
      setHasMore(page.length === HISTORY_PAGE);
      setError("");
    } catch (e) {
      if (alive.current) setError((e as Error).message);
    } finally {
      busy.current = false;
      if (alive.current) setLoadingMore(false);
    }
  }, [items]);
  useEffect(() => {
    const node = sentinel.current;
    if (!node || !hasMore) return;
    const io = new IntersectionObserver(
      (rows) => {
        if (rows.some((row) => row.isIntersecting)) void more();
      },
      { root: node.parentElement, rootMargin: "160px" },
    );
    io.observe(node);
    return () => io.disconnect();
  }, [hasMore, loadingMore, more]);
  useEffect(() => {
    alive.current = true;
    let idle = 0;
    let wait = 0;
    const start = () => {
      void refresh();
    };
    if (typeof window.requestIdleCallback === "function") {
      idle = window.requestIdleCallback(start, { timeout: 480 });
    } else {
      wait = window.setTimeout(start, 60);
    }
    const load = () => { void refresh(); };
    const updated = () => { void refresh(true); };
    const timer = window.setInterval(load, HISTORY_REFRESH_MS);
    window.addEventListener("history-updated", updated);
    window.addEventListener("focus", load);
    window.addEventListener("online", load);
    document.addEventListener("visibilitychange", load);
    return () => {
      alive.current = false;
      if (idle) window.cancelIdleCallback(idle);
      if (wait) window.clearTimeout(wait);
      window.clearInterval(timer);
      window.removeEventListener("history-updated", updated);
      window.removeEventListener("focus", load);
      window.removeEventListener("online", load);
      document.removeEventListener("visibilitychange", load);
    };
  }, [refresh]);
  const sawPath = useRef(false);
  useEffect(() => {
    if (!sawPath.current) {
      sawPath.current = true;
      return;
    }
    void refresh(true);
  }, [pathname, refresh]);
  return (
    <aside className={"sidebar " + (mobileOpen ? "mobile-open" : "")}>
      <Link href="/" className="sidebar-logo">
        <span className="logo-symbol">?</span>
        <span>
          {tr("掺水")}<span className="logo-punct">？</span>
        </span>
      </Link>
      <div className="sidebar-sub">{tr("模型检测台")}</div>
      <Link
        className={"nav-home " + (pathname === "/" ? "active" : "")}
        href="/"
      >
        <Icon name="scan" /> {tr("新建检测")}
      </Link>
      <Link
        className={"nav-home nav-docs" + (pathname === "/docs" ? " active" : "")}
        href="/docs"
      >
        <Icon name="scan" /> {tr("接口文档")}
      </Link>
      <button
        className="mobile-history"
        aria-expanded={mobileOpen}
        onClick={() => setMobileOpen(!mobileOpen)}
      >
        <Icon name="clock" width="16" height="16" />
        <span>{mobileOpen ? tr("收起历史") : tr("历史记录")}</span>
      </button>
      <div className="sidebar-title">
        <span>{tr("全站检测历史")}</span>
        <span>{ready ? items.filter((x) => (x.total_score ?? x.iq_score) !== 0).length : "·"}</span>
      </div>
      <div className="sidebar-list">
        {error ? (
          <p className="sidebar-empty">{error}</p>
        ) : !ready ? (
          <div className="history-skeleton" aria-hidden="true">
            <i />
            <i />
            <i />
          </div>
        ) : !items.filter((x) => (x.total_score ?? x.iq_score) !== 0).length ? (
          <div className="sidebar-empty">
            <div className="empty-lines">
              <i />
              <i />
              <i />
            </div>
            {tr("还没有检测记录")}
            <br />
            <small>{tr("所有访客的检测报告都会出现在这里")}</small>
          </div>
        ) : (
          items.filter((x) => (x.total_score ?? x.iq_score) !== 0).map((x) => (
            <Link
              className="hist-item"
              key={x.id}
              href={reportPath(x.report_id)}
              prefetch={false}
            >
              <div className="history-model">{x.model}</div>
              <div className="history-meta">
                <b className="hist-badge" title={x.total_score == null ? tr("旧报告只有推理分") : tr("总分")}>
                  {(x.total_score ?? x.iq_score) == null
                    ? tr("待完成")
                    : tr("{n} 分", { n: (x.total_score ?? x.iq_score) ?? 0 })}
                </b>
                <time className="history-time" dateTime={x.created_at}>
                  {new Date(x.created_at).toLocaleString(locale, {
                    month: "2-digit",
                    day: "2-digit",
                    hour: "2-digit",
                    minute: "2-digit",
                  })}
                </time>
              </div>
            </Link>
          ))
        )}
        {hasMore && <div ref={sentinel} className="history-sentinel" aria-hidden="true" />}
      </div>
      <div className="sidebar-bottom">
        <Feedback />
        <span className="sidebar-retention">
          <Icon name="clock" width="14" height="14" />
          {tr("报告保留 24 小时")}
        </span>
      </div>
    </aside>
  );
}
