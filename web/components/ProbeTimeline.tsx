"use client";
import { ReactNode } from "react";
import { Probe } from "@/lib/api";
import Icon from "@/components/Icon";
export default function ProbeTimeline({
  probes,
  status,
  children,
}: {
  probes: Probe[];
  status: string;
  children?: ReactNode;
}) {
  const questionProbes = probes.filter((p) => /^Q\d+$/.test(p.id));
  const finished = probes.filter(
    (p) => !["queued", "running"].includes(p.status),
  ).length;
  const active = ["queued", "running"].includes(status);
  const ratio = probes.length ? finished / probes.length : 0;
  const title =
    status === "queued"
      ? "任务已排队"
      : status === "running"
        ? "正在收集证据"
        : status === "cancelled"
          ? "检测已停止"
          : status === "partial"
            ? "报告已生成，部分请求失败"
            : status === "interrupted"
              ? "检测已中断"
              : "检测报告已生成";
  return (
    <section
      className={
        "timeline " + (active ? "timeline-active" : "timeline-settled")
      }
      aria-label="检测进度"
    >
      <div className="section-heading">
        <div>
          <span className={"timeline-symbol " + (active ? "is-working" : "")}>
            <Icon
              name={active ? "spinner" : status === "done" ? "check" : "chart"}
            />
          </span>
          <div>
            <h2>{title}</h2>
            <p className="timeline-caption">
              {active ? "Job 已创建，可以分享。" : ""}
            </p>
          </div>
        </div>
        <span className="progress-count">
          <b>{finished}</b>
          <span> / {probes.length}</span>
        </span>
      </div>
      <div
        className="progress-track"
        role="progressbar"
        aria-label="已完成探针"
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
              {p.name}
            </span>
            <small className="probe-state">
              <span className="probe-dot" />
              {p.status === "queued"
                ? "等待中"
                : p.status === "running"
                  ? "检测中"
                  : p.status === "done"
                    ? (p.ms / 1000).toFixed(1) + "s"
                    : p.status === "cancelled"
                      ? "已停止"
                      : "请求失败"}
            </small>
            {p.error && <p>{p.error}</p>}
          </div>
        ))}
      </div>
      <span className="sr-only" role="status">
        {title}，已完成 {finished} 项，共 {probes.length} 项
      </span>
      {children}
    </section>
  );
}
