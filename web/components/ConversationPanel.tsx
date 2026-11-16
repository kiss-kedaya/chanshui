"use client";
import { ConversationTurn, ConversationVerdict } from "@/lib/api";
import { useI18n } from "@/lib/i18n";

// The serial-conversation evidence: one row per request so a cache read that
// falls back to zero mid-thread is visible, not buried in a score.
export default function ConversationPanel({
  data,
}: {
  data?: ConversationVerdict | null;
}) {
  const { t: tr } = useI18n();
  if (!data || !data.rounds) return null;
  const turns = data.turns ?? [];
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
      ? (turn.id || "") + " · " + (turn.title || "")
      : turn.kind === "opener"
        ? tr("开场白")
        : tr("闲聊");
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
      <p className="muted">{tr(data.reason)}</p>
      <div className="conv-chain" aria-hidden="true">
        {turns.map((turn) => (
          <span
            key={turn.index}
            className={"conv-chain-seg " + (turn.dropped ? "is-break" : turn.expected ? "is-ok" : "is-base")}
            title={`#${turn.index} ${label(turn)} · ${tr("读")} ${turn.read} / ${tr("应读")} ${turn.expected ?? "—"} · ${tr("写")} ${turn.write}`}
          />
        ))}
      </div>
      <div className="conv-legend">
        <span><i className="is-ok" />{tr("接上")}</span>
        <span><i className="is-break" />{tr("未接续")}</span>
        <span><i className="is-base" />{tr("基线")}</span>
      </div>
          <details className="fold conv-turns">
            <summary>{tr("多轮对话缓存明细与衔接")}</summary>
            <div className="table-wrap">
              <table className="cache-table">
                <thead>
                  <tr>
                    <th>#</th>
                    <th>{tr("轮次")}</th>
                    <th>{tr("读")}</th>
                    <th>{tr("写")}</th>
                    <th>{tr("应读")}</th>
                    <th>{tr("差值")}</th>
                    <th>{tr("输入")}</th>
                    <th>{tr("缓存率")}</th>
                    <th>{tr("状态")}</th>
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
                          <span className={"conv-state " + (turn.read === 0 ? "is-hard" : "is-break")}>
                            {turn.read === 0 ? tr("整段重写") : tr("未接续")}
                          </span>
                        ) : turn.error ? (
                          <span className="conv-state is-hard">{tr("请求失败")}</span>
                        ) : turn.expected ? (
                          <span className="conv-state is-ok">{tr("接上")}</span>
                        ) : (
                          <span className="conv-state is-base">{tr("基线")}</span>
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
