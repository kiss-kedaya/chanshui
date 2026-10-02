"use client";
import { useState } from "react";
import { useI18n } from "@/lib/i18n";

export function splitError(text: string) {
  const index = text.indexOf("\n");
  if (index < 0) return { summary: text, detail: "" };
  return { summary: text.slice(0, index), detail: text.slice(index + 1).trim() };
}

export default function ErrorFold({ text, compact = false }: { text: string; compact?: boolean }) {
  const { t: tr } = useI18n();
  const { summary, detail } = splitError(text);
  const [open, setOpen] = useState(false);
  if (!detail) {
    return (
      <div className={compact ? "error-box error-compact" : "error-box"} role="alert">
        {tr(summary)}
      </div>
    );
  }
  return (
    <div className={"error-box error-fold" + (compact ? " error-compact" : "") + (open ? " is-open" : "")} role="alert">
      <button
        type="button"
        className="error-toggle"
        aria-expanded={open}
        onClick={() => setOpen((value) => !value)}
      >
        <span className="error-mark" aria-hidden="true">
          <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
            <path className="error-ring" d="M12 3 2.8 20h18.4L12 3Z" />
            <path d="M12 9v5" />
            <path d="M12 17h.01" />
          </svg>
        </span>
        <span className="error-summary">{tr(summary)}</span>
        <svg className="error-chevron" viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" strokeWidth="1.8" aria-hidden="true">
          <path d="m8 10 4 4 4-4" />
        </svg>
      </button>
      {open && <p className="error-detail">{tr(detail)}</p>}
    </div>
  );
}
