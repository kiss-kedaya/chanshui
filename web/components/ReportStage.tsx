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
        <div className="hero-glow" aria-hidden="true" />
        <div className="hero-grid" aria-hidden="true" />
        <div className="hero-scan" aria-hidden="true" />
        <div className="stage-status">
          <i className="stage-spin" aria-hidden="true" />
          <span>{text}</span>
        </div>
        <div className="stage-ghost lg" aria-hidden="true" />
        <div className="stage-ghost sm" aria-hidden="true" />
      </div>
      <div className="stage-grid" aria-hidden="true">
        {["a", "b", "c", "d"].map((key) => (
          <article className="stage-card" key={key}>
            <i />
            <b />
            <i />
          </article>
        ))}
      </div>
    </section>
  );
}
