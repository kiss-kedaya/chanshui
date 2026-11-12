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
  const m = model.trim().toLowerCase().replace(/^(openai|anthropic|moonshotai)\//, "");
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
  model = model.trim().toLowerCase().replace(/^(openai|anthropic|moonshotai)\//, "");
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
