"use client";
import { ReactNode } from "react";
import AnimatedNumber from "@/components/AnimatedNumber";
import ResultSkeleton from "@/components/ResultSkeleton";
import ConversationPanel, { trConvReason } from "@/components/ConversationPanel";
import ErrorFold from "@/components/ErrorFold";
import Hint from "@/components/Hint";
import ScoreHero from "@/components/ScoreHero";
import { fingerprintState, presentedTotal, probabilityVisible } from "@/lib/report-view";
import { DetectVerdict, Probe } from "@/lib/api";
import { translateError } from "@/lib/error-i18n";
import { useI18n } from "@/lib/i18n";

type Translator = (source: string, vars?: Record<string, string | number>) => string;

function trTag(tag: string, tr: Translator): string {
  const m = tag.match(/^(库存|实例|种子)[:：](.+)$/);
  if (m) return tr(m[1]) + ":" + m[2];
  return tr(tag);
}

function trAntiCheatSummary(summary: string, tr: Translator): string {
  const m1 = summary.match(/^检测到 (\d+) 项针对性作假证据（(.+)），对应板块已按 0 分计入。$/);
  if (m1) {
    const titles = m1[2]
      .split("、")
      .map((item) => tr(item.trim()))
      .join(", ");
    return tr("检测到 {n} 项针对性作假证据（{titles}），对应板块已按 0 分计入。", { n: m1[1], titles });
  }
  const m2 = summary.match(/^目标域名命中已确认针对本站作假规则（(.+)），未发起上游请求，直接按 0 分处置。$/);
  if (m2) return tr("目标域名命中已确认针对本站作假规则（{host}），未发起上游请求，直接按 0 分处置。", { host: m2[1] });
  return tr(summary);
}

function trForceCacheConclusion(conclusion: string, tr: Translator): string {
  if (!conclusion) return "";
  const http = conclusion.match(/^(.+) 返回 HTTP (\d+)，样本不定$/);
  if (http) return tr("{label} 返回 HTTP {status}，样本不定", { label: tr(http[1]), status: http[2] });
  const mixed = conclusion.match(/^样本介于两者之间：input=(\d+) write5m=(\d+) write1h=(\d+) read=(\d+)$/);
  if (mixed) {
    return tr("样本介于两者之间：input={input} write5m={write5m} write1h={write1h} read={read}", {
      input: mixed[1],
      write5m: mixed[2],
      write1h: mixed[3],
      read: mixed[4],
    });
  }
  const segs = conclusion.split("；");
  if (segs.length > 1) {
    const mapped = segs.map((seg) => trForceCacheConclusion(seg, tr));
    return mapped.every((m, idx) => m === segs[idx]) ? conclusion : mapped.join("; ");
  }
  return tr(conclusion);
}

function trNote(note: string | undefined, tr: Translator): string | undefined {
  if (!note) return note;
  const thresh = note.match(/^Claude 按缓存首写阈值反推：首写 (\d+) token，名义门槛 (\d+)，隐藏 (\d+)。$/);
  if (thresh) {
    return tr("Claude 按缓存首写阈值反推：首写 {threshold} token，名义门槛 {nominal}，隐藏 {hidden}。", {
      threshold: thresh[1],
      nominal: thresh[2],
      hidden: thresh[3],
    });
  }
  const zeroSamples = note.match(/^(\d+) 次里没有拿到有效数字样本，这项记 0 分。$/);
  if (zeroSamples) return tr("{n} 次里没有拿到有效数字样本，这项记 0 分。", { n: zeroSamples[1] });
  const iqNote = note.match(
    /^当前卷面 (\d+)；半分仅在卷面展示，不进入百分制通过分。失败请求标记未完成，但已答题目照常计分，不伪装为模型答错。$/,
  );
  if (iqNote) {
    return tr(
      "当前卷面 {n}；半分仅在卷面展示，不进入百分制通过分。失败请求标记未完成，但已答题目照常计分，不伪装为模型答错。",
      { n: iqNote[1] },
    );
  }
  if (note.startsWith("行为探针失败：")) {
    return tr("行为探针失败：") + translateError(note.slice("行为探针失败：".length), tr);
  }
  if (note.startsWith("目标域名命中已确认针对本站作假规则")) {
    return trAntiCheatSummary(note, tr);
  }
  const parts = note
    .split(/(?<=。)\s+/)
    .map((p) => p.trim())
    .filter(Boolean);
  if (parts.length > 1) {
    const mapped = parts.map((p) => trNote(p, tr) || p);
    return mapped.every((m, idx) => m === parts[idx]) ? note : mapped.join(" ");
  }
  return tr(note);
}

function trKnowledgeGap(text: string, tr: Translator): string {
  const m = text.match(/^有 (\d+) 题结果对不上，未用它们移动边界$/);
  if (m) return tr("有 {n} 题结果对不上，未用它们移动边界", { n: m[1] });
  return tr(text);
}

function trKnowledgeSuspect(text: string, tr: Translator): string {
  const m = text.match(/^答对了 (\d+) 天前的事件（(.+)），疑似联网$/);
  if (m) return tr("答对了 {days} 天前的事件（{event}），疑似联网", { days: m[1], event: m[2] });
  return tr(text);
}

function trKnowledgeReason(reason: string, tr: Translator): string {
  if (!reason) return "";
  const segs = reason
    .split(/(?<=。)\s*/)
    .map((s) => s.trim())
    .filter(Boolean);
  if (segs.length <= 1) {
    const clean = reason.replace(/。$/, "");
    if (/^有 (\d+) 题结果对不上，未用它们移动边界$/.test(clean)) return trKnowledgeGap(clean, tr);
    if (/^答对了 (\d+) 天前的事件（(.+)），疑似联网$/.test(clean)) return trKnowledgeSuspect(clean, tr);
    return tr(reason);
  }
  const endSentence = (s: string) =>
    /[.。!?]$/.test(s) ? s : /[\u4e00-\u9fff\u3040-\u30ff]/.test(s) ? s + "。" : s + ".";
  const mapped = segs.map((seg) => {
    const clean = seg.replace(/。$/, "");
    if (/^有 (\d+) 题结果对不上，未用它们移动边界$/.test(clean)) return endSentence(trKnowledgeGap(clean, tr));
    if (/^答对了 (\d+) 天前的事件（(.+)），疑似联网$/.test(clean)) return endSentence(trKnowledgeSuspect(clean, tr));
    return tr(seg);
  });
  return mapped.every((m, idx) => m === segs[idx]) ? reason : mapped.join(" ");
}

function trThinkingReason(reason: string, tr: Translator): string {
  if (!reason) return "";
  if (reason.startsWith("目标域名命中已确认针对本站作假规则")) {
    return trAntiCheatSummary(reason, tr);
  }
  const exact = tr(reason);
  if (exact !== reason) return exact;
  const body = reason.replace(/。$/, "");
  const segs = body
    .split("；")
    .map((s) => s.trim())
    .filter(Boolean);
  if (segs.length === 0) return reason;
  const mapped = segs.map((seg) => {
    const forged = seg.match(/^检测到伪造思维链签名，思维链项记 0 分(?:（([\s\S]+)）)?$/);
    if (forged) {
      const head = tr("检测到伪造思维链签名，思维链项记 0 分");
      return forged[1] ? `${head} (${tr(forged[1])})` : head;
    }
    const reused = seg.match(/^复用 (.+) 的签名$/);
    if (reused) return tr("复用 {source} 的签名", { source: reused[1] });
    const mismatch = seg.match(/^签名型号 (.+) 与请求不符（-2）$/);
    if (mismatch) return tr("签名型号 {model} 与请求不符（-2）", { model: mismatch[1] });
    const summary = seg.match(/^摘要 (\d+)\/(\d+)$/);
    if (summary) return tr("摘要 {a}/{b}", { a: summary[1], b: summary[2] });
    const sig = seg.match(/^签名 (\d+)\/(\d+)$/);
    if (sig) return tr("签名 {a}/{b}", { a: sig[1], b: sig[2] });
    const replay = seg.match(/^原始续轮 (\d+)\/(\d+)$/);
    if (replay) return tr("原始续轮 {a}/{b}", { a: replay[1], b: replay[2] });
    const tamper = seg.match(/^篡改拒绝 (\d+)\/(\d+)$/);
    if (tamper) return tr("篡改拒绝 {a}/{b}", { a: tamper[1], b: tamper[2] });
    const selfReport = seg.match(/^自述：([\s\S]+)$/);
    if (selfReport) return tr("自述：{text}", { text: selfReport[1] });
    const struct = seg.match(/^签名结构 (.+?)(?:（(\d+) 字段(?:，([^）]+))?）)?$/);
    if (struct) {
      const cls = tr(sigShort(struct[1]));
      if (struct[2] && struct[3]) {
        return tr("签名结构 {cls}（{n} 字段，{infra}）", { cls, n: struct[2], infra: struct[3] });
      }
      if (struct[2]) {
        return tr("签名结构 {cls}（{n} 字段）", { cls, n: struct[2] });
      }
      return tr("签名结构 {cls}", { cls });
    }
    const strict = seg.match(/^协议严格性 (\d+)\/(\d+)(?:（未拒绝：(.+)）)?$/);
    if (strict) {
      if (strict[3]) {
        const items = strict[3]
          .replace(/ 等 (\d+) 项$/, (_, n) => tr(" 等 {n} 项", { n }))
          .replace(/、/g, ", ");
        return tr("协议严格性 {a}/{b}（未拒绝：{items}）", { a: strict[1], b: strict[2], items });
      }
      return tr("协议严格性 {a}/{b}", { a: strict[1], b: strict[2] });
    }
    return tr(seg);
  });
  return mapped.every((m, idx) => m === segs[idx]) ? reason : mapped.join("; ");
}

function trCapabilityReason(reason: string, tr: Translator): string {
  if (!reason) return "";
  if (reason.startsWith("目标域名命中已确认针对本站作假规则")) {
    return trAntiCheatSummary(reason, tr);
  }
  const auth = reason.match(/^Search 接口鉴权失败（HTTP (\d+)）。$/);
  if (auth) return tr("Search 接口鉴权失败（HTTP {status}）。", { status: auth[1] });
  const unavail = reason.match(/^Search 接口暂时不可用（HTTP (\d+)）。$/);
  if (unavail) return tr("Search 接口暂时不可用（HTTP {status}）。", { status: unavail[1] });
  const http = reason.match(/^Search 接口返回 HTTP (\d+)。$/);
  if (http) return tr("Search 接口返回 HTTP {status}。", { status: http[1] });
  const ok = reason.match(/^DeepSeek Search API 返回 (\d+) 条有效结果，含标题、摘要和来源链接。$/);
  if (ok) return tr("DeepSeek Search API 返回 {n} 条有效结果，含标题、摘要和来源链接。", { n: ok[1] });
  return tr(reason);
}
// One heading per kind of evidence, so a long report reads as chapters
// instead of a flat pile of cards.
function Group({ id, title, children }: { id: string; title: string; children: ReactNode }) {
  const { t: tr } = useI18n();
  return (
    <section className="result-group" aria-labelledby={id}>
      <h2 className="group-title" id={id}>
        {tr(title)}
      </h2>
      <div className="group-cards">{children}</div>
    </section>
  );
}
function ScoreShare({ verdict, part }: { verdict: DetectVerdict; part: string }) {
  const { t: tr } = useI18n();
  const row = verdict.total?.parts?.find((item) => item.key === part && item.applicable);
  if (!row) return null;
  return (
    <b className={"score-chip" + (row.contribution === 0 ? " is-zero" : "")} title={tr("计入总分 / 系数")}>
      {row.contribution}/{row.coefficient}
    </b>
  );
}
function CardTitle({ title, label, text }: { title: string; label?: string; text?: string }) {
  const { t: tr } = useI18n();
  return (
    <div className="rc-title">
      <h3>{tr(title)}</h3>
      {text ? <Hint label={label || title} text={text} /> : null}
    </div>
  );
}
function Details({ label, value }: { label: string; value: unknown }) {
  const { t: tr } = useI18n();
  return (
    <details className="fold">
      <summary>{tr(label)}</summary>
      <pre>
        {typeof value === "string"
          ? value || tr("未回传")
          : JSON.stringify(value, null, 2)}
      </pre>
    </details>
  );
}


function sigShort(cls: string): string {
  if (cls.startsWith("伪造")) return "伪造签名";
  if (cls.startsWith("真签")) return "真签";
  if (cls.startsWith("历史")) return "历史缓存";
  if (cls.startsWith("标准信封")) return "标准信封";
  if (cls.startsWith("空信封")) return "空信封";
  if (cls.startsWith("半成品")) return "半成品";
  if (cls.startsWith("未识别")) return "未识别";
  if (cls.startsWith("无签")) return "无签";
  return cls;
}

const ANTICHEAT_SECTION_LABEL: Record<string, string> = {
  domain: "站点规则",
  fingerprint: "模型指纹",
  thinking: "思维链",
  iq: "推理表现",
  vision: "识图能力",
  tools: "工具调用",
};

function sigBytes(cls: string): string {
  const m = cls.match(/密文\s*(\d+)\s*字节/);
  return m ? m[1] + " B" : "";
}

// Only a 200 on the tampered signature proves it was accepted. A 429, 5xx,
// timeout or a 400 that is not about the signature says nothing either way.
function tamperView(t: { tamper_rejected?: boolean; tamper_status?: number }, tr: (source: string) => string) {
  if (t.tamper_rejected) return { cls: "ok", text: tr("被拒") };
  if (t.tamper_status === 200) return { cls: "no", text: tr("被接受") };
  return { cls: "skip", text: t.tamper_status ? "HTTP " + t.tamper_status : tr("不定") };
}

function knowledgeMark(row: { kind?: string; ok: boolean; unsure: boolean; error?: string }, tr: (source: string) => string) {
  if (row.kind === "know" || (row.ok && row.kind !== "wrong" && row.kind !== "error" && row.kind !== "invalid")) {
    return { text: tr("知道"), cls: "good" };
  }
  if (row.kind === "wrong" || (!row.ok && !row.unsure && !row.error && row.kind !== "unknown")) {
    return { text: tr("答错"), cls: "warn" };
  }
  if (row.kind === "invalid") return { text: tr("无效"), cls: "warn" };
  if (row.kind === "error" || row.error) return { text: tr("失败"), cls: "warn" };
  return { text: tr("不知道"), cls: "muted" };
}

function knowledgeLabel(k: { label?: string; cutoff?: string; correct?: number; total?: number }, tr: (source: string, vars?: Record<string, string | number>) => string) {
  if (k.label) {
    const range = k.label.match(/^知识≈(\S+) 至 (\S+)（(\d+) 问）$/);
    if (range) return tr("知识≈{lo} 至 {hi}（{n} 问）", { lo: range[1], hi: range[2], n: range[3] });
    const ge = k.label.match(/^知识≥(\S+)$/);
    if (ge) return tr("知识≥{lo}", { lo: ge[1] });
    const lt = k.label.match(/^知识＜(\S+)$/);
    if (lt) return tr("知识＜{hi}", { hi: lt[1] });
    return tr(k.label);
  }
  if (k.cutoff && /^\d{4}-\d{2}$/.test(k.cutoff)) return tr("旧报告自报 {cutoff}", { cutoff: k.cutoff });
  if (k.cutoff && k.total) return tr("知识≈{cutoff}（{a}/{b}）", { cutoff: k.cutoff, a: k.correct ?? 0, b: k.total });
  if (k.cutoff) return tr("知识≈{cutoff}", { cutoff: k.cutoff });
  return tr("知识≈未知");
}
export default function ResultCards({
  verdict: v,
  model,
  loading = false,
  showCache = false,
  probes,
  progress,
}: {
  verdict: DetectVerdict;
  model?: string;
  loading?: boolean;
  showCache?: boolean;
  probes?: Probe[];
  progress?: { done: number; total: number };
}) {
  const { t: tr } = useI18n();
  const a = v.injection,
    f = v.fingerprint,
    t = v.thinking,
    q = v.iq;
  // v0.1.27/28 落库的报告里 signals/asks 可能是 null（后端 nil 切片），
  // 旧报告不会被重写，所以前端必须兜底。
  const knowledge = v.knowledge;
  const vision = v.vision;
  const integrity = v.integrity;
  const claude = (model || "").toLowerCase().startsWith("claude-");
  const cache = v.cache;
  const conversationCache = Boolean(v.conversation && cache?.samples?.length);
  const cacheSamples = cache?.samples ?? [];
  const cacheTransitions = cache?.transitions ?? [];
  const injectionFirstWrite =
    a?.mode === "cache_threshold" && a.points?.length
      ? Math.min(...a.points.filter((point) => (point.write ?? 0) > 0).map((point) => point.n))
      : null;
  const thirdCache = cacheSamples.length ? cacheSamples[cacheSamples.length - 1] : undefined;
  const previousCache = cacheSamples.length > 1 ? cacheSamples[cacheSamples.length - 2] : undefined;
  const thirdTotal = thirdCache && thirdCache.read_reported !== false ? thirdCache.total : null;
  const previousCacheTotal = previousCache
    ? cache?.third_read_baseline != null
      ? cache.third_read_baseline
      : previousCache.read_reported !== false
        ? previousCache.write_reported === true
          ? previousCache.read + previousCache.write
          : previousCache.read
        : null
    : null;
  const cacheKnown = cache?.status !== "unknown" && cache?.third_read_ratio != null;
  const reportedReads = cacheSamples.filter(
    (row) => row.read_reported !== false && !row.error,
  ).length;
  const readOnlyCache =
    cacheSamples.length > 1 &&
    cacheSamples.every((row) => row.write_reported === false && !row.error) &&
    reportedReads >= 2;
  const cacheToken = (value?: number | null, reported?: boolean) =>
    reported === false || value == null || !Number.isFinite(value) ? "—" : value;
  const cacheRate = (value?: number | null) =>
    value == null || !Number.isFinite(value) ? "—" : value.toFixed(2) + "%";
  const behavioral = f?.basis === "behavioral";
  const sampleLimit =
    f?.sample_limit ?? (f?.confidence_reached === undefined ? 3 : 10);
  const fpState =
    !f || f.error || f.low_confidence
      ? "match"
      : fingerprintState(model || "", f.family || "", f.top_model || "", {
          behavioral,
          prob: f.top_prob,
        });
  const diluted = fpState === "fake";
  const suspicious = fpState === "suspicious";
  const strictChecks = t?.strictness ?? [];
  const strictAnswered = strictChecks.filter((c) => !c.skipped).length;
  const strictPassed = strictChecks.filter((c) => c.pass && !c.skipped).length;
  const scored: DetectVerdict = v.total
    ? {
        ...v,
        total: {
          ...v.total,
          ...presentedTotal(v.total.score, v.total.parts, fpState, v.anticheat),
        },
      }
    : v;
  const anticheat = v.anticheat;
  return (
    <div
      className={"results " + (loading ? "results-loading" : "results-settled")}
    >
      <ScoreHero verdict={scored} fp={fpState} claude={claude} loading={loading} progress={progress} />
      {anticheat?.detected && (anticheat.signals?.length ?? 0) > 0 && (
        <section className="result-card anticheat-card" aria-labelledby="anticheat-title">
          <div className="rc-head">
            <div className="rc-title">
              <h3 id="anticheat-title">{tr("反作弊实锤证据")}</h3>
              <span className="ac-badge">{tr("{n} 项命中", { n: anticheat.signals.length })}</span>
            </div>
          </div>
          {anticheat.summary && <p className="muted">{trAntiCheatSummary(anticheat.summary, tr)}</p>}
          <ul className="ac-signals">
            {anticheat.signals.map((sig) => (
              <li key={sig.code}>
                <div className="ac-signal-head">
                  <span className="ac-section">{tr(ANTICHEAT_SECTION_LABEL[sig.section] || sig.section)}</span>
                  <strong>{tr(sig.title)}</strong>
                </div>
                <p>{tr(sig.detail)}</p>
              </li>
            ))}
          </ul>
        </section>
      )}
      <Group id="group-identity" title="身份与渠道">
      <section className={"result-card" + (diluted ? " fingerprint-bad" : "")}>
        <div className="rc-head">
          <CardTitle title="模型指纹归因" label="指纹归因说明" text={trNote(f?.note, tr)} />
          <span className="rc-score">
            <ScoreShare verdict={scored} part="fingerprint" />
          </span>
        </div>
        {!f ? (
          <ResultSkeleton label={loading ? tr("指纹检测中…") : tr("暂无结果")} />
        ) : (
          <>
            {f.error ? (
              <ErrorFold text={f.error} />
            ) : (
              <>
                <div className={"fp-claim-band" + (diluted || suspicious ? " has-seal" : "")}>
                  <div className="model-claim">
                    <p>
                      <span>{tr("声称")}</span>
                      <b>{model || tr("未填写")}</b>
                    </p>
                    <p>
                      <span>{tr("实际")}</span>
                      <b className={diluted ? "bad" : suspicious ? "warn" : ""}>
                        {f.top_display || f.top_model || f.family_name || tr("未归因")}
                      </b>
                    </p>
                  </div>
                  {(diluted || suspicious) && (
                    <div className={"fake-seal is-round" + (suspicious ? " suspicious" : "")} aria-hidden="true">
                      <span>{tr(diluted ? "假" : "可疑")}</span>
                    </div>
                  )}
                </div>
                <p className="basis-line">
                  <span className={"basis-chip" + (behavioral ? " behavioral" : "")}>
                    {behavioral ? tr("行为归因") : tr("数字归因")}
                  </span>
                  <span className="muted">
                    {behavioral
                      ? tr("数字库没有该家族，已用行为库 {n} 个候选直接比对。", {
                          n: f.behavioral_total ?? 34,
                        })
                      : tr("按数字基线候选比较。")}
                    {f.low_confidence ? " " + tr("证据不足，不判掺水。") : ""}
                  </span>
                </p>
                <div className="metric-line">
                  <strong className={diluted ? "bad family-title" : suspicious ? "warn family-title" : "family-title"}>
                    {tr(f.family_name || f.family)}
                  </strong>
                  <span>
                    {behavioral
                      ? tr("行为相似度 {n}%", {
                          n: ((f.top_similarity ?? f.top_prob ?? 0) * 100).toFixed(1),
                        })
                      : tr("家族概率 {n}%", {
                          n: (f.family_prob * 100).toFixed(1),
                        })}
                  </span>
                </div>
                {f.results
                  ?.filter((r) => probabilityVisible(r.probability))
                  .slice(0, 3)
                  .map((r) => (
                    <div className="candidate" key={r.model}>
                      <div>
                        <span>{r.display || r.model}</span>
                        <b>{(r.probability * 100).toFixed(1)}%</b>
                      </div>
                      <div className="prob-bar">
                        <div
                          style={{ transform: "scaleX(" + r.probability + ")" }}
                        />
                      </div>
                    </div>
                  ))}
                <p className="muted">
                  {behavioral
                    ? tr("行为探针 3 个 · 相似度按 6 维特征比对")
                    : <>
                        {tr("有效样本 {a} / {b}", {
                          a: f.used_outputs,
                          b: sampleLimit,
                        })}
                        {typeof f.requests === "number"
                          ? tr(" · 请求 {n} / 10", { n: f.requests })
                          : ""}
                      </>}
                </p>
                {!behavioral &&
                  f.confidence_reached === true &&
                  (f.used_outputs ?? 0) < sampleLimit && (
                    <p className="muted">
                      {tr("顶部概率达到 90%，已提前停止采样。")}
                    </p>
                  )}
                {!behavioral &&
                  f.confidence_reached === false &&
                  (f.used_outputs ?? 0) >= sampleLimit && (
                    <p className="muted">
                      {tr("已取满 {n} 个样本，顶部概率未达到 90%。", {
                        n: sampleLimit,
                      })}
                    </p>
                  )}
                {!behavioral && f.behavioral_top3 && (
                  <Details
                    label={tr("行为相似候选（探索性参考）")}
                    value={{
                      candidates: f.behavioral_top3,
                      note: trNote(f.behavior_note, tr),
                    }}
                  />
                )}
              </>
            )}
            {!behavioral && f.evidence?.length ? (
              <Details label={tr("数字挑战原始证据")} value={f.evidence} />
            ) : null}
          </>
        )}
      </section>
      <section className="result-card">
        <div className="rc-head">
          <CardTitle title="注入大小" label="注入估计说明" text={trNote(a?.note, tr)} />
          <span className="rc-score">
            <ScoreShare verdict={scored} part="injection" />
          </span>
        </div>
        {!a ? (
          <ResultSkeleton label={loading ? tr("采样中…") : tr("暂无结果")} />
        ) : a.error ? (
          <ErrorFold text={a.error} />
        ) : (
          <>
            <div className="metric-line">
              {a.mode === "cache_threshold" && a.score != null ? (
                <>
                  <strong>
                    ≈ <AnimatedNumber value={a.inj} />
                  </strong>
                  <span>{tr("隐藏 token")}</span>
                  <b className="score-chip">{a.score + " / 10"}</b>
                </>
              ) : a.score != null && a.reliable !== false ? (
                <>
                  <strong>
                    ≈ <AnimatedNumber value={a.inj} />
                  </strong>
                  <span>{tr("token / 请求")}</span>
                  <b className="score-chip">{a.score + " / 10"}</b>
                </>
              ) : (
                <>
                  <strong>{tr("样本异常")}</strong>
                  <span>{tr("未计分")}</span>
                </>
              )}
            </div>
            <div className="mini-stats">
              {a.mode !== "cache_threshold" && typeof a.slope === "number" && (
                <span>
                  {tr("斜率")} <b>{a.slope.toFixed(3)}</b>
                </span>
              )}
              <span>
                {a.mode === "cache_threshold" ? tr("扫描样本") : tr("复用采样")} <b>{tr("{n} 次", { n: (a.points ?? []).length })}</b>
              </span>
              {(a.points ?? []).length > 2 && a.max_residual != null && (
                <span>
                  {tr("最大残差")} <b>{a.max_residual.toFixed(1)}</b>
                </span>
              )}
            </div>
            {a.mode === "cache_threshold" && a.nominal ? (
              <div className="inject-threshold">
                <div className="inject-bar" title={tr("绿色是能写入的计费前缀，红色是网关额外加、但未计费的隐藏前缀")}>
                  <span
                    className="inject-bar-fill"
                    style={{ width: `${Math.min(100, ((a.threshold ?? 0) / a.nominal) * 100)}%` }}
                  />
                  <span
                    className="inject-bar-gap"
                    style={{ width: `${Math.max(0, 100 - ((a.threshold ?? 0) / a.nominal) * 100)}%` }}
                  />
                </div>
                <div className="inject-bar-labels">
                  <span title={tr("第一次出现缓存写入的计费 token 数")}>{tr("首写")} <b>{a.threshold ?? "—"}</b></span>
                  <span title={tr("该模型/缓存类型的理论最低缓存长度")}>{tr("名义门槛")} <b>{a.nominal}</b></span>
                  <span className={a.inj ? "bad" : "good"} title={tr("名义门槛 - 首写阈值；网关额外加的系统前缀")}>{tr("隐藏")} <b>{a.inj}</b></span>
                </div>
              </div>
            ) : null}
            <details className="fold">
              <summary>{a.mode === "cache_threshold" ? tr("扫描明细（调试）") : tr("采样明细与评分标准")}</summary>
              <div className="table-wrap">
                <table>
                  <thead>
                    <tr>
                      <th title={tr("二分探测顺序，按文本长度排序")}>{a.mode === "cache_threshold" ? tr("扫描") : tr("轮次")}</th>
                      <th title={tr("这次探测的 system 文本字符数")}>{tr("文本长度")}</th>
                      <th title={tr("上游回传的未缓存输入 token")}>{tr("输入")}</th>
                      <th title={tr("这次探测写入缓存的 token；第一次大于 0 就是首写阈值")}>{tr("写入")}</th>
                      <th title={tr("输入 + 缓存读取 + 缓存写入")}>{tr("合计")}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {a.points.map((p, index) => (
                      <tr key={p.round || p.n}>
                        <td title={JSON.stringify(p)}>{p.round || index + 1}</td>
                        <td title={JSON.stringify(p)}>{p.n}</td>
                        <td title={JSON.stringify(p)}>{p.input}</td>
                        <td title={JSON.stringify(p)}>
                          {p.write ? <b className={p.n === injectionFirstWrite ? "good" : ""}>{p.write}</b> : "—"}
                        </td>
                        <td title={JSON.stringify(p)}>{p.total}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
              {a.round_overhead != null && (
                <p className="muted">{tr("每轮新增包装修正 {n} token", { n: a.round_overhead.toFixed(2) })}</p>
              )}
              <p className="muted">
                {tr("<50：10分 · 50～1000：8分 · 1001～3000：6分 · 3001～6000：4分 · 6001～10000：2分 · >10000：0分")}
              </p>
            </details>
            <Details
              label={
                a.probe
                  ? tr("SSE 与缓存头诊断（不计分）")
                  : tr("缓存头诊断（不增加请求）")
              }
              value={{ ...a.probe, cache_header: a.cache_header }}
            />
          </>
        )}
      </section>
      {!v.total && integrity && (
        <section className="result-card integrity-card">
          <div className="rc-head">
            <CardTitle title="接口完整性" label="接口完整性说明" text="只评价协议和证据链，不代表模型智力。未支持或缺字段会降低覆盖率，不会伪判为失败。" />
          </div>
          <div className="metric-line">
            <strong className={integrity.score == null ? "warn" : "good"}>
              {integrity.score == null ? "—" : <AnimatedNumber value={integrity.score} />}
            </strong>
            <span>/ 100</span>
            <b className="score-chip">{tr("覆盖 {n}%", { n: integrity.coverage.toFixed(0) })}</b>
          </div>
          <p className="muted">{tr(integrity.reason || "")}</p>
          <div className="integrity-signals">
            {integrity.signals.filter(s => s.applicable).map(signal => (
              <div className="integrity-signal" key={signal.key}>
                <span>{tr(signal.label)}</span>
                <b className={!signal.known ? "warn" : signal.points && signal.points > 0 ? "good" : "bad"}>
                  {!signal.known ? tr("未验证") : signal.points && signal.max_points > 1 ? `${signal.points}/${signal.max_points}` : signal.points ? tr("通过") : tr("未通过")}
                </b>
              </div>
            ))}
          </div>
        </section>
      )}
      {claude && (
        <section className="result-card">
          <div className="rc-head">
            <h3>{tr("思维链回传")}</h3>
            <span className="rc-score">
              <ScoreShare verdict={scored} part="thinking" />
            </span>
          </div>
          {!t ? (
            <ResultSkeleton label={loading ? tr("等待思维链…") : tr("暂无结果")} />
          ) : t.error || t.err ? (
            <ErrorFold text={t.error || t.err || ""} />
          ) : (
            <>
              <div className="metric-line">
                <strong className={((t.score ?? t.summary_score ?? (t.think_len > 0 ? 5 : 0)) === (t.max_score ?? 5)) ? "good" : "warn"}>
                  {t.score ?? t.summary_score ?? (t.think_len > 0 ? 5 : 0)} / {t.max_score ?? 5}
                </strong>
              </div>
              <div className="check-list">
                <span className="check-item">
                  <i>{tr("签名")}</i>
                  <b className={t.signature_present ? "ok" : "no"}>
                    {t.signature_present ? tr("有") : tr("无")}
                    {t.signature_present && t.source ? " · " + tr("复用") + " " + t.source : ""}
                  </b>
                </span>
                {t.signature_present && (
                  <span className="check-item">
                    <i>{tr("签名模型")}</i>
                    <b className={!t.signed_model ? "skip" : t.signed_model_match ? "ok" : "warn"}>
                      {t.signed_model || tr("不透明")}
                    </b>
                  </span>
                )}
                {t.sig_class && (
                  <span className="check-item" title={t.sig_class}>
                    <i>{tr("签名结构")}</i>
                    <b className={t.sig_forged || t.sig_class.startsWith("伪造") ? "no" : t.sig_class.startsWith("真签") || t.sig_class.startsWith("标准信封") ? "ok" : "warn"}>
                      {tr(sigShort(t.sig_class))}
                      {sigBytes(t.sig_class) ? " · " + sigBytes(t.sig_class) : ""}
                    </b>
                  </span>
                )}
                <span className="check-item">
                  <i>{tr("原始续轮")}</i>
                  <b className={t.original_replay ? "ok" : "no"}>{t.original_replay ? tr("通过") : tr("失败")}</b>
                </span>
                <span className="check-item">
                  <i>{tr("篡改")}</i>
                  <b className={tamperView(t, tr).cls}>{tamperView(t, tr).text}</b>
                </span>
                {t.max_score === 12 && (
                  <span className="check-item">
                    <i>temperature</i>
                    <b className={t.temperature_rejected ? "ok" : "no"}>{t.temperature_rejected ? "400" : tr("未拦截")}</b>
                  </span>
                )}
                {typeof t.aws_b === "boolean" && (
                  <span className="check-item" title={t.aws_b_detail || ""}>
                    <i>AWS-B</i>
                    <b className={t.aws_b ? "ok" : "skip"}>{t.aws_b ? tr("是") : tr("否")}</b>
                  </span>
                )}
              </div>
              {t.reason ? <p className="muted">{trThinkingReason(t.reason, tr)}</p> : null}
              <Details
                label={tr("签名校验回显")}
                value={{
                  self_report: t.self_report,
                  family_consistent: t.family_consistent,
                  signed_model: t.signed_model,
                  signed_model_match: t.signed_model_match,
                  sig_class: t.sig_class,
                  sig_field_count: t.sig_field_count,
                  sig_infra: t.sig_infra,
                  sig_timestamp: t.sig_timestamp,
                  sig_age_seconds: t.sig_age_seconds,
                  tamper_status: t.tamper_status,
                  tamper_error: t.tamper_error,
                  temperature_error: t.temperature_error,
                }}
              />
              <Details label={tr("查看思维链全文")} value={t.thinking_text} />
              <Details label={tr("探针回答")} value={t.answer} />
            </>
          )}
        </section>
      )}
      {claude && strictChecks.length > 0 && (
        <section className="result-card">
          <div className="rc-head">
            <CardTitle
              title="协议校验"
              label="协议校验说明"
              text="向渠道发 10 个官方 API 必定拒绝的请求：缺 max_tokens、未知字段、system role 等。被拒绝才算通过；上游不可用、限流或超时算不定，不计分。"
            />
            <span className="rc-score">
              <ScoreShare verdict={scored} part="strictness" />
            </span>
          </div>
          <div className="metric-line">
            <strong className={strictPassed === strictAnswered ? "good" : "warn"}>
              {strictPassed} / {strictAnswered}
            </strong>
          </div>
          <div className="check-rows strict-check-rows">
            {strictChecks.map((c) => (
              <span key={c.name} className="check-row" title={c.detail ? translateError(c.detail, tr) : ""}>
                <i>{tr(c.title || c.name)}</i>
                <b className={c.skipped ? "skip" : c.pass ? "ok" : "no"}>
                  {c.skipped ? tr("跳过") : c.pass ? tr("已拒绝") : tr("未拒绝") + " " + c.status}
                </b>
              </span>
            ))}
          </div>
        </section>
      )}
      {claude && v.claude_deep && (
        <section className="result-card">
          <div className="rc-head">
            <h3>{tr("Claude 深度体检")}</h3>
            <span className="rc-score">
              <ScoreShare verdict={scored} part="deep" />
            </span>
          </div>
          <div className="check-list">
            {v.claude_deep.checks.map((c) => (
              <span key={c.name} className="check-item" title={c.detail ? translateError(c.detail, tr) : ""}>
                <i>{tr(c.title || c.name)}</i>
                <b className={c.skipped ? "skip" : c.pass ? "ok" : "no"}>
                  {c.skipped ? tr("跳过") : c.pass ? tr("通过") : tr("未通过")}
                </b>
              </span>
            ))}
          </div>
          {v.claude_deep.note ? <p className="muted">{tr(v.claude_deep.note)}</p> : null}
          {v.claude_deep.error ? <ErrorFold text={v.claude_deep.error} /> : null}
        </section>
      )}
      </Group>
      <Group id="group-ability" title="能力表现">
      <section className={"result-card" + (v.conversation ? " conversation-wide" : "")}>
        <div className="rc-head">
          <CardTitle title="推理表现" label="评分说明" text={trNote(q?.note, tr)} />
          <span className="rc-score">
            <ScoreShare verdict={scored} part="iq" />
          </span>
        </div>
        {!q || (loading && q.per_question.length === 0) ? (
          <ResultSkeleton label={loading ? tr("推理测试中…") : tr("暂无结果")} />
        ) : (
          (() => {
            const iqProbes = (probes || []).filter((p) => /^Q\d+$/i.test(p.id));
            const doneIds = new Set(q.per_question.map((x) => x.id));
            const pendingProbes = loading
              ? iqProbes.filter(
                  (p) => !doneIds.has(p.id) && (p.status === "running" || p.status === "queued"),
                )
              : [];
            const iqTotal = q.total_questions || iqProbes.length || q.per_question.length;
            const iqDone = q.done_count ?? q.per_question.length;
            const iqInProgress = loading && !q.complete && iqTotal > iqDone;
            return (
              <>
                <div className="metric-line">
                  <strong>
                    {q.score == null ? "—" : <AnimatedNumber value={q.score} />}
                  </strong>
                  <span>/ 100</span>
                  <b className={"score-chip" + (iqInProgress ? " is-live" : "")}>
                    {q.complete
                      ? tr("本次测评")
                      : iqInProgress
                        ? tr("已完成 {done} / {total} 题", { done: iqDone, total: iqTotal })
                        : tr("尚未完成")}
                  </b>
                </div>
                {v.conversation && <ConversationPanel data={v.conversation} />}
                <div className="question-list">
                  {q.per_question.map((x, index) => {
                    const qCode = /^Q\d+$/i.test(x.id) ? x.id.toUpperCase() : `Q${index + 1}`;
                    return (
                      <details key={x.id} className="question">
                        <summary>
                          <span>
                            {qCode} · {tr(x.title)}
                          </span>
                          <b
                            className={
                              x.skipped
                                ? "warn"
                                : x.error
                                  ? "bad"
                                  : x.earned === x.max
                                    ? "good"
                                    : "warn"
                            }
                          >
                            {x.skipped
                              ? tr("不适用")
                              : x.error
                                ? tr("请求失败")
                                : x.earned + " / " + x.max + (x.half ? tr("（半分）") : "")}
                          </b>
                        </summary>
                        {x.skipped && <p>{x.error ? tr(x.error) : null}</p>}
                        {x.error && !x.skipped && <ErrorFold text={x.error} />}
                        <h4>{tr("题目")}</h4>
                        <pre>{x.prompt}</pre>
                        {x.image && <img className="question-image" src={x.image} alt="" />}
                        <h4>{tr("标准结论")}</h4>
                        <pre>{x.expected}</pre>
                        <h4>{tr("模型回答")}</h4>
                        <pre>{x.answer || tr("正文为空，使用已返回思考文本判分")}</pre>
                        {claude && (
                          <Details
                            label={tr("思维链全文与签名")}
                            value={{
                              thinking: x.thinking_text,
                              signature: x.signature,
                            }}
                          />
                        )}
                        <Details
                          label={tr("用量与耗时")}
                          value={{
                            [tr("输入")]: x.metrics.InputTokens,
                            [tr("输出")]: x.metrics.OutputTokens,
                            [tr("思考程度")]: x.metrics.EffortFallback
                              ? tr("high 被上游拒绝，回退不指定")
                              : x.metrics.Effort || tr("不指定"),
                            [tr("缓存读")]: x.metrics.CacheRead,
                            [tr("缓存写")]: x.metrics.CacheCreation5m + x.metrics.CacheCreation1h,
                            [tr("耗时")]: x.ms,
                          }}
                        />
                        {x.tags?.length > 0 && (
                          <div className="question-tags">
                            {x.tags.map((tag) => (
                              <span className="question-tag is-pill" key={tag}>
                                {trTag(tag, tr)}
                              </span>
                            ))}
                          </div>
                        )}
                      </details>
                    );
                  })}
                  {pendingProbes.length > 0
                    ? pendingProbes.map((p) => (
                        <div key={p.id} className="question-pending-row" aria-busy="true">
                          <span>
                            {p.id.toUpperCase()} · {tr(p.name)}
                          </span>
                          <b className={p.status === "running" ? "is-running" : "is-queued"}>
                            <i className="q-live-dot is-round" aria-hidden="true" />
                            {p.status === "running" ? tr("检测中") : tr("等待中")}
                          </b>
                        </div>
                      ))
                    : iqInProgress && (
                        <div className="question-pending-row is-summary" aria-busy="true">
                          <span>
                            <i className="q-live-dot is-round" aria-hidden="true" />
                            {tr("其余 {n} 题测试中…", { n: iqTotal - iqDone })}
                          </span>
                        </div>
                      )}
                </div>
              </>
            );
          })()
        )}
      </section>
      {(vision || loading) && (
        <section className="result-card vision-card">
          <div className="rc-head">
            <CardTitle
              title="识图能力"
              label="评分说明"
              text={trNote(vision?.note, tr)}
            />
            <span className="rc-score">
              <ScoreShare verdict={scored} part="vision" />
            </span>
          </div>
          {!vision ? (
            <ResultSkeleton label={loading ? tr("识图测试中…") : tr("暂无结果")} />
          ) : (
            <>
              <div className="metric-line">
                <strong className={vision.recognized ? "good" : "bad"}>
                  {vision.recognized ? tr("识图成功") : tr("识图失败")}
                </strong>
                <span>{tr("识图")}</span>
                <b className={"score-chip" + (vision.recognized_score === 0 ? " is-zero" : "")}>
                  {vision.recognized_score} / {vision.recognized_max}
                </b>
              </div>
              <div className="metric-line">
                <strong
                  className={
                    vision.correct ? "good" : vision.answer_score > 0 ? "warn" : "bad"
                  }
                >
                  {vision.correct
                    ? tr("答对")
                    : vision.answer_score > 0
                      ? tr("部分正确")
                      : tr("答错")}
                </strong>
                <span>{tr("答对")}</span>
                <b className={"score-chip" + (vision.answer_score === 0 ? " is-zero" : "")}>
                  {vision.answer_score} / {vision.answer_max}
                </b>
              </div>
              {vision.error && <p className="muted">{tr(vision.error)}</p>}
              {vision.question && (
                <details className="question" open>
                  <summary>
                    <span>{tr("K线读数")}</span>
                    <b
                      className={
                        vision.correct
                          ? "good"
                          : vision.answer_score > 0
                            ? "warn"
                            : "bad"
                      }
                    >
                      {vision.earned ?? 0} / {vision.question_max ?? vision.question.max}
                    </b>
                  </summary>
                  {vision.question.image && (
                    <img className="question-image" src={vision.question.image} alt="" />
                  )}
                  <h4>{tr("标准结论")}</h4>
                  <pre>{vision.question.expected}</pre>
                  <h4>{tr("模型回答")}</h4>
                  <pre>{vision.question.answer || tr("正文为空，使用已返回思考文本判分")}</pre>
                </details>
              )}
            </>
          )}
        </section>
      )}
      {(
        [
          [tr("工具调用检测"), v.tools],
          [tr("联网检测"), v.web],
        ] as const
      ).map(([title, result]) => (
        <section className="result-card" key={title}>
          <div className="rc-head">
            <h3>{title}</h3>
            <span className="rc-score">
              {result?.protocol === "deepseek_search" && (
                <span className="basis-chip behavioral">DeepSeek Search API</span>
              )}
              <ScoreShare
                verdict={scored}
                part={title === tr("工具调用检测") ? "tools" : "web"}
              />
            </span>
          </div>
          {!result ? (
            <ResultSkeleton label={loading ? tr("检测中…") : tr("暂无结果")} />
          ) : result.error ? (
            <ErrorFold text={result.error} />
          ) : (
            <>
              <div className="metric-line">
                <strong className={result.pass === true ? "good" : "warn"}>
                  {result.pass === true
                    ? tr("通过")
                    : result.pass === false
                      ? tr("未通过")
                      : tr("未经验证")}
                </strong>
              </div>
              <p className="muted">{trCapabilityReason(result.reason || "", tr)}</p>
              {result.evidence != null && (
                <Details label={tr("检测证据")} value={result.evidence} />
              )}
            </>
          )}
        </section>
      ))}
      <section className="result-card knowledge-card">
        <div className="rc-head">
          <h3>{tr("知识截止日期")}</h3>
        </div>
        {!knowledge ? (
          <ResultSkeleton label={loading ? tr("知识截止探针中…") : tr("暂无结果")} />
        ) : knowledge.error ? (
          <ErrorFold text={knowledge.error} />
        ) : (
          <>
            <div className="metric-line">
              <strong className={knowledge.correct ? "good" : "warn"}>
                {knowledgeLabel(knowledge, tr)}
              </strong>
              <span>{tr("客观事件")}</span>
            </div>
            <p className="muted">{trKnowledgeReason(knowledge.reason || "", tr)}</p>
            {knowledge.web_suspect && <p className="knowledge-note">{trKnowledgeSuspect(knowledge.web_suspect, tr)}</p>}
            {knowledge.gap_note && <p className="knowledge-note">{trKnowledgeGap(knowledge.gap_note, tr)}</p>}
            {!!knowledge.rows?.length && (
              <ul className="knowledge-ladder">
                {knowledge.rows.map((row) => (
                  <li key={row.id}>
                    <b className={knowledgeMark(row, tr).cls}>{knowledgeMark(row, tr).text}</b>
                    <span>{row.happened_at || row.quarter}</span>
                    <span>{row.event}</span>
                  </li>
                ))}
              </ul>
            )}
          </>
        )}
      </section>
      </Group>
      {(showCache || cache || (claude && v.force_cache)) && (
        <Group id="group-cache" title="缓存">
      {(showCache || cache) && <section className="result-card cache-card">
        <div className="rc-head">
          <h3>{tr("缓存读写验证")}</h3>
          <span className="rc-score">
            <ScoreShare verdict={scored} part="cache" />
          </span>
        </div>
        {!v.cache ? (
          <ResultSkeleton label={loading ? tr("缓存验证中…") : tr("暂无结果")} />
        ) : v.cache.error ? (
          <ErrorFold text={v.cache.error} />
        ) : (
          <>
            <div className="metric-line">
              <strong className={v.cache.pass ? "good" : "warn"}>
                {v.cache.pass === true
                  ? tr("通过")
                  : v.cache.pass === false
                    ? tr("未对齐")
                    : tr("无法验证")}
              </strong>
            </div>
            {v.cache.reason ? <p className="muted">{trConvReason(v.cache.reason, tr)}</p> : null}
            {v.cache.samples ? (
              <>
                <div className="cache-rates">
                  <div>
                    <span>{tr("末轮缓存率")}</span>
                    <strong>{cacheKnown ? cacheRate(v.cache.third_read_ratio) : "—"}</strong>
                  </div>
                  <div>
                    <span>{tr("末轮读取")}</span>
                    <strong>{cacheToken(thirdCache?.read, thirdCache?.read_reported)}</strong>
                  </div>
                  <div>
                    <span>{tr("末轮总输入")}</span>
                    <strong>{cacheToken(thirdTotal, thirdCache?.read_reported)}</strong>
                  </div>
                  <div>
                    <span>{readOnlyCache ? tr("上一轮读取") : tr("上一轮读 ＋ 写")}</span>
                    <strong>{cacheToken(previousCacheTotal)}</strong>
                  </div>
                </div>
                <p className="muted">
                  {cacheKnown && readOnlyCache
                    ? `${cacheToken(thirdCache?.read, thirdCache?.read_reported)} ÷ ${cacheToken(thirdTotal, thirdCache?.read_reported)}`
                    : cacheKnown && previousCacheTotal != null
                    ? `${cacheToken(thirdCache?.read, thirdCache?.read_reported)} ÷ ${cacheToken(previousCacheTotal)} × 100%`
                    : "—"}
                  <Hint
                    label="末轮缓存率说明"
                    text={
                      readOnlyCache
                        ? "上游没有缓存写入时，末轮缓存率 = 末轮读取 ÷ 末轮总输入。上一轮读取作为读取链参考，缺写不当成 0。"
                        : "上一条缓存读 + 写 = 下一条缓存读，就是 100%。末轮缓存率 = 末轮读取 ÷ 上一轮读加写。缺字段或请求失败时不计算。"
                    }
                  />
                </p>
                {v.cache.third_read_ratio != null && v.cache.third_read_ratio > 100 && (
                  <p className="muted">{tr("末轮读取超过上一轮读加写，保留原始比例。")}</p>
                )}
                <div className={"table-wrap" + (conversationCache ? " cache-table-hidden" : "")}>
                  <table className="cache-table">
                    <thead>
                      <tr>
                        <th title={tr("请求序号")}>{tr("请求")}</th>
                        <th title={tr("缓存读取 token")}>{tr("读")}</th>
                        <th title={tr("缓存写入 token")}>{tr("写")}</th>
                        <th title={tr("未命中缓存的输入 token")}>{tr("输入")}</th>
                        <th title={tr("输出 token")}>{tr("输出")}</th>
                        <th title={tr("读 ÷ 总输入")}>{tr("命中率")}</th>
                        <th title={tr("接上 / 未接续 / 整段重写")}>{tr("状态")}</th>
                      </tr>
                    </thead>
                    <tbody>
                      {v.cache.samples.map((row) => (
                        <tr key={row.index}>
                          <td>#{row.index}</td>
                          <td>{cacheToken(row.read, row.read_reported)}</td>
                          <td>{cacheToken(row.write, row.write_reported)}</td>
                          <td>{cacheToken(row.input, row.input_reported)}</td>
                          <td>{cacheToken(row.output, row.output_reported)}</td>
                          <td>{cacheRate(row.hit_rate)}</td>
                          <td className={cacheTransitions.find((x) => x.to === row.index)?.pass === false ? "bad" : ""}>
                            {cacheTransitions.find((x) => x.to === row.index)?.pass === false
                              ? row.read === 0
                                ? tr("整段重写")
                                : tr("未接续")
                              : row.index > 1 && !row.error && row.read === 0 && row.write > 64
                                ? tr("整段重写")
                                : ""}
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </>
            ) : (
              <div className="cache-equation">
                <span>
                  {tr("前次读")} <b>{v.cache.first?.read ?? "—"}</b>
                </span>
                <span>
                  {tr("＋ 写")} <b>{v.cache.first?.write ?? "—"}</b>
                </span>
                <span>
                  {tr("→ 后次读")} <b>{v.cache.second?.read ?? "—"}</b>
                </span>
              </div>
            )}
            {!conversationCache && (
              <Details
                label={v.cache.samples ? tr("多轮缓存明细与衔接") : tr("旧报告缓存明细")}
                value={v.cache}
              />
            )}
            {conversationCache && (
              <p className="muted cache-conv-link">{tr("明细见「推理表现」。")}</p>
            )}
          </>
        )}
      </section>}
      {claude && v.force_cache && (
        <section className="result-card">
          <div className="rc-head">
            <h3>{tr("强制缓存检测")}</h3>
            <span className="rc-score">
              <ScoreShare verdict={scored} part="forcecache" />
            </span>
          </div>
          {typeof v.force_cache.forced === "boolean" && (
            <div className="check-list">
              <span className="check-item">
                <i>{tr("无参数写缓存")}</i>
                <b className={v.force_cache.forced ? "no" : "ok"}>{v.force_cache.forced ? tr("会（强制）") : tr("不会")}</b>
              </span>
              {typeof v.force_cache.tier_5m === "boolean" && (
                <span className="check-item">
                  <i>{tr("5 分钟档")}</i>
                  <b className={v.force_cache.tier_5m ? "ok" : "no"}>{v.force_cache.tier_5m ? tr("生效") : tr("未生效")}</b>
                </span>
              )}
              {typeof v.force_cache.tier_1h === "boolean" && (
                <span className="check-item">
                  <i>{tr("1 小时档")}</i>
                  <b className={v.force_cache.tier_1h ? "ok" : "no"}>{v.force_cache.tier_1h ? tr("生效") : tr("未生效")}</b>
                </span>
              )}
            </div>
          )}
          {v.force_cache.conclusion ? <p className="muted">{trForceCacheConclusion(v.force_cache.conclusion, tr)}</p> : null}
          <div className="check-rows force-cache-rows">
            {v.force_cache.steps.map((s, index) => (
              <span key={s.key + "-" + index} className="check-row">
                <i>{tr(s.label) + " · " + (index % 2 === 0 ? tr("首写") : tr("重放"))}</i>
                <b>
                  input {s.input} · 5m {s.write_5m} · 1h {s.write_1h} · read {s.read}
                </b>
              </span>
            ))}
          </div>
          {v.force_cache.note ? <p className="muted">{tr(v.force_cache.note)}</p> : null}
          {v.force_cache.error ? <ErrorFold text={v.force_cache.error} /> : null}
        </section>
      )}
        </Group>
      )}
    </div>
  );
}