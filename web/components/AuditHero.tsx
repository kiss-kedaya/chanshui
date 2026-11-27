"use client";
import Icon from "@/components/Icon";
import { useI18n } from "@/lib/i18n";

export default function AuditHero() {
  const { t: tr } = useI18n();
  const points = [
    ["01", "模型指纹"],
    ["02", "隐藏注入"],
    ["03", "签名验真"],
    ["04", "推理表现"],
    ["05", "识图能力"],
    ["06", "反作弊拦截"],
  ] as const;

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
          {points.map(([num, label]) => (
            <li key={label}>
              <small>{num}</small>
              <span>{tr(label)}</span>
            </li>
          ))}
        </ul>
      </div>
      <div className="hero-foot-meta">
        <Icon name="shield" width="14" height="14" />
        <span>{tr("全流程公开探针 · 自动识别回放与伪造签名")}</span>
      </div>
    </section>
  );
}
