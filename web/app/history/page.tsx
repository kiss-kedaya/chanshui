"use client";
import { Suspense, useEffect, useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { getDetection, getReport, SavedReport } from "@/lib/api";
import { parseReportID, reportHost, reportPath } from "@/lib/report-view";
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
    [entryError, setEntryError] = useState("");
  useEffect(() => {
    let active = true;
    setData(null);
    setError("");
    setEntry(id);
    setEntryError("");
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
    let timer: ReturnType<typeof setTimeout>;
    async function load() {
      try {
        const d = await (publicID
          ? getReport(publicID)
          : getDetection(Number(id)));
        if (!active) return;
        setData(d);
        setLoading(false);
        if (["queued", "running"].includes(d.status))
          timer = setTimeout(load, 1800);
      } catch (e) {
        if (active) {
          setError((e as Error).message);
          setLoading(false);
        }
      }
    }
    void load();
    return () => {
      active = false;
      clearTimeout(timer);
    };
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
        {!data && !error && !loading && (
          <p className="muted">粘贴 Job ID，或从左侧历史选择报告。</p>
        )}
        {error && (
          <div className="error-box" role="alert">
            {error}
          </div>
        )}
      </section>
      {data && ["running", "queued"].includes(data.status) && (
        <ProbeTimeline probes={data.probes || []} status={data.status} />
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
