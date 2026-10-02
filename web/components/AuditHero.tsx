"use client";
import Icon from "@/components/Icon";
import { useI18n } from "@/lib/i18n";

export default function AuditHero() {
  const { t: tr } = useI18n();
  const points = ["模型指纹", "注入大小", "工具与联网", "推理表现"];
  return (
    <section className="audit-hero" aria-labelledby="audit-title">
      <div className="hero-glow" aria-hidden="true" />
      <div className="hero-grid" aria-hidden="true" />
      <div className="hero-content">
        <div className="hero-kicker">
          <Icon name="scan" />
          <span>{tr("中转站降智审计")}</span>
          <i />
        </div>
        <h1 id="audit-title">
          {tr("你的模型，")}
          <span>{tr("掺水了吗？")}</span>
        </h1>
        <p>{tr("填一次网关，看声称模型和实际回答是不是同一只。")}</p>
        <ul className="hero-points">
          {points.map((label) => (
            <li key={label}>{tr(label)}</li>
          ))}
        </ul>
      </div>
    </section>
  );
}
