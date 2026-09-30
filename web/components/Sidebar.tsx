"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useEffect, useState } from "react";
import { getHistory, getBankStats, HistorySummary, BankStats } from "@/lib/api";

function scoreBadge(s: HistorySummary["iq_score"]): { cls: string; text: string } {
  if (s == null) return { cls: "badge-neutral", text: "IQ —" };
  const cls = s >= 80 ? "badge-ok" : s >= 50 ? "badge-neutral" : "badge-bad";
  return { cls, text: `IQ ${s}` };
}
function thinkBadge(p: number | null): { cls: string; text: string } {
  if (p == null) return { cls: "badge-neutral", text: "思考 —" };
  return p === 1 ? { cls: "badge-ok", text: "思考✓" } : { cls: "badge-bad", text: "思考✗" };
}

export default function Sidebar() {
  const [items, setItems] = useState<HistorySummary[]>([]);
  const [stats, setStats] = useState<BankStats | null>(null);

  useEffect(() => {
    getHistory().then(setItems).catch(() => {});
    getBankStats().then(setStats).catch(() => {});
  }, []);

  return (
    <aside className="sidebar">
      <div className="sidebar-logo">掺水？</div>
      <div className="sidebar-sub">
        {stats
          ? `已收录指纹模型 ${stats.fingerprint_count} 个 · ${stats.families.length} 家族`
          : "指纹库加载中…"}
      </div>
      <div className="sidebar-list">
        <div className="sidebar-title">历史检测</div>
        {items.length === 0 && (
          <div style={{ padding: "10px", fontSize: 12, color: "#7a8290" }}>
            还没有检测记录
          </div>
        )}
        {items.map((it) => {
          const host = it.base_url.replace(/^https?:\/\//, "").split("/")[0];
          const sb = scoreBadge(it.iq_score);
          const tb = thinkBadge(it.thinking_pass);
          return (
            <Link key={it.id} href={`/history?id=${it.id}`} className="hist-item">
              <div className="row1">
                <span style={{ fontWeight: 600 }}>{it.model}</span>
                <span>
                  <span className={`hist-badge ${sb.cls}`}>{sb.text}</span>{" "}
                  <span className={`hist-badge ${tb.cls}`}>{tb.text}</span>
                </span>
              </div>
              <div className="row2">
                {host} · {new Date(it.created_at).toLocaleString("zh-CN")}
              </div>
            </Link>
          );
        })}
      </div>
    </aside>
  );
}
