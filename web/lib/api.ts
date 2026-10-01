export const API_BASE = process.env.NEXT_PUBLIC_API_BASE || "";
export interface DetectRequest {
  protocol?: "auto" | "chat" | "anthropic" | "openai";
  base_url: string;
  key: string;
  model: string;
  concurrency?: number;
  timeout?: number;
}
export interface Metrics {
  Status: number;
  Ms: number;
  Err: string;
  Answer: string;
  ThinkingText: string;
  Signature: string;
  InputTokens: number;
  OutputTokens: number;
  ThinkingTokens: number;
  CacheCreation5m: number;
  CacheCreation1h: number;
  CacheRead: number;
  StopReason: string;
}
export interface Question {
  id: string;
  title: string;
  max: number;
  earned: number;
  half: boolean;
  tags: string[];
  prompt: string;
  expected: string;
  answer: string;
  thinking_text: string;
  signature: string;
  ms: number;
  error?: string;
  metrics: Metrics;
}
export interface CapabilityResult {
  status: string;
  pass: boolean | null;
  reason: string;
  protocol: string;
  evidence?: unknown;
  attempts?: number;
  error?: string;
}
export interface CacheSample {
  index: number;
  read: number;
  write: number;
  input: number;
  output: number;
  total: number;
  hit_rate: number | null;
  attempts: number;
  read_reported?: boolean;
  write_reported?: boolean;
  input_reported?: boolean;
  output_reported?: boolean;
  prompt_chars?: number;
  messages?: number;
  request_sha256?: string;
  error?: string;
}
export interface KnowledgeVerdict {
  status: string;
  cutoff?: string;
  raw?: string;
  reason: string;
  protocol: string;
  attempts?: number;
  thinking_used?: boolean;
  error?: string;
}
export interface IntegrityVerdict {
  score: number | null;
  coverage: number;
  available_weight: number;
  applicable_weight: number;
  status: string;
  reason: string;
  signals: { key: string; label: string; weight: number; applicable: boolean; known: boolean; points?: number; max_points: number; reason: string }[];
}
export interface DetectVerdict {
  timing?: { started_at: string; finished_at?: string; total_ms: number; last_run_ms: number; runs: number };
  protocol?: string;
  tools?: CapabilityResult | null;
  web?: CapabilityResult | null;
  knowledge?: KnowledgeVerdict | null;
  integrity?: IntegrityVerdict | null;
  cache?: {
    samples?: CacheSample[];
    transitions?: {
      from: number;
      to: number;
      expected_read: number;
      actual_read: number;
      delta: number;
      pass: boolean | null;
    }[];
    third_read_ratio?: number | null;
    third_read_baseline?: number;
    total_read?: number;
    total_output?: number;
    prefix_sha256?: string;
    retries_observed?: boolean;
    status: string;
    pass: boolean | null;
    reason: string;
    first: { read: number; write: number; input: number } | null;
    second: { read: number; write: number; input: number } | null;
    expected_read: number;
    actual_read: number;
    delta: number;
    protocol: string;
    error?: string;
  } | null;
  injection: {
    inj: number;
    slope: number;
    round_overhead?: number;
    r2: number | null;
    max_residual: number | null;
    reliable: boolean;
    score: number | null;
    max_score: number;
    note: string;
    points: {
      round?: number;
      output_before?: number;
      adjusted_total?: number;
      n: number;
      input: number;
      cc: number;
      cr: number;
      total: number;
    }[];
    cache_header: string;
    probe?: {
      sse_ok: boolean;
      sse_frames: number;
      stream_error?: string;
    } | null;
    error?: string;
  } | null;
  fingerprint: {
    family: string;
    family_name: string;
    family_prob: number;
    top_model: string;
    top_prob: number;
    results: { model: string; probability: number; similarity: number }[];
    behavioral_top3?: { modelId: string; similarity: number }[];
    behavior_note?: string;
    used_outputs: number;
    low_confidence: boolean;
    note: string;
    evidence: {
      id: string;
      expected_count: number;
      parsed_count: number;
      answer: string;
      error: string;
      ms: number;
    }[];
    error?: string;
  } | null;
  thinking: {
    pass: boolean;
    think_len: number;
    sig_len: number;
    reason: string;
    thinking_text: string;
    signature: string;
    answer: string;
    error?: string;
    err?: string;
  } | null;
  iq: {
    score: number | null;
    max: number;
    paper_max: number;
    complete: boolean;
    per_question: Question[];
    note: string;
    error?: string;
  } | null;
}
export interface Probe {
  id: string;
  name: string;
  status: string;
  ms: number;
  error?: string;
}
export interface DetectTask {
  id: string;
  status: string;
  base_url: string;
  model: string;
  created_at: string;
  probes: Probe[];
  verdict: DetectVerdict;
  history_id?: number;
  report_id?: string;
  warning?: string;
  expires_at: number;
}
export interface BankStats {
  supported_models: string[];
  fingerprint_count: number;
  numeric_count: number;
  behavioral_count: number;
  families: string[];
  numeric_families: string[];
  source?: { commit: string; synced_at: string; built_at: string };
}
export interface HistorySummary {
  report_id: string;
  id: number;
  created_at: string;
  base_url: string;
  model: string;
  iq_score: number | null;
  family: string | null;
  thinking_pass: boolean | null;
}
const readSnapshots = new Map<string, { etag: string; data: unknown }>();
async function request<T>(
  path: string,
  body?: unknown,
  token?: string,
  signal?: AbortSignal,
): Promise<T> {
  const publicRead = body === undefined && /^\/api\/(detect\/|reports\/|history(?:\?|\/))/.test(path);
  const snapshot = publicRead ? readSnapshots.get(path) : undefined;
  const res = await fetch(
    API_BASE + path,
    body === undefined
      ? { cache: "no-store", signal, headers: snapshot ? { "If-None-Match": snapshot.etag } : undefined }
      : {
          method: "POST",
          redirect: "error",
          headers: {
            "content-type": "application/json",
            ...(token ? { "X-Job-Token": token } : {}),
          },
          body: JSON.stringify(body),
          signal,
        },
  );
  if (res.status === 304 && snapshot) return snapshot.data as T;
  let data;
  try {
    data = await res.json();
  } catch {
    throw Error("服务返回了非 JSON 响应，请检查本地服务");
  }
  if (!res.ok) {
    if (publicRead) readSnapshots.delete(path);
    throw Error(data.error || "请求失败 HTTP " + res.status);
  }
  const etag = res.headers.get("ETag");
  if (publicRead && etag) {
    readSnapshots.delete(path);
    readSnapshots.set(path, { etag, data });
    if (readSnapshots.size > 8) readSnapshots.delete(readSnapshots.keys().next().value!);
  }
  return data as T;
}
export const startDetect = (q: DetectRequest) =>
  request<{
    task_id: string;
    report_id: string;
    control_token: string;
    expires_at: number;
  }>("/api/detect", q);
export const getDetect = (id: string, signal?: AbortSignal) =>
  request<DetectTask>("/api/detect/" + encodeURIComponent(id), undefined, undefined, signal);
export const retryDetect = (id: string, q: DetectRequest, token: string) =>
  request<{ task_id: string }>("/api/detect/" + encodeURIComponent(id) + "/retry", q, token);
export const getBankStats = () => request<BankStats>("/api/bank/stats");
export const getHistory = (limit = 50, before?: number) =>
  request<HistorySummary[]>(
    "/api/history?limit=" + limit + (before ? "&before=" + before : ""),
  );
export interface SavedReport {
  status: string;
  expires_at: number;
  probes: Probe[];
  id: number;
  report_id: string;
  host: string;
  created_at: string;
  base_url: string;
  model: string;
  verdict_json: DetectVerdict;
}
export const getDetection = (id: number, signal?: AbortSignal) =>
  request<SavedReport>("/api/history/" + id, undefined, undefined, signal);
export const getReport = (id: string, signal?: AbortSignal) =>
  request<SavedReport>("/api/reports/" + encodeURIComponent(id), undefined, undefined, signal);
export const getModels = (q: DetectRequest) =>
  request<{ models: string[]; upstream_count: number }>("/api/models", q);
export const ask = (
  q: DetectRequest,
  prompt: string,
  thinking: unknown,
  effort: string,
) =>
  request<Metrics>("/api/ask", {
    ...q,
    prompt,
    thinking,
    ...(effort ? { effort } : {}),
  });

export const submitFeedback = (data: {
  kind: string;
  contact: string;
  message: string;
}) => request<{ id: string; message: string }>("/api/feedback", data);
