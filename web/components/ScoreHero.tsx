"use client";
import type { CSSProperties } from "react";
import AnimatedNumber from "@/components/AnimatedNumber";
import Hint from "@/components/Hint";
import ResultSkeleton from "@/components/ResultSkeleton";
import { DetectVerdict } from "@/lib/api";
import { channelChips, FingerprintState, reportVerdict } from "@/lib/report-view";
import { useI18n } from "@/lib/i18n";

const HEADLINE = {
  ok: "未发现掺水迹象",
  caution: "存在 {n} 处疑点",
  fake: "疑似掺水",
  unknown: "证据不足",
} as const;
const LEDE = {
  ok: "本次范围内指纹、注入和工具都没有异常。这不是型号证明。",
  caution: "总分只是加权平均，下面这些发现值得你再看一眼。",
  fake: "指纹在 99.9% 以上置信度下指向另一个模型，总分已扣掉指纹分。",
  unknown: "多项请求失败，分数只反映拿到的部分，建议重新检测后再看结论。",
} as const;
const SHOWN = 4;

function translateFindingVars(
  vars: Record<string, string | number> | undefined,
  tr: (source: string, vars?: Record<string, string | number>) => string,
): Record<string, string | number> | undefined {
  if (!vars) return vars;
  const out: Record<string, string | number> = {};
  for (const [key, value] of Object.entries(vars)) {
    out[key] = typeof value === "string" && key !== "model" ? tr(value) : value;
  }
  return out;
}

// Verdict first, number second, evidence third: the score is a weighted
// average, so the headline says how far to trust it and the rows show where
// the points went.
export default function ScoreHero({
  verdict,
  fp,
  claude,
  loading,
  progress,
}: {
  verdict: DetectVerdict;
  fp: FingerprintState;
  claude: boolean;
  loading: boolean;
  progress?: { done: number; total: number };
}) {
  const { t: tr } = useI18n();
  const total = verdict.total;
  const result = reportVerdict(verdict, fp);
  if (!total || !result) {
    if (!loading) return null;
    // Keep the slot (no layout jump when the verdict lands) and show how far
    // the audit is, so progress is visible without scrolling to the timeline.
    const pendingRatio =
      progress && progress.total > 0
        ? Math.min(1, Math.max(0, progress.done / progress.total))
        : 0;
    const pendingR = 14;
    const pendingC = 2 * Math.PI * pendingR;
    return (
      <section className="result-card score-hero is-pending" aria-busy="true">
        <h2 className="sh-kicker">{tr("检测结论")}</h2>
        {progress && progress.total > 0 ? (
          <>
            <div className="sh-pending-head">
              <svg className="sh-pending-ring" viewBox="0 0 36 36" aria-hidden="true">
                <circle
                  className="sh-pending-track"
                  cx="18"
                  cy="18"
                  r={pendingR}
                  fill="none"
                  strokeWidth="3"
                />
                <circle
                  className="sh-pending-fill"
                  cx="18"
                  cy="18"
                  r={pendingR}
                  fill="none"
                  strokeWidth="3"
                  strokeLinecap="round"
                  strokeDasharray={pendingC}
                  strokeDashoffset={pendingC * (1 - pendingRatio)}
                  transform="rotate(-90 18 18)"
                />
              </svg>
              <p className="sh-headline">
                {tr("正在收集证据")}
                <span className="sh-count">
                  {progress.done} / {progress.total}
                </span>
              </p>
            </div>
            <div
              className="progress-track is-pill"
              role="progressbar"
              aria-label={tr("已完成探针")}
              aria-valuemin={0}
              aria-valuemax={progress.total}
              aria-valuenow={progress.done}
            >
              <div style={{ transform: "scaleX(" + Math.min(1, progress.done / progress.total) + ")" }} />
            </div>
            <p className="sh-lede">{tr("检测完成后在这里给出结论")}</p>
          </>
        ) : (
          <ResultSkeleton label={tr("检测完成后在这里给出结论")} />
        )}
      </section>
    );
  }
  const real = result.findings.filter((item) => item.tone !== "note").length;
  const shown = result.findings.slice(0, SHOWN);
  const more = result.findings.length - shown.length;
  const parts = (total.parts ?? []).filter((item) => item.applicable);
  const chips = claude ? channelChips(verdict) : [];
  const cheated = Boolean(verdict.anticheat?.detected || verdict.thinking?.sig_forged || verdict.thinking?.replay_confessed);
  const headlineText = cheated
    ? tr("检测到针对性作假")
    : tr(HEADLINE[result.level], { n: Math.max(1, real) });
  const ledeText = cheated
    ? tr("探针原始证据命中回放、伪造签名或查表特征，对应作假板块已按 0 分计入。")
    : tr(LEDE[result.level]);
  const scoreMax = total.max || 100;
  const scoreRatio = Math.min(1, Math.max(0, total.score / scoreMax));
  const gaugeR = 58;
  const gaugeC = 2 * Math.PI * gaugeR;
  const gaugeArc = 0.75 * gaugeC;
  return (
    <section className="result-card score-hero" data-level={result.level} aria-labelledby="score-hero-title">
      <div className="sh-top">
        <h2 className="sh-kicker" id="score-hero-title">
          {tr("检测结论")}
          <Hint
            label="总分说明"
            text="总分按各板块权重加权，没有适用的板块不占分，其余按比例补足到 100。指纹只在数字基线 99.9% 以上确认是别的模型时才扣分。知识截止日期只展示，不计分。"
          />
        </h2>
      </div>
      <div className="sh-body">
        <div className="sh-gauge" data-gauge-tone={result.tone}>
          <svg className="sh-gauge-svg" viewBox="0 0 144 144" aria-hidden="true">
            <circle
              className="sh-arc-track"
              cx="72"
              cy="72"
              r={gaugeR}
              fill="none"
              strokeWidth="7"
              strokeLinecap="round"
              strokeDasharray={`${gaugeArc} ${gaugeC}`}
              transform="rotate(135 72 72)"
            />
            <circle
              className="sh-arc-ticks"
              cx="72"
              cy="72"
              r={49}
              fill="none"
              strokeWidth="1"
              strokeDasharray="1.5 6"
            />
            <circle
              className="sh-arc-fill"
              cx="72"
              cy="72"
              r={gaugeR}
              fill="none"
              strokeWidth="7"
              strokeLinecap="round"
              strokeDasharray={`${gaugeArc} ${gaugeC}`}
              strokeDashoffset={gaugeArc * (1 - scoreRatio)}
              transform="rotate(135 72 72)"
              style={{ opacity: total.score <= 0 ? 0 : 1 }}
            />
          </svg>
          <div className="sh-score">
            <strong className={"sh-number tone-" + result.tone}>
              <AnimatedNumber value={total.score} />
            </strong>
            <span className="sh-max">/ {scoreMax}</span>
          </div>
        </div>
        <div className="sh-summary">
          <p className="sh-headline">{headlineText}</p>
          <p className="sh-lede">{ledeText}</p>
          {shown.length > 0 && (
            <ul className="sh-findings">
              {shown.map((item, index) => (
                <li key={index} data-tone={item.tone}>
                  {tr(item.text, translateFindingVars(item.vars, tr))}
                </li>
              ))}
              {more > 0 && (
                <li data-tone="note">{tr("另有 {n} 项，见下方各卡片", { n: more })}</li>
              )}
            </ul>
          )}
        </div>
      </div>
      {parts.length > 0 && (
        <ul className="sh-parts">
          {parts.map((item, index) => {
            const ratio = item.coefficient > 0 ? Math.min(1, Math.max(0, item.contribution / item.coefficient)) : 0;
            const state = !item.known ? "none" : ratio >= 1 ? "full" : ratio >= 0.5 ? "half" : "low";
            return (
              <li
                className="sh-part"
                key={item.key}
                data-state={state}
                style={{ "--r": ratio, "--n": index } as CSSProperties}
              >
                <span>{tr(item.label)}</span>
                <b>{item.known ? item.contribution + "/" + item.coefficient : "—"}</b>
                <i className="is-pill" aria-hidden="true" />
              </li>
            );
          })}
        </ul>
      )}
      {claude && (
        <div className="sh-channel">
          <span className="sh-channel-label">{tr("Claude 渠道")}</span>
          {chips.map((chip) => (
            <span className="sh-chip is-pill" key={chip.key} data-state={chip.state}>
              <i>{tr(chip.label)}</i>
              <b>{tr(chip.value)}</b>
            </span>
          ))}
          {!verdict.claude_deep && (
            <p className="sh-hint">{tr("勾选「Claude 深度体检」可得到现签、0 注入、ccmax 结论。")}</p>
          )}
        </div>
      )}
    </section>
  );
}
