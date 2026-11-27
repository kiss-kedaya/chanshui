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
  conversation?: boolean;
  claude_deep?: boolean;
  force_cache?: boolean;
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
  Effort?: string;
  EffortFallback?: boolean;
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
  skipped?: boolean;
  image?: string;
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
  prefix_sha256?: string;
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
export interface VisionVerdict {
  score: number | null;
  max: number;
  recognized: boolean;
  recognized_score: number;
  recognized_max: number;
  answer_score: number;
  answer_max: number;
  correct: boolean;
  half?: boolean;
  earned?: number;
  question_max?: number;
  skipped?: boolean;
  question?: Question;
  note?: string;
  error?: string;
}
export interface ConversationTurn {
  index: number;
  kind: "opener" | "question" | "smalltalk";
  id?: string;
  title?: string;
  read: number;
  write: number;
  input: number;
  output: number;
  hit_rate: number | null;
  expected?: number;
  dropped: boolean;
  ms: number;
  error?: string;
}
export interface ConversationVerdict {
  rounds: number;
  hits: number;
  drops: number;
  status: "pass" | "mismatch" | "unknown";
  pass: boolean | null;
  reason: string;
  turns: ConversationTurn[];
}
export interface AntiCheatSignal {
  code: string;
  section: string;
  severity: string;
  title: string;
  detail: string;
}
export interface AntiCheatVerdict {
  detected: boolean;
  confirmed_count: number;
  penalized_parts?: string[];
  signals: AntiCheatSignal[];
  summary?: string;
}
export interface DetectVerdict {
  timing?: { started_at: string; finished_at?: string; total_ms: number; last_run_ms: number; runs: number };
  protocol?: string;
  tools?: CapabilityResult | null;
  web?: CapabilityResult | null;
  knowledge?: KnowledgeVerdict | null;
  vision?: VisionVerdict | null;
  conversation?: ConversationVerdict | null;
  integrity?: IntegrityVerdict | null;
  anticheat?: AntiCheatVerdict | null;
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
    mode?: string;
    threshold?: number;
    nominal?: number;
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
      write?: number;
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
  claude_deep?: {
    checks: { name: string; title: string; pass: boolean; skipped?: boolean; status?: number; detail?: string }[];
    note: string;
    error?: string;
  } | null;
  force_cache?: {
    steps: { key: string; label: string; status: number; input: number; write_5m: number; write_1h: number; read: number }[];
    forced?: boolean;
    tier_5m?: boolean;
    tier_1h?: boolean;
    conclusion: string;
    note: string;
    error?: string;
  } | null;
  thinking: {
    pass: boolean;
    score?: number;
    summary_score?: number;
    signature_score?: number;
    replay_score?: number;
    tamper_score?: number;
    temperature_score?: number;
    max_score?: number;
    signature_present?: boolean;
    signed_model?: string;
    signed_model_match?: boolean;
    sig_class?: string;
    sig_field_count?: number;
    sig_infra?: string;
    sig_timestamp?: number;
    sig_age_seconds?: number;
    sig_fresh?: boolean;
    strictness?: { name: string; title?: string; status: number; pass: boolean; skipped?: boolean; detail?: string }[];
    source?: string;
    original_replay?: boolean;
    tamper_rejected?: boolean;
    tamper_status?: number;
    tamper_error?: string;
    self_report?: string;
    family_consistent?: boolean;
    temperature_rejected?: boolean;
    temperature_error?: string;
    aws_b?: boolean;
    aws_b_detail?: string;
    sig_forged?: boolean;
    sig_forge_reason?: string;
    replay_confessed?: boolean;
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
  model: string;
  iq_score: number | null;
  total_score: number | null;
  status?: string;
  family: string | null;
  thinking_pass: boolean | null;
  anticheat_detected?: boolean;
}
const readSnapshots = new Map<string, { etag: string; data: unknown }>();
let pageUnloading = false;
if (typeof window !== "undefined") {
  window.addEventListener("pagehide", () => {
    pageUnloading = true;
  });
  window.addEventListener("pageshow", () => {
    pageUnloading = false;
  });
}

export function isTransientNetworkError(error: unknown): boolean {
  if (!error || typeof error !== "object") return false;
  const err = error as { name?: string; message?: string; transient?: boolean };
  if (err.transient) return true;
  const msg = (err.message || "").toLowerCase();
  return (
    err.name === "TypeError" ||
    msg.includes("networkerror") ||
    msg.includes("failed to fetch") ||
    msg.includes("load failed") ||
    msg.includes("网络连接中断或超时")
  );
}

export function friendlyFetchError(error: unknown): Error {
  if (isTransientNetworkError(error)) {
    const out = new Error("网络连接中断或超时，请稍后重试") as Error & { transient?: boolean };
    out.transient = true;
    return out;
  }
  return error instanceof Error ? error : new Error(String(error));
}

async function fetchOnce(url: string, init: RequestInit, callerSignal?: AbortSignal, timeoutMs = 15000): Promise<Response> {
  if (typeof AbortController !== "function") {
    return fetch(url, init);
  }
  const controller = new AbortController();
  let timedOut = false;
  const onCallerAbort = () => controller.abort();
  if (callerSignal) {
    if (callerSignal.aborted) {
      controller.abort();
    } else {
      callerSignal.addEventListener("abort", onCallerAbort, { once: true });
    }
  }
  const timer = setTimeout(() => {
    timedOut = true;
    controller.abort();
  }, timeoutMs);
  try {
    return await fetch(url, { ...init, signal: controller.signal });
  } catch (error) {
    if (callerSignal?.aborted) throw error;
    if (timedOut) {
      const err = new Error("网络连接中断或超时，请稍后重试") as Error & { transient?: boolean };
      err.transient = true;
      throw err;
    }
    throw error;
  } finally {
    clearTimeout(timer);
    if (callerSignal) callerSignal.removeEventListener("abort", onCallerAbort);
  }
}

async function request<T>(
  path: string,
  body?: unknown,
  token?: string,
  signal?: AbortSignal,
  retried = false,
): Promise<T> {
  const isGet = body === undefined;
  const publicRead = isGet && /^\/api\/(detect\/|reports\/|history(?:\?|\/|$)|bank\/stats)/.test(path);
  // Report, history and stats reads carry validators and explicit cache headers.
  // Polling endpoints stay uncached so progress never comes from a stale browser entry.
  const httpCacheable = isGet && /^\/api\/(reports\/|history(?:\?|\/|$)|bank\/stats)/.test(path);
  const cacheableRead = publicRead;
  const snapshotKey = token ? path + "::authorized" : path;
  const snapshot = cacheableRead ? readSnapshots.get(snapshotKey) : undefined;
  const visitor = visitorID();
  const readHeaders: Record<string, string> = {
    ...(token ? { "X-Job-Token": token } : {}),
    ...(visitor ? { "X-Visitor-ID": visitor } : {}),
    ...(snapshot ? { "If-None-Match": snapshot.etag } : {}),
  };
  let init: RequestInit;
  if (isGet) {
    init = { cache: httpCacheable ? "default" : "no-store", headers: readHeaders };
  } else {
    let sealedBody: string;
    try {
      sealedBody = await sealJSON(API_BASE, body);
    } catch (error) {
      throw friendlyFetchError(error);
    }
    init = {
      method: "POST",
      redirect: "error",
      headers: {
        "content-type": "application/json",
        ...(token ? { "X-Job-Token": token } : {}),
        ...(visitor ? { "X-Visitor-ID": visitor } : {}),
      },
      body: sealedBody,
    };
  }
  const retryDelays = isGet ? [0, 350, 900] : [0];
  let res: Response | null = null;
  let lastError: unknown;
  for (let attempt = 0; attempt < retryDelays.length; attempt++) {
    if (retryDelays[attempt] > 0) {
      await new Promise((resolve) => setTimeout(resolve, retryDelays[attempt]));
      if (signal?.aborted) throw new DOMException("Aborted", "AbortError");
    }
    try {
      res = await fetchOnce(API_BASE + path, init, signal, isGet ? 15000 : 30000);
      break;
    } catch (error) {
      if (signal?.aborted || pageUnloading) {
        const abortErr = new Error("Aborted");
        abortErr.name = "AbortError";
        throw abortErr;
      }
      lastError = error;
      if (!isGet || !isTransientNetworkError(error) || attempt === retryDelays.length - 1) {
        throw friendlyFetchError(error);
      }
    }
  }
  if (!res) throw friendlyFetchError(lastError);
  if (res.status === 304 && snapshot) return snapshot.data as T;
  let data;
  try {
    data = await res.json();
  } catch {
    throw Error("服务返回了非 JSON 响应，请检查网络或稍后重试");
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
    if (readSnapshots.size > 12) readSnapshots.delete(readSnapshots.keys().next().value!);
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
export const getBankStats = (signal?: AbortSignal) =>
  request<BankStats>("/api/bank/stats", undefined, undefined, signal);
export const getHistory = (limit = 10, before?: number, signal?: AbortSignal) =>
  request<HistorySummary[]>(
    "/api/history?limit=" + limit + (before ? "&before=" + before : ""),
    undefined,
    undefined,
    signal,
  );
export interface SavedReport {
  status: string;
  expires_at: number;
  probes: Probe[];
  id: number;
  report_id: string;
  created_at: string;
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
