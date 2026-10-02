import Icon from "@/components/Icon";

const points = [
  ["01", "模型指纹"],
  ["02", "注入大小"],
  ["03", "工具与联网"],
  ["04", "推理表现"],
];

export default function AuditHero() {
  return (
    <section className="audit-hero" aria-labelledby="audit-title">
      <div className="hero-glow" aria-hidden="true" />
      <div className="hero-grid" aria-hidden="true" />
      <div className="hero-content">
        <div className="hero-kicker">
          <Icon name="scan" />
          <span>中转站降智审计</span>
          <i />
        </div>
        <h1 id="audit-title">
          你的模型，
          <span>掺水了吗？</span>
        </h1>
        <p>填一次网关，看声称模型和实际回答是不是同一只。</p>
        <ol className="hero-points">
          {points.map(([index, label]) => (
            <li key={index}>
              <b>{index}</b>
              {label}
            </li>
          ))}
        </ol>
      </div>
    </section>
  );
}
