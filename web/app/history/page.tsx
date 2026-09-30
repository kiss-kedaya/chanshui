"use client";


import { useEffect, useState } from "react";
import { useSearchParams } from "next/navigation";
import { Suspense } from "react";
import { getDetection, DetectVerdict } from "@/lib/api";
import ResultCards from "@/components/ResultCards";

function HistoryDetailInner() {
  const search = useSearchParams();
  const id = search.get("id") || "";
  const [verdict, setVerdict] = useState<DetectVerdict | null>(null);
  const [meta, setMeta] = useState<{ base_url: string; model: string; created_at: string } | null>(null);
  const [error, setError] = useState("");

  useEffect(() => {
    getDetection(Number(id))
      .then((d) => {
        setVerdict(d.verdict_json);
        setMeta({ base_url: d.base_url, model: d.model, created_at: d.created_at });
      })
      .catch((e) => setError(e?.message || String(e)));
  }, [id]);

  return (
    <>
      <div className="hero-card">
        <h1 className="hero-title">
          {meta ? meta.model : "加载中…"}
          {meta && (
            <span style={{ fontSize: 13, fontWeight: 400, color: "var(--muted)", marginLeft: 10 }}>
              {meta.base_url.replace(/^https?:\/\//, "")} ·{" "}
              {new Date(meta.created_at).toLocaleString("zh-CN")}
            </span>
          )}
        </h1>
      </div>
      {error && <div className="hero-card" style={{ color: "var(--bad)" }}>{error}</div>}
      {verdict && <ResultCards verdict={verdict} />}
    </>
  );
}


export default function HistoryDetail() {
  return (
    <Suspense fallback={<div className="hero-card">加载中…</div>}>
      <HistoryDetailInner />
    </Suspense>
  );
}
