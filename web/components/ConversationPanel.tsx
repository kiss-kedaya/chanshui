"use client";
import { useCallback, useEffect, useRef, useState } from "react";
import { ConversationTurn, ConversationVerdict } from "@/lib/api";
import Icon from "@/components/Icon";
import { useI18n } from "@/lib/i18n";

export function trConvReason(
  reason: string,
  tr: (source: string, vars?: Record<string, string | number>) => string,
): string {
  if (!reason) return "";
  const miss = reason.match(/^(\d+) 轮未接上前缀，疑似中途换号。$/);
  if (miss) return tr("{n} 轮未接上前缀，疑似中途换号。", { n: miss[1] });
  const hit = reason.match(/^(\d+) 轮接上前缀。$/);
  if (hit) return tr("{n} 轮接上前缀。", { n: hit[1] });
  return tr(reason);
}

// The serial-conversation evidence: one row per request so a cache read that
// falls back to zero mid-thread is visible, not buried in a score.
export default function ConversationPanel({
  data,
}: {
  data?: ConversationVerdict | null;
}) {
  const { t: tr } = useI18n();
  const turns = data?.turns ?? [];
  const firstBreak = turns.find((turn) => turn.dropped)?.index ?? null;
  const [selected, setSelected] = useState<number | null>(firstBreak);
  const userToggledRef = useRef(false);
  const wrapRef = useRef<HTMLDivElement>(null);
  const [scrollState, setScrollState] = useState<"none" | "start" | "mid" | "end">("none");

  useEffect(() => {
    if (!userToggledRef.current && firstBreak !== null) {
      setSelected((cur) => (cur === null ? firstBreak : cur));
    }
  }, [firstBreak]);

  const updateScroll = useCallback(() => {
    const el = wrapRef.current;
    if (!el) return;
    const max = el.scrollWidth - el.clientWidth;
    if (max <= 4) {
      setScrollState("none");
      return;
    }
    if (el.scrollLeft <= 4) setScrollState("start");
    else if (el.scrollLeft >= max - 4) setScrollState("end");
    else setScrollState("mid");
  }, []);

  useEffect(() => {
    updateScroll();
    const el = wrapRef.current;
    if (!el || typeof ResizeObserver === "undefined") return;
    const ro = new ResizeObserver(updateScroll);
    ro.observe(el);
    return () => ro.disconnect();
  }, [updateScroll, turns.length]);

  if (!data || !data.rounds) return null;
  const state =
    data.status === "pass"
      ? tr("缓存连续")
      : data.status === "mismatch"
        ? tr("中途掉缓存")
        : tr("无法判断");
  const rate = (value: number | null) =>
    value == null ? "—" : value.toFixed(1) + "%";
  const label = (turn: ConversationTurn) =>
    turn.kind === "question"
      ? (turn.id || "") + " · " + tr(turn.title || "")
      : turn.kind === "opener"
        ? tr("开场白")
        : tr("闲聊");
  const activeTurn = selected == null ? null : turns.find((turn) => turn.index === selected) ?? null;

  return (
    <div className="conv-block">
      <div className="conv-strip">
        <span className="conv-tag">{tr("多轮对话")}</span>
        <span>
          {data.rounds} {tr("轮")}
        </span>
        <span>
          {tr("缓存命中")} {data.hits}/{data.rounds}
        </span>
        <b className={data.drops ? "bad" : data.status === "pass" ? "good" : "warn"}>
          {data.drops ? tr("未接续") + " " + data.drops : state}
        </b>
      </div>
      <p className="muted">{trConvReason(data.reason, tr)}</p>
      <div className="conv-chain" role="group" aria-label={tr("多轮对话缓存链")}>
        {turns.map((turn) => {
          const isSelected = activeTurn?.index === turn.index;
          const summary = `#${turn.index} ${label(turn)} · ${tr("读")} ${turn.read} / ${tr("应读")} ${turn.expected ?? "—"} · ${tr("写")} ${turn.write}`;
          return (
            <button
              key={turn.index}
              type="button"
              className={
                "conv-chain-seg is-round " +
                (turn.dropped ? "is-break" : turn.expected ? "is-ok" : "is-base") +
                (isSelected ? " is-selected" : "")
              }
              aria-pressed={isSelected}
              aria-label={summary}
              title={summary}
              onClick={() => {
                userToggledRef.current = true;
                setSelected((cur) => (cur === turn.index ? null : turn.index));
              }}
            >
              <i aria-hidden="true" />
            </button>
          );
        })}
      </div>
      <div className="conv-legend">
        <span><i className="is-ok" />{tr("接上")}</span>
        <span><i className="is-break" />{tr("未接续")}</span>
        <span><i className="is-base" />{tr("基线")}</span>
      </div>
      {activeTurn && (
        <div className="conv-turn-detail" role="status">
          <div className="conv-turn-detail-head">
            <strong>
              #{activeTurn.index} · {label(activeTurn)}
            </strong>
            <div className="conv-turn-detail-actions">
              {activeTurn.dropped ? (
                <span className={"conv-state is-pill " + (activeTurn.read === 0 ? "is-hard" : "is-break")}>
                  {activeTurn.read === 0 ? tr("整段重写") : tr("未接续")}
                </span>
              ) : activeTurn.error ? (
                <span className="conv-state is-pill is-hard">{tr("请求失败")}</span>
              ) : activeTurn.expected ? (
                <span className="conv-state is-pill is-ok">{tr("接上")}</span>
              ) : (
                <span className="conv-state is-pill is-base">{tr("基线")}</span>
              )}
              <button
                type="button"
                className="conv-detail-close is-round"
                aria-label={tr("收起轮次详情")}
                onClick={() => {
                  userToggledRef.current = true;
                  setSelected(null);
                }}
              >
                <Icon name="close" width="14" height="14" />
              </button>
            </div>
          </div>
          <div className="conv-turn-metrics">
            <span>{tr("读")} <b>{activeTurn.read}</b></span>
            <span>{tr("应读")} <b>{activeTurn.expected ?? "—"}</b></span>
            <span>{tr("写")} <b>{activeTurn.write}</b></span>
            <span>{tr("缓存率")} <b>{rate(activeTurn.hit_rate)}</b></span>
          </div>
          {activeTurn.error && <p className="conv-turn-err">{activeTurn.error}</p>}
        </div>
      )}
      <details className="fold conv-turns" onToggle={updateScroll}>
        <summary>{tr("多轮对话缓存明细与衔接")}</summary>
        <p className="conv-scroll-hint">{tr("左右滑动查看完整表格")}</p>
        <div
          className="table-wrap conv-table-wrap"
          ref={wrapRef}
          data-scroll={scrollState}
          onScroll={updateScroll}
        >
          <table className="cache-table">
            <thead>
              <tr>
                <th title={tr("请求序号")}>#</th>
                <th title={tr("开场白 / 题号 / 闲聊")}>{tr("轮次")}</th>
                <th title={tr("上游回传的缓存读取 token")}>{tr("读")}</th>
                <th title={tr("本轮写入缓存的新 token")}>{tr("写")}</th>
                <th title={tr("上一轮读 + 写")}>{tr("应读")}</th>
                <th title={tr("本轮读 - 应读；负数为没接上")}>{tr("差值")}</th>
                <th title={tr("未命中缓存的输入 token")}>{tr("输入")}</th>
                <th title={tr("读 ÷ (输入 + 读 + 写)")}>{tr("缓存率")}</th>
                <th title={tr("接上 / 未接续 / 整段重写 / 基线")}>{tr("状态")}</th>
              </tr>
            </thead>
            <tbody>
              {turns.map((turn) => (
                <tr key={turn.index}>
                  <td>#{turn.index}</td>
                  <td>{label(turn)}</td>
                  <td>{turn.read}</td>
                  <td>{turn.write}</td>
                  <td>{turn.expected ?? "—"}</td>
                  <td className={turn.dropped ? "bad" : ""}>{turn.expected ? turn.read - turn.expected : "—"}</td>
                  <td>{turn.input}</td>
                  <td>{rate(turn.hit_rate)}</td>
                  <td>
                    {turn.dropped ? (
                      <span className={"conv-state is-pill " + (turn.read === 0 ? "is-hard" : "is-break")}>
                        {turn.read === 0 ? tr("整段重写") : tr("未接续")}
                      </span>
                    ) : turn.error ? (
                      <span className="conv-state is-pill is-hard">{tr("请求失败")}</span>
                    ) : turn.expected ? (
                      <span className="conv-state is-pill is-ok">{tr("接上")}</span>
                    ) : (
                      <span className="conv-state is-pill is-base">{tr("基线")}</span>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </details>
    </div>
  );
}
