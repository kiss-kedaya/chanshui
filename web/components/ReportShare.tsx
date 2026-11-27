"use client";
import { useEffect, useRef, useState } from "react";
import Icon from "@/components/Icon";
import Link from "next/link";
import { reportPath } from "@/lib/report-view";
import { formatLocaleDate, useI18n } from "@/lib/i18n";
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
  const [copiedKey, setCopiedKey] = useState<"id" | "link" | null>(null);
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    return () => {
      if (timerRef.current) clearTimeout(timerRef.current);
    };
  }, []);

  async function copy(link: boolean) {
    const text = link
      ? new URL(reportPath(id), window.location.origin).href
      : id;
    try {
      await navigator.clipboard.writeText(text);
      setManual("");
      setMessage(link ? tr("报告链接已复制") : tr("检测 ID 已复制"));
      setCopiedKey(link ? "link" : "id");
      if (timerRef.current) clearTimeout(timerRef.current);
      timerRef.current = setTimeout(() => setCopiedKey(null), 1800);
    } catch {
      setManual(text);
      setCopiedKey(null);
      setMessage(tr("剪贴板不可用，可在下方选中复制"));
    }
  }
  return (
    <div className="report-share">
      <div className="report-identity">
        <span>Job ID</span>
        <code>{id}</code>
        {expiresAt != null && expiresAt > 0 && (
          <small className="muted">
            {tr("有效至 {time}", {
              time: formatLocaleDate(expiresAt * 1000, locale, {
                month: "2-digit",
                day: "2-digit",
                hour: "2-digit",
                minute: "2-digit",
              }),
            })}
          </small>
        )}
      </div>
      <div className="report-share-buttons">
        <button
          type="button"
          className="btn-secondary"
          aria-label={tr("复制 ID")}
          title={tr("复制 ID")}
          onClick={() => copy(false)}
        >
          <Icon name={copiedKey === "id" ? "check" : "copy"} width="15" height="15" />
          <span>{tr("复制 ID")}</span>
        </button>
        <button
          type="button"
          className="btn-secondary"
          aria-label={tr("复制报告链接")}
          title={tr("复制报告链接")}
          onClick={() => copy(true)}
        >
          <Icon name={copiedKey === "link" ? "check" : "link"} width="15" height="15" />
          <span>{tr("复制链接")}</span>
        </button>
        <Link
          className="btn-secondary"
          aria-label={tr("打开独立报告")}
          title={tr("打开独立报告")}
          href={reportPath(id)}
        >
          <Icon name="arrow" width="15" height="15" />
          <span>{tr("独立报告")}</span>
        </Link>
      </div>
      <p
        className="report-share-status"
        data-visible={message ? "true" : "false"}
        role="status"
        aria-live="polite"
      >
        {message || "\u00A0"}
      </p>
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
