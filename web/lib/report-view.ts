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
  const m = model.trim().toLowerCase();
  if (m.startsWith("claude")) return "claude";
  if (m.startsWith("gpt") || /^o[134](?:[.-]|$)/.test(m) || m.includes("chatgpt")) return "gpt";
  if (m.startsWith("gemini")) return "gemini";
  if (m.startsWith("deepseek")) return "deepseek";
  if (m.startsWith("grok")) return "grok";
  if (m.startsWith("glm")) return "glm";
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
  return model.toLowerCase().replace(/[^a-z0-9]+/g, "");
}

export function fingerprintDiluted(requested: string, family: string, topModel = ""): boolean {
  const claimed = canonModel(requested);
  const actual = canonModel(topModel);
  if (claimed && actual && claimed !== actual) return true;
  const want = modelFamily(requested);
  const got = fingerprintFamily(family);
  if (!want || !got || got === "unknown") return false;
  if (want !== got) return true;
  const line = (value: string) => ["opus", "sonnet", "haiku"].find((item) => value.toLowerCase().includes(item)) || "";
  const wantLine = line(requested);
  const gotLine = line(topModel);
  return Boolean(wantLine && gotLine && wantLine !== gotLine);
}

export function presentedTotal<T extends { key: string; contribution: number; applicable?: boolean }>(
  score: number,
  parts: T[] | undefined,
  diluted: boolean,
): { score: number; parts: T[] } {
  const rows = parts ? parts.map((item) => ({ ...item })) : [];
  if (!diluted) return { score, parts: rows };
  const fingerprint = rows.find((item) => item.key === "fingerprint");
  if (!fingerprint || fingerprint.contribution <= 0) return { score, parts: rows };
  const next = Math.max(0, score - fingerprint.contribution);
  fingerprint.contribution = 0;
  return { score: next, parts: rows };
}
