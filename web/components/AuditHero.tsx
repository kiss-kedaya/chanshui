import Icon from "@/components/Icon";
export default function AuditHero() {
  return (
    <section className="audit-hero" aria-labelledby="audit-title">
      <div className="hero-glow" aria-hidden="true" />
      <div className="hero-grid" aria-hidden="true" />
      <div className="hero-content">
        <div className="hero-kicker">
          <Icon name="scan" />
          <span>模型检测站 · 中转站降智审计</span>
          <i />
        </div>
        <h1 id="audit-title">
          你的模型，<span>掺水了吗？</span>
        </h1>
        <p>用一次检测，看清模型与接口的真实表现。</p>
      </div>
      <div className="hero-bottom">
        <span>
          注入大小<span className="dot-separator">·</span>模型指纹
          <span className="dot-separator">·</span>缓存一致性
          <span className="dot-separator">·</span>推理表现
        </span>
        <a href="#connection">
          开始检测
          <Icon name="arrow" />
        </a>
      </div>
    </section>
  );
}
