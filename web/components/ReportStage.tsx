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
        <div className="stage-emblem" aria-hidden="true"><span>?</span><i /><i /></div>
        <div className="stage-status">
          <i className="stage-spin" aria-hidden="true" />
          <span>{text}</span>
        </div>
        <h2 className="stage-title">{t("全站检测报告")}</h2>
        <div className="stage-track" aria-hidden="true"><i /></div>
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
