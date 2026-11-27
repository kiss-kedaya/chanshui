"use client";
import { useI18n } from "@/lib/i18n";
export default function ReportStage({
  label = "正在读取报告",
}: {
  label?: string;
}) {
  const { t } = useI18n();
  const text = t(label);
  return (
    <section className="report-stage" role="status" aria-live="polite" aria-label={text}>
      <div className="report-stage-hero">
        <div className="stage-status">
          <i className="stage-spin is-round" aria-hidden="true" />
          <span>{text}</span>
        </div>
        <h2 className="stage-title">{t("全站检测报告")}</h2>
        <div className="stage-track is-pill" aria-hidden="true"><i /></div>
        <div className="stage-emblem is-round" aria-hidden="true">
          <svg className="stage-emblem-svg" viewBox="0 0 132 132" aria-hidden="true">
            <circle className="stage-emblem-ring" cx="66" cy="66" r="60" fill="none" strokeWidth="1.25" />
            <circle
              className="stage-emblem-orbit"
              cx="66"
              cy="66"
              r="49"
              fill="none"
              strokeWidth="1.25"
              strokeLinecap="round"
              strokeDasharray="6 6"
            />
          </svg>
          <span>?</span>
        </div>
      </div>
      <div className="stage-grid" aria-hidden="true">
        {["模型指纹归因", "注入大小", "推理表现", "接口完整性"].map((key, index) => (
          <article className="stage-card" key={key}>
            <header><span>{t(key)}</span><small>0{index + 1}</small></header>
            <b />
            <i />
          </article>
        ))}
      </div>
    </section>
  );
}
