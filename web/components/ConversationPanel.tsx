"use client";
import { ConversationVerdict } from "@/lib/api";
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
          {data.drops ? tr("掉缓存") + " " + data.drops : state}
        </b>
      </div>
      <p className="muted">{tr(data.reason)}</p>
          <details className="fold conv-turns">
            <summary>{tr("对话缓存明细")}</summary>
            <div className="table-wrap">
              <table className="cache-table">
                <thead>
                  <tr>
                    <th>#</th>
                    <th>{tr("轮次")}</th>
                    <th>{tr("读")}</th>
                    <th>{tr("写")}</th>
                    <th>{tr("输入")}</th>
                    <th>{tr("缓存率")}</th>
                    <th>{tr("状态")}</th>
                  </tr>
                </thead>
                <tbody>
                  {turns.map((turn) => (
                    <tr key={turn.index}>
                      <td>#{turn.index}</td>
                      <td>
                        {turn.kind === "question"
                          ? (turn.id || "") + " · " + (turn.title || "")
                          : turn.kind === "opener"
                            ? tr("开场白")
                            : tr("闲聊")}
                      </td>
                      <td>{turn.read}</td>
                      <td>{turn.write}</td>
                      <td>{turn.input}</td>
                      <td>{rate(turn.hit_rate)}</td>
                      <td className={turn.dropped ? "bad" : ""}>
                        {turn.dropped
                          ? tr("整段重写")
                          : turn.error
                            ? tr("请求失败")
                            : ""}
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
