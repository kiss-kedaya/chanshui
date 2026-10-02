"use client";
import Icon from "@/components/Icon";
import { useI18n } from "@/lib/i18n";
export default function AuditGuide() {
  const { t: tr } = useI18n();
  return (
    <section
      className="audit-guide"
      id="audit-guide"
      aria-labelledby="guide-title"
    >
      <div className="guide-heading">
        <div>
          <span className="eyebrow">{tr("过程透明，结论有据")}</span>
          <h2 id="guide-title">{tr("一次检测，沿着证据看清楚。")}</h2>
        </div>
        <span className="guide-note">{tr("检测算法全部运行在服务端")}</span>
      </div>
      <ol className="audit-steps">
        <li>
          <span className="guide-number">01</span>
          <h3>{tr("连接接口")}</h3>
          <p>
            {tr("填写地址、密钥和模型。")}
            <br />
            {tr("系统自动适配支持的检测项。")}
          </p>
        </li>
        <li>
          <span className="guide-number">02</span>
          <h3>{tr("并行核验")}</h3>
          <p>
            {tr("采样用量、对比指纹、验证缓存。")}
            <br />
            {tr("每个探针独立显示状态。")}
          </p>
        </li>
        <li>
          <span className="guide-number">03</span>
          <h3>{tr("读懂报告")}</h3>
          <p>
            {tr("展开每项证据，按 Job ID 分享。")}
            <br />
            {tr("失败项可单独重跑。")}
          </p>
        </li>
      </ol>
      <details className="guide-faq">
        <summary>
          <span>{tr("多出来的 token，一定是隐藏提示词吗？")}</span>
          <Icon name="chevron" />
        </summary>
        <p>
          {tr("不一定。协议包装和默认模板也会增加输入用量。这里用两个独立小请求估计注入偏移，缓存读写另用三轮串行对话验证；不把差额直接当成隐藏提示词的精确数量。模型指纹也仅是现有候选库内的比较。")}
        </p>
      </details>
    </section>
  );
}
