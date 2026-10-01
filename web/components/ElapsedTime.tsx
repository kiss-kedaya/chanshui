"use client";
import { useEffect, useState } from "react";

export interface DetectionTiming {
  started_at: string;
  finished_at?: string;
  total_ms: number;
  last_run_ms: number;
  runs: number;
}

export default function ElapsedTime({ timing, active }: { timing: DetectionTiming; active: boolean }) {
  const [now, setNow] = useState(() => Date.now());
  const ticking = active && !timing.finished_at;
  useEffect(() => {
    if (!ticking) return;
    setNow(Date.now());
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, [ticking, timing.started_at]);
  const extra = ticking ? Math.max(0, now - Date.parse(timing.started_at)) : 0;
  const seconds = Math.max(0, Math.floor((timing.total_ms + extra) / 1000));
  const value = seconds < 60 ? `${seconds} 秒` : `${Math.floor(seconds / 60)} 分 ${seconds % 60} 秒`;
  return <span className="muted" title="各轮检测实际耗时累计，包含自动重试；不累计各并发项，也不包含手动重试前的等待时间。">检测总耗时 <b>{value}</b></span>;
}
