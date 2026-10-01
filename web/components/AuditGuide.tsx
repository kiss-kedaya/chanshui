import Icon from "@/components/Icon";
export default function AuditGuide() {
  return (
    <section
      className="audit-guide"
      id="audit-guide"
      aria-labelledby="guide-title"
    >
      <div className="guide-heading">
        <div>
          <span className="eyebrow">过程透明，结论有据</span>
          <h2 id="guide-title">一次检测，沿着证据看清楚。</h2>
        </div>
        <span className="guide-note">检测算法全部运行在服务端</span>
      </div>
      <ol className="audit-steps">
        <li>
          <span className="guide-number">01</span>
          <h3>连接接口</h3>
          <p>
            填写地址、密钥和模型。
            <br />
            系统自动适配支持的检测项。
          </p>
        </li>
        <li>
          <span className="guide-number">02</span>
          <h3>并行核验</h3>
          <p>
            采样用量、对比指纹、验证缓存。
            <br />
            每个探针独立显示状态。
          </p>
        </li>
        <li>
          <span className="guide-number">03</span>
          <h3>读懂报告</h3>
          <p>
            展开每项证据，按 Job ID 分享。
            <br />
            失败项可单独重跑。
          </p>
        </li>
      </ol>
      <details className="guide-faq">
        <summary>
          <span>多出来的 token，一定是隐藏提示词吗？</span>
          <Icon name="chevron" />
        </summary>
        <p>
          不一定。协议包装和默认模板也会增加输入用量。这里用三次串行请求合并估计注入偏移与缓存链，不把差额直接当成隐藏提示词的精确数量。模型指纹也仅是现有候选库内的比较。
        </p>
      </details>
    </section>
  );
}
