export const API_BASE =
  process.env.NEXT_PUBLIC_API_BASE || "http://127.0.0.1:8770";

export interface DetectRequest {
  base_url: string;
  key: string;
  model: string;
}

export interface DetectVerdict {
  injection: {
    inj: number;
    slope: number;
    points: { n: number; input: number; cc: number; cr: number; total: number }[];
    cache_header: string;
    error?: string;
  } | null;
  fingerprint: {
    family: string;
    family_name: string;
    family_prob: number;
    top_model: string;
    top_prob: number;
    results: { model: string; probability: number }[];
    behavioral_top3?: { model_id: string; similarity: number }[];
    used_outputs: number;
    error?: string;
  } | null;
  thinking: {
    pass: boolean;
    think_len: number;
    sig_len: number;
    reason: string;
    error?: string;
  } | null;
  iq: {
    score: number;
    max: number;
    per_question: {
      id: string;
      title: string;
      max: number;
      earned: number;
      half: boolean;
      tags: string[];
    }[];
    error?: string;
  } | null;
}

export interface DetectTask {
  id: string;
  status: "running" | "done" | "error";
  base_url: string;
  model: string;
  created_at: string;
  verdict?: DetectVerdict;
  iq_score?: number;
}

export async function startDetect(req: DetectRequest): Promise<{ task_id: string }> {
  const res = await fetch(`${API_BASE}/api/detect`, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify(req),
  });
  if (!res.ok) throw new Error(`检测任务启动失败: HTTP ${res.status}`);
  return res.json();
}

export async function getDetect(id: string): Promise<DetectTask> {
  const res = await fetch(`${API_BASE}/api/detect/${id}`);
  if (!res.ok) throw new Error(`查询失败: HTTP ${res.status}`);
  return res.json();
}

export interface BankStats {
  supported_models: string[];
  fingerprint_count: number;
  families: string[];
}

export async function getBankStats(): Promise<BankStats> {
  const res = await fetch(`${API_BASE}/api/bank/stats`);
  if (!res.ok) throw new Error("bank stats 加载失败");
  return res.json();
}

export interface HistorySummary {
  id: number;
  created_at: string;
  base_url: string;
  model: string;
  iq_score: number | null;
  family: string | null;
  thinking_pass: number | null;
}

export async function getHistory(limit = 50): Promise<HistorySummary[]> {
  const res = await fetch(`${API_BASE}/api/history?limit=${limit}`);
  if (!res.ok) throw new Error("历史加载失败");
  return res.json();
}

export async function getDetection(id: number): Promise<{
  id: number;
  created_at: string;
  base_url: string;
  model: string;
  verdict_json: DetectVerdict;
}> {
  const res = await fetch(`${API_BASE}/api/history/${id}`);
  if (!res.ok) throw new Error("详情加载失败");
  return res.json();
}
