"use client";

import { DetectVerdict } from "@/lib/api";

const FAMILIES: Record<string, string> = {
  gpt: "#10a37f",
  openai: "#10a37f",
  claude: "#d97757",
  anthropic: "#d97757",
  google: "#4285f4",
  deepseek: "#4d6bfe",
  xai: "#111827",
  zhipu: "#4b6bfb",
};

function familyColor(f: string): string {
  return FAMILIES[f?.toLowerCase()] || "#6b7280";
}

export default function ResultCards({ verdict }: { verdict: DetectVerdict }) {
  const inj = verdict.injection;
  const fp = verdict.fingerprint;
  const tk = verdict.thinking;
  const iq = verdict.iq;

  return (
    <div className="results">
      {inj && (
        <div className="result-card">
          <div className="rc-head">
            <span className="rc-title">注入 token</span>
            {inj.error && <span className="hist-badge badge-bad">探测失败</span>}
          </div>
          {!inj.error ? (
            <>
              <div className="rc-big">≈ {inj.inj} tok</div>
              <div className="rc-sub">
                每次请求额外注入的系统提示（斜率 {inj.slope.toFixed(3)} tok/字符）
                {inj.cache_header && ` · 缓存头: ${inj.cache_header}`}
              </div>
              <details className="fold">
                <summary>采样点</summary>
                <pre>
                  {inj.points.map((p) => `${p.n} → ${p.total}`).join("\n")}
                </pre>
              </details>
            </>
          ) : (
            <div className="rc-sub">{inj.error}</div>
          )}
        </div>
      )}

      {fp && (
        <div className="result-card">
          <div className="rc-head">
            <span className="rc-title">模型归因</span>
            {fp.error && <span className="hist-badge badge-bad">探测失败</span>}
          </div>
          {!fp.error ? (
            <>
              <div style={{ marginBottom: 8 }}>
                <span
                  className="family-badge"
                  style={{ background: familyColor(fp.family) }}
                >
                  {fp.family_name || fp.family}
                </span>
                <span style={{ marginLeft: 10, fontSize: 13, color: "var(--muted)" }}>
                  家族置信度 {(fp.family_prob * 100).toFixed(1)}%
                </span>
              </div>
              {fp.results.slice(0, 3).map((r) => (
                <div key={r.model} style={{ marginTop: 8 }}>
                  <div style={{ display: "flex", justifyContent: "space-between", fontSize: 12.5 }}>
                    <span style={{ fontFamily: "ui-monospace, Consolas, monospace" }}>{r.model}</span>
                    <span>{(r.probability * 100).toFixed(1)}%</span>
                  </div>
                  <div className="prob-bar">
                    <div
                      style={{
                        width: `${Math.max(2, r.probability * 100)}%`,
                        background: familyColor(fp.family),
                      }}
                    />
                  </div>
                </div>
              ))}
              <div className="rc-sub" style={{ marginTop: 8 }}>
                有效输出 {fp.used_outputs} 份
                {fp.behavioral_top3 && fp.behavioral_top3.length > 0 && (
                  <>
                    {" · "}行为指纹佐证:{" "}
                    {fp.behavioral_top3.map((b) => b.model_id).join(" / ")}
                  </>
                )}
              </div>
            </>
          ) : (
            <div className="rc-sub">{fp.error}</div>
          )}
        </div>
      )}

      {tk && (
        <div className="result-card">
          <div className="rc-head">
            <span className="rc-title">思考文本回传</span>
            <span className={`hist-badge ${tk.pass ? "badge-ok" : "badge-bad"}`}>
              {tk.pass ? "通过 ✓" : "未通过 ✗"}
            </span>
          </div>
          <div className="rc-sub">{tk.reason}</div>
          <div className="rc-sub">
            思考文本 {tk.think_len} 字 · 签名 {tk.sig_len} 字
          </div>
        </div>
      )}

      {iq && (
        <div className="result-card">
          <div className="rc-head">
            <span className="rc-title">智商评分</span>
            {iq.error && <span className="hist-badge badge-bad">测试失败</span>}
          </div>
          {!iq.error ? (
            <>
              <div style={{ display: "flex", gap: 18, alignItems: "center" }}>
                <div className="score-ring">{iq.score}</div>
                <div className="rc-sub">
                  满分 {iq.max} · {iq.per_question.filter((q) => q.earned === q.max).length}/
                  {iq.per_question.length} 题全对
                </div>
              </div>
              <details className="fold">
                <summary>每题明细</summary>
                <pre>
                  {iq.per_question
                    .map(
                      (q) =>
                        `${q.earned}/${q.max}  ${q.title}${q.half ? "  [半分]" : ""}${q.tags.length ? `  标记: ${q.tags.join(",")}` : ""}`
                    )
                    .join("\n")}
                </pre>
              </details>
            </>
          ) : (
            <div className="rc-sub">{iq.error}</div>
          )}
        </div>
      )}
    </div>
  );
}
