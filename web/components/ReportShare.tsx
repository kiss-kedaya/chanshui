"use client";
import { useState } from "react";
import Icon from "@/components/Icon";
import Link from "next/link";
import { reportPath } from "@/lib/report-view";
export default function ReportShare({
  id,
  expiresAt,
}: {
  id: string;
  expiresAt?: number;
}) {
  const [message, setMessage] = useState("");
  const [manual, setManual] = useState("");
  async function copy(link: boolean) {
    const text = link
      ? new URL(reportPath(id), window.location.origin).href
      : id;
    try {
      await navigator.clipboard.writeText(text);
      setManual("");
      setMessage(link ? "报告链接已复制" : "检测 ID 已复制");
    } catch {
      setManual(text);
      setMessage("剪贴板不可用，可在下方选中复制");
    }
  }
  return (
    <div className="report-share">
      <div className="report-identity">
        <span>Job ID</span>
        <code>{id}</code>
        {expiresAt && (
          <small className="muted">
            有效至 {new Date(expiresAt * 1000).toLocaleString("zh-CN")}
          </small>
        )}
      </div>
      <div className="report-share-buttons">
        <button
          className="btn-secondary"
          aria-label="复制 ID"
          title="复制 ID"
          onClick={() => copy(false)}
        >
          <Icon name="copy" width="16" height="16" />
        </button>
        <button
          className="btn-secondary"
          aria-label="复制报告链接"
          title="复制报告链接"
          onClick={() => copy(true)}
        >
          <Icon name="link" width="16" height="16" />
        </button>
        <Link
          className="btn-secondary"
          aria-label="打开独立报告"
          title="打开独立报告"
          href={reportPath(id)}
        >
          <Icon name="arrow" width="16" height="16" />
        </Link>
      </div>
      {message && (
        <span className="muted" role="status">
          {message}
        </span>
      )}
      {manual && (
        <input
          aria-label="手动复制内容"
          readOnly
          value={manual}
          onFocus={(e) => e.target.select()}
        />
      )}
    </div>
  );
}
