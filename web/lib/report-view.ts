import type { DetectVerdict } from "@/lib/api";

// Formatting only: scoring and fingerprint inference remain on the Go backend.
export function reportHost(base: string): string {
  try {
    return new URL(base).host;
  } catch {
    return "未知站点";
  }
}
export function reportPath(id: string): string {
  return "/history?id=" + encodeURIComponent(id);
}
export function parseReportID(value: string): string {
  let id = value.trim();
  try {
    const url = new URL(id);
    id = url.searchParams.get("id") || "";
  } catch {}
  return /^[a-f0-9]{32}$/i.test(id) ? id.toLowerCase() : "";
}
export function probabilityVisible(probability: number): boolean {
  return (
    Number.isFinite(probability) &&
    probability > 0 &&
    Number((probability * 100).toFixed(1)) > 0
  );
}

export function modelFamily(model: string): string {
  const m = model.trim().toLowerCase().replace(/^[^/]+\//, "");
  if (m.startsWith("claude")) return "claude";
  if (m.startsWith("gpt") || /^o[134](?:[.-]|$)/.test(m) || m.includes("chatgpt")) return "gpt";
  if (m.startsWith("gemini")) return "gemini";
  if (m.startsWith("deepseek")) return "deepseek";
  if (m.startsWith("grok")) return "grok";
  if (m.startsWith("glm")) return "glm";
  if (m.startsWith("kimi")) return "kimi";
  return "";
}
export function fingerprintFamily(family: string): string {
  const f = family.trim().toLowerCase();
  if (f === "openai" || f === "chatgpt") return "gpt";
  if (f === "anthropic") return "claude";
  if (f === "xai" || f === "x-ai") return "grok";
  if (f === "zhipu" || f === "z-ai") return "glm";
  if (f === "google") return "gemini";
  return f;
}
export function canonModel(model: string): string {
  model = model.trim().toLowerCase().replace(/^[^/]+\//, "");
	if (model.trim().toLowerCase() === "deepseek-flash") model = "deepseek-v4.1-flash-1017";
  return model.toLowerCase().replace(/[^a-z0-9]+/g, "");
}

export type FingerprintState = "match" | "suspicious" | "fake";

// Mirrors integrity.FingerprintState on the Go side. A different family or a
// different named Claude line is a confirmed substitution; any other name
// mismatch may just be a name the library does not carry, so it stays
// suspicious. Without a numeric baseline for the claimed model (behavioral
// fallback) there is no verdict at all, and a substitution only counts as
// confirmed at 99.9% or better.
export function fingerprintState(
  requested: string,
  family: string,
  topModel = "",
  opts: { behavioral?: boolean; prob?: number } = {},
): FingerprintState {
  // Behavioral attribution has no numeric baseline, so it never judges a model.
  if (opts.behavioral) return "match";
  const differs = claimDiffers(requested, family, topModel);
  if (!differs) return "match";
  return (opts.prob ?? 0) >= 0.999 ? "fake" : "suspicious";
}

function claimDiffers(requested: string, family: string, topModel: string): boolean {
  const want = modelFamily(requested);
  const got = fingerprintFamily(family);
  if (want && got && got !== "unknown" && want !== got) return true;
  const line = (value: string) => ["opus", "sonnet", "haiku"].find((item) => value.toLowerCase().includes(item)) || "";
  const wantLine = line(requested);
  const gotLine = line(topModel);
  if (wantLine && gotLine && wantLine !== gotLine) return true;
  const claimed = canonModel(requested);
  const actual = canonModel(topModel);
  if (claimed && actual && claimed !== actual) {
    // A non-dated version claim is compatible with any dated baseline of the
    // same version; the report still shows which date matched.
    if (claimed === "deepseekv41flash" && actual.startsWith("deepseekv41flash")) return false;
    return true;
  }
  return false;
}

// Rebuilds the total against the current fingerprint rule so reports saved
// before it follow the same scoring.
export function presentedTotal<T extends { key: string; contribution: number; coefficient?: number; applicable?: boolean }>(
  score: number,
  parts: T[] | undefined,
  state: FingerprintState,
): { score: number; parts: T[] } {
  const rows = parts ? parts.map((item) => ({ ...item })) : [];
  const fingerprint = rows.find((item) => item.key === "fingerprint");
  if (!fingerprint?.coefficient || fingerprint.coefficient <= 0) return { score, parts: rows };
  const target = state === "fake" ? 0 : fingerprint.coefficient;
  if (target === fingerprint.contribution) return { score, parts: rows };
  const next = Math.max(0, score - fingerprint.contribution + target);
  fingerprint.contribution = target;
  return { score: next, parts: rows };
}

// ---- Report verdict -------------------------------------------------------
// The total is a weighted average, so one hard failure can hide behind many
// passes (a confirmed swap still scores 80+). The verdict is a separate layer
// over the same data: it never changes the number, it says how far to trust
// it. Pure and derived from the saved verdict, so old reports get it too.

export type VerdictLevel = "ok" | "caution" | "fake" | "unknown";
export type FindingTone = "bad" | "warn" | "note";
export interface Finding {
  tone: FindingTone;
  text: string; // Chinese source string; the caller translates it with vars.
  vars?: Record<string, string | number>;
}
export interface ReportVerdict {
  level: VerdictLevel;
  tone: "ok" | "ink" | "warn" | "bad" | "muted";
  findings: Finding[];
  failed: number;
  scored: number;
  applicable: number;
}

const clip = (text: string, n: number) => (text.length > n ? text.slice(0, n) + "…" : text);
const SIGNATURE_ODD = ["空信封", "半成品", "未识别", "历史"];
const SOFT_DEEP_CHECKS = ["thinking_binding", "context_compact"]; // beta features: unsupported is not dilution

export function reportVerdict(v: DetectVerdict, fp: FingerprintState): ReportVerdict | null {
  const total = v.total;
  if (!total) return null;
  const parts = (total.parts ?? []).filter((p) => p.applicable);
  const weight = parts.reduce((n, p) => n + (p.coefficient || 0), 0);
  const findings: Finding[] = [];
  const add = (tone: FindingTone, text: string, vars?: Finding["vars"]) => findings.push({ tone, text, vars });

  const f = v.fingerprint;
  if (f && !f.error && fp !== "match") {
    const top = f.top_display || f.top_model || f.family_name || "?";
    if (fp === "fake") add("bad", "指纹显示实际是 {model}（{p}%）", { model: top, p: ((f.top_prob ?? 0) * 100).toFixed(1) });
    else add("warn", "指纹更像 {model}，证据不足以判假", { model: top });
  }

  const inj = v.injection;
  if (inj && !inj.error && inj.reliable !== false && typeof inj.score === "number" && inj.score <= 6) {
    add(inj.score <= 2 ? "bad" : "warn", "隐藏注入约 {n} token", { n: Math.round(inj.inj || 0) });
  }

  const t = v.thinking;
  if (t && !t.error && !t.err) {
    if (t.signature_present === false) {
      add("warn", "没有拿到思维链签名");
    } else if (t.signature_present) {
      if (t.original_replay === false) add("warn", "原始签名无法续轮");
      else if (!t.tamper_rejected && t.tamper_status === 200) add("warn", "篡改过的签名被接受，渠道没校验签名");
      if (t.signed_model_match === false) add("warn", "签名里的模型名与声称不一致");
      const odd = SIGNATURE_ODD.find((name) => (t.sig_class || "").startsWith(name));
      if (odd) add("warn", "签名结构异常：{cls}", { cls: odd });
    }
    const checks = (t.strictness ?? []).filter((c) => !c.skipped);
    const missed = checks.filter((c) => !c.pass).length;
    if (checks.length > 0 && missed >= Math.ceil(checks.length / 2)) {
      add("note", "协议校验 {a}/{b}：渠道对非法请求偏宽松", { a: checks.length - missed, b: checks.length });
    }
  }

  for (const c of v.claude_deep?.checks ?? []) {
    if (c.skipped || c.pass) continue;
    add(SOFT_DEEP_CHECKS.includes(c.name) ? "note" : "warn", "{title}：{detail}", {
      title: c.title || c.name,
      detail: clip(c.detail || "", 60),
    });
  }
  if (v.force_cache?.forced === true) add("warn", "没带 cache_control 也被记成缓存读写");

  if (v.tools && !v.tools.error && v.tools.pass === false) add("warn", "工具调用没通过");
  if (v.cache && !v.cache.error && v.cache.pass === false) add("warn", "缓存读写没有衔接");
  // A probe that errored is scored 0 by the backend, which reads as "bad" when
  // it only means "no answer". Count them so the score is not over-trusted.
  const iq = v.iq;
  const questions = (iq?.per_question ?? []).filter((q) => !q.skipped);
  const errored: Record<string, boolean> = {
    fingerprint: !!f?.error,
    injection: !!inj?.error,
    tools: !!v.tools?.error,
    web: !!v.web?.error,
    thinking: !!(t?.error || t?.err),
    vision: !!v.vision?.error,
    cache: !!v.cache?.error,
    deep: !!v.claude_deep?.error,
    forcecache: !!v.force_cache?.error,
    iq: questions.length > 0 && questions.filter((q) => q.error).length * 2 > questions.length,
  };
  if (iq && typeof iq.score === "number" && iq.score <= 60 && !errored.iq) {
    add("warn", "推理表现偏低：{n}/100", { n: iq.score });
  }
  const failedParts = parts.filter((p) => errored[p.key]);
  const failedWeight = failedParts.reduce((n, p) => n + (p.coefficient || 0), 0);
  if (failedParts.length > 0) add("note", "{n} 项请求失败，按 0 分计入", { n: failedParts.length });

  const order: Record<FindingTone, number> = { bad: 0, warn: 1, note: 2 };
  findings.sort((a, b) => order[a.tone] - order[b.tone]);

  const level: VerdictLevel =
    fp === "fake"
      ? "fake"
      : weight > 0 && failedWeight / weight >= 0.4
        ? "unknown"
        : fp === "suspicious" || findings.some((item) => item.tone !== "note")
          ? "caution"
          : "ok";
  const tone: ReportVerdict["tone"] =
    level === "fake"
      ? "bad"
      : level === "unknown"
        ? "muted"
        : total.score < 60
          ? "bad"
          : level === "caution"
            ? "warn"
            : total.score >= 85
              ? "ok"
              : "ink";
  return { level, tone, findings, failed: failedParts.length, scored: parts.length - failedParts.length, applicable: parts.length };
}

// ---- Claude channel summary ----------------------------------------------
// The channel-fingerprint spec ends in one line: fresh signature | zero
// injection | ccmax | AWS-B. These chips are that line, built from the checks
// that already ran; a check that did not run simply has no chip.

export type ChipState = "ok" | "no" | "warn" | "info" | "unsure";
export interface Chip {
  key: string;
  label: string;
  value: string; // Chinese source string
  state: ChipState;
}

export function channelChips(v: DetectVerdict): Chip[] {
  const checks = new Map((v.claude_deep?.checks ?? []).map((c) => [c.name, c] as const));
  const out: Chip[] = [];
  const push = (key: string, label: string, value: string, state: ChipState) => out.push({ key, label, value, state });

  const fresh = checks.get("signature_fresh");
  if (fresh) {
    if (fresh.skipped) push("fresh", "现签", "不定", "unsure");
    else push("fresh", "现签", fresh.pass ? "是" : "复签", fresh.pass ? "ok" : "no");
  }

  const hi = checks.get("hi_usage");
  const inject = checks.get("injection");
  if (hi || inject) {
    const seen = [hi, inject].filter((c): c is NonNullable<typeof c> => c !== undefined);
    if (seen.some((c) => !c.pass && !c.skipped)) push("zero", "0 注入", "否", "no");
    else if (seen.length === 2 && seen.every((c) => c.pass)) push("zero", "0 注入", "是", "ok");
    else push("zero", "0 注入", "不定", "unsure");
  }

  if (inject) {
    const detail = inject.detail || "";
    if (inject.skipped) push("ccmax", "ccmax", "不定", "unsure");
    else if (detail.includes("ccmax")) push("ccmax", "ccmax", "是", "warn");
    else if (detail.includes("Agent SDK")) push("ccmax", "ccmax", "Agent SDK", "warn");
    else if (detail.includes("claude.ai")) push("ccmax", "ccmax", "claude.ai", "warn");
    else push("ccmax", "ccmax", "否", "info");
  }

  const t = v.thinking;
  if (typeof t?.aws_b === "boolean") push("awsb", "AWS-B", t.aws_b ? "是" : "否", "info");
  else if (t?.tamper_status === 200) push("awsb", "AWS-B", "否", "info");

  const pass = checks.get("system_passthrough");
  if (pass) {
    if (pass.skipped) push("system", "system 透传", "不定", "unsure");
    else push("system", "system 透传", pass.pass ? "是" : "否", pass.pass ? "ok" : "no");
  }

  const force = v.force_cache;
  if (force) {
    if (force.forced === true) push("force", "强制缓存", "强制", "no");
    else if (force.forced === false) push("force", "强制缓存", "透传", "ok");
    else push("force", "强制缓存", "不定", "unsure");
  }
  return out;
}
