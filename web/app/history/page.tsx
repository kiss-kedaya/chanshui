"use client";
import { Suspense, useEffect, useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import Link from "next/link";
import { getDetection, getHistory, getReport, retryDetect, HistorySummary, SavedReport } from "@/lib/api";
import { parseReportID, reportPath } from "@/lib/report-view";
import { getJobControl, reconcileJobControl } from "@/lib/job-controls";
import { watchResource } from "@/lib/polling";
import ResultCards from "@/components/ResultCards";
import ProbeTimeline from "@/components/ProbeTimeline";
import ErrorFold from "@/components/ErrorFold";
import Icon from "@/components/Icon";
import { loadKey } from "@/lib/keyvault";
import ReportShare from "@/components/ReportShare";
import ReportStage from "@/components/ReportStage";
import { useI18n } from "@/lib/i18n";
function ReportBoot() {
  const { t: tr } = useI18n();
  return (
    <div className="page-shell history-page">
      <section className="hero-card report-boot-card">
        <div className="section-heading">
          <div>
            <span className="step">{tr("报告")}</span>
            <h2>{tr("按检测 ID 查看")}</h2>
          </div>
          <span className="muted">{tr("全站报告")}</span>
        </div>
        <div className="report-lookup" aria-hidden="true">
          <span className="stage-input" />
          <span className="stage-button" />
        </div>
      </section>
      <ReportStage />
    </div>
  );
}

function ReportIndex() {
  const { t: tr, locale } = useI18n();
  const [items, setItems] = useState<HistorySummary[] | null>(null);
  useEffect(() => {
    let alive = true;
    getHistory(12)
      .then((rows) => {
        if (alive) setItems(rows);
      })
      .catch(() => {
        if (alive) setItems([]);
      });
    return () => {
      alive = false;
    };
  }, []);
  const rows = (items || []).filter((row) => (row.total_score ?? row.iq_score) !== 0);
  return (
    <section className="report-index" aria-label={tr("最近检测")}>
      {items === null
        ? [0, 1, 2].map((n) => <i className="report-index-wait" key={n} />)
        : rows.map((row) => {
            const score = row.total_score ?? row.iq_score;
            return (
              <Link key={row.report_id} href={reportPath(row.report_id)} prefetch={false}>
                <b>{score == null ? "—" : score}</b>
                <span>{row.model}</span>
                <time dateTime={row.created_at}>
                  {new Date(row.created_at).toLocaleString(locale, {
                    month: "2-digit",
                    day: "2-digit",
                    hour: "2-digit",
                    minute: "2-digit",
                  })}
                </time>
              </Link>
            );
          })}
    </section>
  );
}

function HistoryDetailInner() {
  const { t: tr } = useI18n();
  const search = useSearchParams(),
    router = useRouter();
  const id = search.get("id") || "";
  const [data, setData] = useState<SavedReport | null>(null),
    [error, setError] = useState(""),
    [entry, setEntry] = useState(id),
    [loading, setLoading] = useState(!!id),
    [entryError, setEntryError] = useState(""),
    [canRetry, setCanRetry] = useState(false);
  const [loadedID, setLoadedID] = useState(id);
  const [reload, setReload] = useState(0);
  const [retrying, setRetrying] = useState(false);
  const [retryError, setRetryError] = useState("");
  // 本地没有保存的 Key 时，允许用户在这个页面临时补填一次（不落盘）。
  const [retryKey, setRetryKey] = useState("");
  useEffect(() => {
    setLoadedID(id);
    setData(null);
    setError("");
    setEntry(id);
    setEntryError("");
    setCanRetry(false);
    if (!id) {
      setLoading(false);
      return;
    }
    const publicID = parseReportID(id);
    const legacy = /^[1-9]\d*$/.test(id) && Number.isSafeInteger(Number(id));
    if (!publicID && !legacy) {
      setLoading(false);
      setError(tr("检测 ID 无效，请粘贴完整 ID 或报告链接"));
      return;
    }
    setLoading(true);
    return watchResource<SavedReport>({
      load: async (signal) => {
        if (publicID) return getReport(publicID, signal, getJobControl(publicID)?.token);
        const first = await getDetection(Number(id), signal);
        // Legacy numeric links do not carry the report ID in the URL. Resolve
        // it once, then repeat privately when this browser owns its key.
        const token = getJobControl(first.report_id)?.token;
        return token ? getDetection(Number(id), signal, token) : first;
      },
      onValue: (d) => {
        reconcileJobControl({
          id: d.report_id,
          status: d.status,
          expires_at: d.expires_at,
          probes: d.probes,
        });
        setData(d);
        setLoading(false);
        setCanRetry(
          !!getJobControl(d.report_id) &&
          !["queued", "running"].includes(d.status) &&
          (d.probes || []).some((p) => p.status === "error" || p.status === "cancelled"),
        );
      },
      isActive: (d) => ["queued", "running"].includes(d.status),
      onError: (e) => {
        setError(e.message);
        setLoading(false);
      },
    });
  }, [id, reload]);
  // 重跑失败项：地址与模型取自本任务，Key 从本地加固存储读回，无需重填。
  // 鉴权优先用本地 jobkey；没有（换了浏览器/清了存储）时服务端用访客 ID 认。
  const retry = async () => {
    if (!data || retrying) return;
    setRetryError("");
    setRetrying(true);
    try {
      const saved = retryKey.trim() || (await loadKey());
      if (!saved) {
        setRetryError(tr("本地没有保存的 API Key，请回新建检测页填一次。"));
        setRetrying(false);
        return;
      }
      const control = getJobControl(data.report_id);
      await retryDetect(
        data.report_id,
        { base_url: data.base_url, key: saved, model: data.model, protocol: "auto" },
        control?.token || "",
      );
      setRetryKey("");
      setReload((n) => n + 1);
      window.dispatchEvent(new Event("history-updated"));
    } catch (e) {
      setRetryError((e as Error).message);
    } finally {
      setRetrying(false);
    }
  };
  if (loadedID !== id) return <ReportBoot />;
  return (
    <div className="page-shell history-page">
      <section className="hero-card">
        <div className="section-heading">
          <div>
            <span className="step">{tr("报告")}</span>
            <h2>{tr("按检测 ID 查看")}</h2>
          </div>
          <span className="muted">{tr("全站报告")}</span>
        </div>
        <form
          className="report-lookup"
          onSubmit={(e) => {
            e.preventDefault();
            const parsed = parseReportID(entry);
            if (!parsed) {
              setEntryError(tr("请填写完整的 32 位检测 ID 或报告链接"));
              return;
            }
            setEntryError("");
            router.push(reportPath(parsed));
          }}
        >
          <label className="sr-only" htmlFor="report-id">
            {tr("检测 ID 或报告链接")}
          </label>
          <input
            id="report-id"
            value={entry}
            onChange={(e) => setEntry(e.target.value)}
            placeholder={tr("粘贴检测 ID 或报告链接")}
            spellCheck={false}
          />
          <button className="btn-primary" type="submit">
            {tr("查看报告")}
          </button>
        </form>
        {entryError && <ErrorFold text={entryError} />}
        {data ? <h1 className="hero-title">{data.model}</h1> : null}
        {!data && error ? <h1 className="hero-title">{tr("报告不可用")}</h1> : null}
        {!data && !error && !loading ? (
          <h1 className="hero-title">{tr("全站检测报告")}</h1>
        ) : null}
        {data && (
          <p className="report-meta">
            <time dateTime={data.created_at}>{new Date(data.created_at).toLocaleString("zh-CN")}</time>
          </p>
        )}
        {data && (
          <ReportShare
            key={data.report_id}
            id={data.report_id}
            expiresAt={data.expires_at}
          />
        )}
        {data && canRetry && (
          <div className="retry-block">
            <button
              type="button"
              className="btn-retry btn-retry-block"
              onClick={() => void retry()}
              disabled={retrying}
              aria-busy={retrying}
            >
              <Icon name={retrying ? "spinner" : "refresh"} className={retrying ? "icon-spin" : ""} />
              {retrying ? tr("正在重跑失败项…") : tr("重试失败项")}
            </button>
            {retryError && <ErrorFold text={retryError} />}
          </div>
        )}
        {!data && !error && !loading && <ReportIndex />}
        {error && (
          <div className="report-load-error">
            <ErrorFold text={error} />
            <button type="button" className="btn-secondary" onClick={() => setReload((n) => n + 1)}>
              {tr("重新加载")}
            </button>
          </div>
        )}
      </section>
      {loading && !data && !error ? <ReportStage /> : null}
      {data && (
        <ResultCards
          verdict={data.verdict_json}
          model={data.model}
          loading={["running", "queued"].includes(data.status)}
          showCache={data.probes?.some((p) => p.id === "cache")}
        />
      )}
      {data && (
        <ProbeTimeline
          probes={data.probes || []}
          status={data.status}
          timing={data.verdict_json?.timing}
        />
      )}
    </div>
  );
}
export default function HistoryDetail() {
  return (
    <Suspense fallback={<ReportBoot />}>
      <HistoryDetailInner />
    </Suspense>
  );
}
