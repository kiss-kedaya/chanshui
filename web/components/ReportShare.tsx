"use client";
import { useState } from "react";
import Icon from "@/components/Icon";
import Link from "next/link";
import { reportPath } from "@/lib/report-view";
import { useI18n } from "@/lib/i18n";
export default function ReportShare({
  id,
  expiresAt,
}: {
  id: string;
  expiresAt?: number;
}) {
  const { t: tr, locale } = useI18n();
  const [message, setMessage] = useState("");
  const [manual, setManual] = useState("");
  async function copy(link: boolean) {
    const text = link
      ? new URL(reportPath(id), window.location.origin).href
      : id;
    try {
      await navigator.clipboard.writeText(text);
      setManual("");
      setMessage(link ? tr("报告链接已复制") : tr("检测 ID 已复制"));
    } catch {
      setManual(text);
      setMessage(tr("剪贴板不可用，可在下方选中复制"));
    }
  }
  return (
    <div className="report-share">
      <div className="report-identity">
        <span>Job ID</span>
        <code>{id}</code>
        {expiresAt && (
          <small className="muted">
            {tr("有效至 {time}", { time: new Date(expiresAt * 1000).toLocaleString(locale) })}
          </small>
        )}
      </div>
      <div className="report-share-buttons">
        <button
          className="btn-secondary"
          aria-label={tr("复制 ID")}
          title={tr("复制 ID")}
          onClick={() => copy(false)}
        >
          <Icon name="copy" width="16" height="16" />
        </button>
        <button
          className="btn-secondary"
          aria-label={tr("复制报告链接")}
          title={tr("复制报告链接")}
          onClick={() => copy(true)}
        >
          <Icon name="link" width="16" height="16" />
        </button>
        <Link
          className="btn-secondary"
          aria-label={tr("打开独立报告")}
          title={tr("打开独立报告")}
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
          aria-label={tr("手动复制内容")}
          readOnly
          value={manual}
          onFocus={(e) => e.target.select()}
        />
      )}
    </div>
  );
}
