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
