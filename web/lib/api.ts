import { resetSealKey, sealJSON } from "@/lib/seal";
import { visitorID } from "@/lib/visitor";

export const API_BASE = process.env.NEXT_PUBLIC_API_BASE || "";
export interface DetectRequest {
  protocol?: "auto" | "chat" | "anthropic" | "openai";
  base_url: string;
  key: string;
  model: string;
  concurrency?: number;
  timeout?: number;
  cache?: boolean;
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
export interface KnowledgeRow {
  id: string;
  quarter: string;
  happened_at: string;
  event: string;
  ok: boolean;
  unsure: boolean;
  kind?: string;
  answer: string;
  error?: string;
}
export interface KnowledgeVerdict {
  status: string;
  cutoff?: string;
  label?: string;
  correct?: number;
  total?: number;
  web_suspect?: string;
  gap_note?: string;
  raw?: string;
  reason: string;
  protocol: string;
  attempts?: number;
  thinking_used?: boolean;
  rows?: KnowledgeRow[];
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

export interface TotalPart {
  key: string;
  label: string;
  applicable: boolean;
  known: boolean;
  points: number;
  max_points: number;
  weight: number;
  coefficient: number;
  contribution: number;
}
export interface TotalVerdict {
  score: number;
  max: number;
  reason: string;
  parts: TotalPart[];
}
export interface DetectVerdict {
  timing?: { started_at: string; finished_at?: string; total_ms: number; last_run_ms: number; runs: number };
  protocol?: string;
  tools?: CapabilityResult | null;
  web?: CapabilityResult | null;
  knowledge?: KnowledgeVerdict | null;
  integrity?: IntegrityVerdict | null;
  total?: TotalVerdict | null;
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
      cc?: number;
      cr?: number;
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
  hidden: {
    status: "clean" | "suspect" | "injected" | "unknown" | "error";
    injected: boolean;
    score: number;
    max_score: number;
    reason: string;
    protocol: string;
    attempts: number;
    ack_count: number;
    deny_count: number;
    asks: {
      id: string;
      kind: string;
      prompt: string;
      answer: string;
      stance: string;
      error?: string;
    }[] | null;
    identity?: {
      claimed_cutoff?: string;
      expected_cutoff?: string;
      cutoff_mismatch: boolean;
      family?: string;
      self_introduces: boolean;
    } | null;
    signals: string[] | null;
    error?: string;
  } | null;
  fingerprint: {
    basis?: "numeric" | "behavioral";
    family: string;
    family_name: string;
    family_prob: number;
    top_model: string;
    top_display?: string;
    top_prob: number;
    top_similarity?: number;
    sample_min?: number;
    sample_limit?: number;
    confidence_target?: number;
    confidence_reached?: boolean;
    results: {
      model: string;
      display?: string;
      probability: number;
      similarity: number;
    }[];
    behavioral_top3?: {
      modelId: string;
      displayName?: string;
      family?: string;
      similarity: number;
    }[];
    behavioral_total?: number;
    behavior_note?: string;
    used_outputs: number;
    requests?: number;
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
    score?: number;
    summary_score?: number;
    signature_score?: number;
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
  total_score: number | null;
  status?: string;
  family: string | null;
  thinking_pass: boolean | null;
}
const readSnapshots = new Map<string, { etag: string; data: unknown }>();
async function request<T>(
  path: string,
  body?: unknown,
  token?: string,
  signal?: AbortSignal,
  retried = false,
): Promise<T> {
  const publicRead = body === undefined && /^\/api\/(detect\/|reports\/|history(?:\?|\/))/.test(path);
  // Report and history reads carry validators and explicit cache headers.
  // Polling endpoints stay uncached so progress never comes from a stale
  // browser entry.
  const httpCacheable = body === undefined && /^\/api\/(reports\/|history(?:\?|\/))/.test(path);
  // A capability-bearing read is private. Keep its validator separate from
  // the public snapshot so a 304 can never substitute an unmasked payload
  // into an anonymous response (or vice versa).
  const cacheableRead = publicRead;
  const snapshotKey = token ? path + "::authorized" : path;
  const snapshot = cacheableRead ? readSnapshots.get(snapshotKey) : undefined;
  // 访客标识随每次请求发送，服务端据此判断“这个任务是不是这台浏览器提交的”。
  // 它不是凭据：不含密钥，只用来把任务绑定到浏览器。
  const visitor = visitorID();
  const readHeaders: Record<string, string> = {
    ...(token ? { "X-Job-Token": token } : {}),
    ...(visitor ? { "X-Visitor-ID": visitor } : {}),
    ...(snapshot ? { "If-None-Match": snapshot.etag } : {}),
  };
  const res = await fetch(
    API_BASE + path,
    body === undefined
      ? { cache: httpCacheable ? "default" : "no-store", signal, headers: readHeaders }
      : {
          method: "POST",
          redirect: "error",
          headers: {
            "content-type": "application/json",
            ...(token ? { "X-Job-Token": token } : {}),
            ...(visitor ? { "X-Visitor-ID": visitor } : {}),
          },
          body: await sealJSON(API_BASE, body),
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
    if (cacheableRead) readSnapshots.delete(snapshotKey);
    if (!retried && typeof data.error === "string" && data.error.includes("加密密钥已更新")) {
      resetSealKey();
      return request<T>(path, body, token, signal, true);
    }
    throw Error(data.error || "请求失败 HTTP " + res.status);
  }
  const etag = res.headers.get("ETag");
  if (cacheableRead && etag) {
    readSnapshots.delete(snapshotKey);
    readSnapshots.set(snapshotKey, { etag, data });
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
export const getDetect = (id: string, signal?: AbortSignal, token?: string) =>
  request<DetectTask>("/api/detect/" + encodeURIComponent(id), undefined, token, signal);
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
export const getDetection = (id: number, signal?: AbortSignal, token?: string) =>
  request<SavedReport>("/api/history/" + id, undefined, token, signal);
export const getReport = (id: string, signal?: AbortSignal, token?: string) =>
  request<SavedReport>("/api/reports/" + encodeURIComponent(id), undefined, token, signal);
export const getModels = (q: Pick<DetectRequest, "base_url" | "key" | "protocol">) =>
  request<{ models: string[]; upstream_count: number }>("/api/models", {
    base_url: q.base_url,
    key: q.key,
    protocol: q.protocol || "auto",
  });
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
