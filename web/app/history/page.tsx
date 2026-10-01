"use client";
import { Suspense, useEffect, useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import Link from "next/link";
import { getDetection, getReport, SavedReport } from "@/lib/api";
import { parseReportID, reportHost, reportPath } from "@/lib/report-view";
import { getJobControl, reconcileJobControl } from "@/lib/job-controls";
import { watchResource } from "@/lib/polling";
import ResultCards from "@/components/ResultCards";
import ProbeTimeline from "@/components/ProbeTimeline";
import ReportShare from "@/components/ReportShare";
function HistoryDetailInner() {
  const search = useSearchParams(),
    router = useRouter();
  const id = search.get("id") || "";
  const [data, setData] = useState<SavedReport | null>(null),
    [error, setError] = useState(""),
    [entry, setEntry] = useState(id),
    [loading, setLoading] = useState(false),
    [entryError, setEntryError] = useState(""),
    [canRetry, setCanRetry] = useState(false);
  useEffect(() => {
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
      setError("检测 ID 无效，请粘贴完整 ID 或报告链接");
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
  }, [id]);
  return (
    <div className="page-shell history-page">
      <section className="hero-card">
        <div className="section-heading">
          <div>
            <span className="step">报告</span>
            <h2>按检测 ID 查看</h2>
          </div>
          <span className="muted">全站报告</span>
        </div>
        <form
          className="report-lookup"
          onSubmit={(e) => {
            e.preventDefault();
            const parsed = parseReportID(entry);
            if (!parsed) {
              setEntryError("请填写完整的 32 位检测 ID 或报告链接");
              return;
            }
            setEntryError("");
            router.push(reportPath(parsed));
          }}
        >
          <label className="sr-only" htmlFor="report-id">
            检测 ID 或报告链接
          </label>
          <input
            id="report-id"
            value={entry}
            onChange={(e) => setEntry(e.target.value)}
            placeholder="粘贴检测 ID 或报告链接"
            spellCheck={false}
          />
          <button className="btn-primary" type="submit">
            查看报告
          </button>
        </form>
        {entryError && (
          <p className="error-box" role="alert">
            {entryError}
          </p>
        )}
        <h1 className="hero-title">
          {data
            ? data.model
            : loading
              ? "正在加载报告…"
              : error
                ? "报告不可用"
                : "全站检测报告"}
        </h1>
        {data && (
          <p className="report-meta">
            <span>{data.host || reportHost(data.base_url)}</span> ·{" "}
            {new Date(data.created_at).toLocaleString("zh-CN")}
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
          <Link
            className="btn-secondary"
            href={"/?job=" + encodeURIComponent(data.report_id)}
            prefetch={false}
          >
            重试失败项
          </Link>
        )}
        {!data && !error && !loading && (
          <p className="muted">粘贴 Job ID，或从左侧历史选择报告。</p>
        )}
        {error && (
          <div className="error-box" role="alert">
            {error}
          </div>
        )}
      </section>
      {data && (
        <ProbeTimeline
          probes={data.probes || []}
          status={data.status}
          timing={data.verdict_json.timing}
        />
      )}
      {data && (
        <ResultCards
          verdict={data.verdict_json}
          model={data.model}
          loading={["running", "queued"].includes(data.status)}
        />
      )}
    </div>
  );
}
export default function HistoryDetail() {
  return (
    <Suspense fallback={<div className="hero-card">正在加载报告…</div>}>
      <HistoryDetailInner />
    </Suspense>
  );
}
