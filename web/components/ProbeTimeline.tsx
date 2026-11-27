"use client";
import { ReactNode } from "react";
import { Probe } from "@/lib/api";
import Icon from "@/components/Icon";
import ErrorFold from "@/components/ErrorFold";
import ElapsedTime, { DetectionTiming } from "@/components/ElapsedTime";
import { useI18n } from "@/lib/i18n";
export default function ProbeTimeline({
  probes,
  status,
  children,
  timing,
}: {
  probes: Probe[];
  status: string;
  children?: ReactNode;
  timing?: DetectionTiming;
}) {
  const { t: tr } = useI18n();
  const questionProbes = probes.filter((p) => /^Q\d+$/.test(p.id));
  const finished = probes.filter(
    (p) => !["queued", "running"].includes(p.status),
  ).length;
  const active = ["queued", "running"].includes(status);
  const ratio = probes.length ? finished / probes.length : 0;
  const ringR = 19;
  const ringC = 2 * Math.PI * ringR;
  const title =
    status === "queued"
      ? tr("任务已排队")
      : status === "running"
        ? tr("正在收集证据")
        : status === "cancelled"
          ? tr("检测已停止")
          : status === "partial"
            ? tr("报告已生成，部分请求失败")
            : status === "interrupted"
              ? tr("检测已中断")
              : tr("检测报告已生成");
  return (
    <section
      className={
        "timeline " + (active ? "timeline-active" : "timeline-settled")
      }
      data-status={status}
      aria-label={tr("检测进度")}
    >
      <div className="section-heading">
        <div>
          <span className={"timeline-symbol is-round " + (active ? "is-working" : "")}>
            <svg className="timeline-ring" viewBox="0 0 44 44" aria-hidden="true">
              <circle
                className="timeline-ring-track"
                cx="22"
                cy="22"
                r={ringR}
                fill="none"
                strokeWidth="2.5"
              />
              <circle
                className="timeline-ring-fill"
                cx="22"
                cy="22"
                r={ringR}
                fill="none"
                strokeWidth="2.5"
                strokeLinecap="round"
                strokeDasharray={ringC}
                strokeDashoffset={ringC * (1 - ratio)}
                transform="rotate(-90 22 22)"
              />
            </svg>
            <Icon
              name={active ? "spinner" : status === "done" ? "check" : "chart"}
            />
          </span>
          <div>
            <h2>{title}</h2>
            <p className="timeline-caption">
              {timing ? <ElapsedTime timing={timing} active={active} /> : active ? tr("Job 已创建，可以分享。") : ""}
            </p>
          </div>
        </div>
        <span className="progress-count">
          <b>{finished}</b>
          <span> / {probes.length}</span>
        </span>
      </div>
      <div
        className="progress-track is-pill"
        role="progressbar"
        aria-label={tr("已完成探针")}
        aria-valuemin={0}
        aria-valuemax={probes.length}
        aria-valuenow={finished}
      >
        <div style={{ transform: "scaleX(" + ratio + ")" }} />
      </div>
      <div className="probe-grid">
        {probes.map((p, i) => (
          <div key={p.id} className={"probe probe-" + p.status}>
            <span className="probe-index">
              {String(i + 1).padStart(2, "0")}
            </span>
            <span className="probe-name">
              {/^Q\d+$/.test(p.id)
                ? "Q" +
                  (questionProbes.findIndex((q) => q.id === p.id) + 1) +
                  " · "
                : ""}
              {tr(p.name)}
            </span>
            <small className="probe-state">
              <span className="probe-dot is-round" />
              {p.status === "queued"
                ? tr("等待中")
                : p.status === "running"
                  ? tr("检测中")
                  : p.status === "done"
                    ? (p.ms / 1000).toFixed(1) + "s"
                    : p.status === "cancelled"
                      ? tr("已停止")
                      : tr("请求失败")}
            </small>
            {p.error && <ErrorFold text={p.error} compact />}
          </div>
        ))}
      </div>
      <span className="sr-only" role="status">
        {tr("{title}，已完成 {done} 项，共 {total} 项", { title, done: finished, total: probes.length })}
      </span>
      {children}
    </section>
  );
}
