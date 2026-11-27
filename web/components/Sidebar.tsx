"use client";
import Feedback from "@/components/Feedback";
import Icon from "@/components/Icon";
import Link from "next/link";
import { Suspense, useCallback, useEffect, useRef, useState } from "react";
import { usePathname, useSearchParams } from "next/navigation";
import { reportPath } from "@/lib/report-view";
import { getHistory, HistorySummary } from "@/lib/api";
import { formatLocaleDate, useI18n } from "@/lib/i18n";

const HISTORY_REFRESH_MS = 45000;
const HISTORY_PAGE = 10;

export type ScrollCue = "none" | "top" | "bottom" | "both";

function isAbort(e: unknown): boolean {
  return Boolean(e && typeof e === "object" && (e as { name?: string }).name === "AbortError");
}

export function sidebarBadgeTone(
  item: Pick<HistorySummary, "anticheat_detected" | "total_score" | "iq_score">,
): string {
  if (item.anticheat_detected) return " is-bad";
  const score = item.total_score ?? item.iq_score;
  if (score == null) return " is-pending";
  if (score >= 85) return " is-ok";
  if (score >= 60) return " is-warn";
  return " is-bad";
}

export function isActiveReportItem(
  pathname: string,
  activeReportId: string,
  item: Pick<HistorySummary, "id" | "report_id">,
): boolean {
  if (pathname !== "/history" || !activeReportId) return false;
  return activeReportId === item.report_id || activeReportId === String(item.id);
}

export function computeScrollCue(el: {
  scrollTop: number;
  scrollHeight: number;
  clientHeight: number;
}): ScrollCue {
  const overflow = el.scrollHeight - el.clientHeight;
  if (overflow <= 6) return "none";
  const top = el.scrollTop > 6;
  const bottom = el.scrollTop < overflow - 6;
  if (top && bottom) return "both";
  if (top) return "top";
  if (bottom) return "bottom";
  return "none";
}

function SearchReportTracker({ onChange }: { onChange: (id: string) => void }) {
  const searchParams = useSearchParams();
  const id = searchParams?.get("id") || "";
  useEffect(() => {
    onChange(id);
  }, [id, onChange]);
  return null;
}

export default function Sidebar() {
  const { t: tr, locale } = useI18n();
  const [items, setItems] = useState<HistorySummary[]>([]),
    [error, setError] = useState("");
  const pathname = usePathname() || "/";
  const [activeReportId, setActiveReportId] = useState("");
  const [mobileOpen, setMobileOpen] = useState(false),
    [hasMore, setHasMore] = useState(false),
    [loadingMore, setLoadingMore] = useState(false),
    [ready, setReady] = useState(false),
    [scrollCue, setScrollCue] = useState<ScrollCue>("none");
  const alive = useRef(true),
    busy = useRef(false),
    lastRefresh = useRef(0),
    sentinel = useRef<HTMLDivElement | null>(null),
    listRef = useRef<HTMLDivElement | null>(null);

  const syncScrollCue = useCallback(() => {
    const el = listRef.current;
    if (!el) return;
    const next = computeScrollCue(el);
    setScrollCue((prev) => (prev === next ? prev : next));
  }, []);

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
              x.anticheat_detected === next[i].anticheat_detected &&
              x.created_at === next[i].created_at,
          )
        )
          return prev;
        return next;
      });
      setHasMore(page.length === HISTORY_PAGE);
      setError("");
    } catch (e) {
      if (alive.current && !isAbort(e)) setError((e as Error).message);
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
      if (alive.current && !isAbort(e)) setError((e as Error).message);
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
    setMobileOpen(false);
    void refresh(true);
  }, [pathname, refresh]);

  const visibleItems = items.filter((x) => (x.total_score ?? x.iq_score) !== 0 || x.anticheat_detected);

  useEffect(() => {
    syncScrollCue();
    const el = listRef.current;
    if (!el || typeof ResizeObserver !== "function") return;
    const ro = new ResizeObserver(() => syncScrollCue());
    ro.observe(el);
    return () => ro.disconnect();
  }, [visibleItems.length, mobileOpen, ready, syncScrollCue]);

  return (
    <aside className={"sidebar" + (mobileOpen ? " mobile-open" : "")}>
      <Suspense fallback={null}>
        <SearchReportTracker onChange={setActiveReportId} />
      </Suspense>
      <Link href="/" className="sidebar-logo">
        <span className="logo-symbol">?</span>
        <span>
          {tr("掺水")}<span className="logo-punct">？</span>
        </span>
      </Link>
      <div className="sidebar-sub">{tr("模型检测台")}</div>
      <nav className="sidebar-nav" aria-label={tr("侧边导航")}>
        <Link
          className={"nav-home" + (pathname === "/" ? " active" : "")}
          href="/"
        >
          <Icon name="scan" width="16" height="16" />
          <span>{tr("新建检测")}</span>
        </Link>
        <Link
          className={"nav-home nav-sub" + (pathname === "/history" ? " active" : "")}
          href="/history"
        >
          <Icon name="fileText" width="16" height="16" />
          <span>{tr("全站报告")}</span>
        </Link>
        <Link
          className={"nav-home nav-sub nav-docs" + (pathname === "/docs" ? " active" : "")}
          href="/docs"
        >
          <Icon name="layers" width="16" height="16" />
          <span>{tr("接口文档")}</span>
        </Link>
      </nav>
      <button
        type="button"
        className="mobile-history"
        aria-expanded={mobileOpen}
        aria-controls="sidebar-history-list"
        onClick={() => setMobileOpen(!mobileOpen)}
      >
        <Icon name="clock" width="16" height="16" />
        <span>{mobileOpen ? tr("收起历史") : tr("历史记录")}</span>
      </button>
      <div className="sidebar-title">
        <span>{tr("全站检测历史")}</span>
        <span>{ready ? visibleItems.length : "·"}</span>
      </div>
      <div
        id="sidebar-history-list"
        ref={listRef}
        className="sidebar-list"
        data-scroll-cue={scrollCue}
        onScroll={syncScrollCue}
      >
        {error && !visibleItems.length ? (
          <div className="sidebar-empty">
            <p>{tr(error)}</p>
            <button type="button" className="history-more" onClick={() => void refresh(true)}>
              {tr("重新加载")}
            </button>
          </div>
        ) : !ready ? (
          <div className="history-skeleton" aria-hidden="true">
            <i />
            <i />
            <i />
          </div>
        ) : !visibleItems.length ? (
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
          visibleItems.map((x) => {
            const score = x.total_score ?? x.iq_score;
            const badgeTone = sidebarBadgeTone(x);
            const active = isActiveReportItem(pathname, activeReportId, x);
            return (
              <Link
                className={"hist-item" + (active ? " is-active" : "")}
                key={x.id}
                href={reportPath(x.report_id)}
                prefetch={false}
                aria-current={active ? "page" : undefined}
              >
                <div className="history-model">
                  <span>{x.model}</span>
                  {x.anticheat_detected && <em className="hist-cheat-tag">{tr("作假")}</em>}
                </div>
                <div className="history-meta">
                  <b className={"hist-badge" + badgeTone} title={x.total_score == null ? tr("旧报告只有推理分") : tr("总分")}>
                    {score == null ? (
                      <>
                        <i className="live-dot live-dot--pending" aria-hidden="true" />
                        {tr("待完成")}
                      </>
                    ) : (
                      tr("{n} 分", { n: score })
                    )}
                  </b>
                  <time className="history-time" dateTime={x.created_at}>
                    {formatLocaleDate(x.created_at, locale, {
                      month: "2-digit",
                      day: "2-digit",
                      hour: "2-digit",
                      minute: "2-digit",
                    })}
                  </time>
                </div>
              </Link>
            );
          })
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
