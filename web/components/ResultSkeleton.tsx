"use client";
import { useI18n } from "@/lib/i18n";
export default function ResultSkeleton({ label }: { label: string }) {
  const { t } = useI18n();
  return (
    <div className="result-skeleton">
      <span className="skeleton-line skeleton-value" aria-hidden="true" />
      <span className="skeleton-line" aria-hidden="true" />
      <span className="skeleton-line skeleton-short" aria-hidden="true" />
      <p className="muted">{t(label)}</p>
    </div>
  );
}
