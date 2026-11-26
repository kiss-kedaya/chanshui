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
    return (
      <section className="result-card score-hero is-pending" aria-busy="true">
        <h2 className="sh-kicker">{tr("检测结论")}</h2>
        {progress && progress.total > 0 ? (
          <>
            <p className="sh-headline">
              {tr("正在收集证据")}
              <span className="sh-count">
                {progress.done} / {progress.total}
              </span>
            </p>
            <div
              className="progress-track"
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
  return (
    <section className="result-card score-hero" data-level={result.level} aria-labelledby="score-hero-title">
      <div className="sh-top">
        <h2 className="sh-kicker" id="score-hero-title">
          {tr("检测结论")}
          <Hint
            label={tr("总分说明")}
            text={tr("总分按各板块权重加权，没有适用的板块不占分，其余按比例补足到 100。指纹只在数字基线 99.9% 以上确认是别的模型时才扣分。知识截止日期只展示，不计分。")}
          />
        </h2>
      </div>
      <div className="sh-body">
        <div className="sh-score">
          <strong className={"sh-number tone-" + result.tone}>
            <AnimatedNumber value={total.score} />
          </strong>
          <span className="sh-max">/ {total.max || 100}</span>
        </div>
        <div className="sh-summary">
          <p className="sh-headline">{tr(HEADLINE[result.level], { n: Math.max(1, real) })}</p>
          <p className="sh-lede">{tr(LEDE[result.level])}</p>
          {shown.length > 0 && (
            <ul className="sh-findings">
              {shown.map((item, index) => (
                <li key={index} data-tone={item.tone}>
                  {tr(item.text, item.vars)}
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
                <i aria-hidden="true" />
              </li>
            );
          })}
        </ul>
      )}
      {claude && (
        <div className="sh-channel">
          <span className="sh-channel-label">{tr("Claude 渠道")}</span>
          {chips.map((chip) => (
            <span className="sh-chip" key={chip.key} data-state={chip.state}>
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
