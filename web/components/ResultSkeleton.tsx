export default function ResultSkeleton({ label }: { label: string }) {
  return (
    <div className="result-skeleton">
      <span className="skeleton-line skeleton-value" aria-hidden="true" />
      <span className="skeleton-line" aria-hidden="true" />
      <span className="skeleton-line skeleton-short" aria-hidden="true" />
      <p className="muted">{label}</p>
    </div>
  );
}
