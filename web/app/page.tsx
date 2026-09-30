"use client";

import { useCallback, useEffect, useState } from "react";
import {
  startDetect,
  getDetect,
  getBankStats,
  DetectTask,
  BankStats,
  DetectVerdict,
} from "@/lib/api";
import ResultCards from "@/components/ResultCards";

const FAMILIES: Record<string, string> = {
  gpt: "#10a37f",
  openai: "#10a37f",
  claude: "#d97757",
  anthropic: "#d97757",
  google: "#4285f4",
  deepseek: "#4d6bfe",
  xai: "#111827",
  zhipu: "#4b6bfb",
};

export default function DetectPage() {
  const [baseUrl, setBaseUrl] = useState("");
  const [key, setKey] = useState("");
  const [model, setModel] = useState("");
  const [stats, setStats] = useState<BankStats | null>(null);
  const [task, setTask] = useState<DetectTask | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    getBankStats().then(setStats).catch(() => {});
  }, []);

  const run = useCallback(async () => {
    setError("");
    setBusy(true);
    setTask(null);
    try {
      const { task_id } = await startDetect({
        base_url: baseUrl.trim(),
        key: key.trim(),
        model: model.trim(),
      });
      // 轮询
      for (;;) {
        const t = await getDetect(task_id);
        setTask(t);
        if (t.status !== "running") break;
        await new Promise((r) => setTimeout(r, 1200));
      }
    } catch (e: any) {
      setError(e?.message || String(e));
    } finally {
      setBusy(false);
    }
  }, [baseUrl, key, model]);

  const canRun = baseUrl.trim() && key.trim() && model.trim() && !busy;

  return (
    <>
      <div className="hero-card">
        <h1 className="hero-title">一键检测</h1>
        <p className="hero-sub">
          填入网关地址和 Key，后端自动完成四项鉴定：注入 token · 模型家族 · 思考文本回传 · 智商评分。
          Key 只在本次检测使用，不落盘。
        </p>
        {stats && (
          <div className="statline">
            <span>
              已收录指纹模型 <b>{stats.fingerprint_count}</b> 个
            </span>
            <span>
              支持家族 <b>{stats.families.join(" / ")}</b>
            </span>
          </div>
        )}
        <div style={{ display: "grid", gap: 12, marginBottom: 16 }}>
          <div className="field">
            <label>API 接口地址</label>
            <input
              placeholder="网关地址"
              value={baseUrl}
              onChange={(e) => setBaseUrl(e.target.value)}
              spellCheck={false}
            />
          </div>
          <div className="field">
            <label>API Key</label>
            <input
              type="password"
              placeholder="sk-..."
              value={key}
              onChange={(e) => setKey(e.target.value)}
              spellCheck={false}
            />
          </div>
          <div className="field">
            <label>目标模型</label>
            <input
              placeholder="模型名"
              value={model}
              onChange={(e) => setModel(e.target.value)}
              spellCheck={false}
              list="model-suggestions"
            />
            <datalist id="model-suggestions">
              {(stats?.supported_models || []).map((m) => (
                <option key={m} value={m} />
              ))}
            </datalist>
          </div>
        </div>
        <button className="btn-primary" onClick={run} disabled={!canRun}>
          {busy ? "检测中…" : "开始检测"}
        </button>
        {error && (
          <p style={{ color: "var(--bad)", fontSize: 13, marginTop: 10 }}>{error}</p>
        )}
      </div>

      {task && task.status === "running" && (
        <div className="timeline">
          <div className="tl-line tl-wait">后端探测中：注入测量 · 指纹归因 · 思考检查 · IQ 测试 并行执行…</div>
        </div>
      )}

      {task && task.status !== "running" && task.verdict && (
        <ResultCards verdict={task.verdict} />
      )}
    </>
  );
}
