"use client";
import AnimatedNumber from "@/components/AnimatedNumber";
import ResultSkeleton from "@/components/ResultSkeleton";
import ErrorFold from "@/components/ErrorFold";
import Hint from "@/components/Hint";
import { fingerprintDiluted, presentedTotal, probabilityVisible } from "@/lib/report-view";
import { DetectVerdict } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
function ScoreShare({ verdict, part }: { verdict: DetectVerdict; part: string }) {
  const { t: tr } = useI18n();
  const row = verdict.total?.parts?.find((item) => item.key === part && item.applicable);
  if (!row) return null;
  return (
    <b className={"score-chip" + (row.contribution === 0 ? " is-zero" : "")} title={tr("计入总分 / 系数")}>
      {row.contribution}/{row.coefficient}
    </b>
  );
}
function CardTitle({ title, label, text }: { title: string; label?: string; text?: string }) {
  const { t: tr } = useI18n();
  return (
    <div className="rc-title">
      <h3>{tr(title)}</h3>
      {text ? <Hint label={label || title} text={text} /> : null}
    </div>
  );
}
function Details({ label, value }: { label: string; value: unknown }) {
  const { t: tr } = useI18n();
  return (
    <details className="fold">
      <summary>{tr(label)}</summary>
      <pre>
        {typeof value === "string"
          ? value || tr("未回传")
          : JSON.stringify(value, null, 2)}
      </pre>
    </details>
  );
}


function knowledgeMark(row: { kind?: string; ok: boolean; unsure: boolean; error?: string }, tr: (source: string) => string) {
  if (row.kind === "know" || (row.ok && row.kind !== "wrong" && row.kind !== "error" && row.kind !== "invalid")) {
    return { text: tr("知道"), cls: "good" };
  }
  if (row.kind === "wrong" || (!row.ok && !row.unsure && !row.error && row.kind !== "unknown")) {
    return { text: tr("答错"), cls: "warn" };
  }
  if (row.kind === "invalid") return { text: tr("无效"), cls: "warn" };
  if (row.kind === "error" || row.error) return { text: tr("失败"), cls: "warn" };
  return { text: tr("不知道"), cls: "muted" };
}

function knowledgeLabel(k: { label?: string; cutoff?: string; correct?: number; total?: number }, tr: (source: string, vars?: Record<string, string | number>) => string) {
  if (k.label) return tr(k.label);
  if (k.cutoff && /^\d{4}-\d{2}$/.test(k.cutoff)) return tr("旧报告自报 {cutoff}", { cutoff: k.cutoff });
  if (k.cutoff && k.total) return tr("知识≈{cutoff}（{a}/{b}）", { cutoff: k.cutoff, a: k.correct ?? 0, b: k.total });
  if (k.cutoff) return tr("知识≈{cutoff}", { cutoff: k.cutoff });
  return tr("知识≈未知");
}
export default function ResultCards({
  verdict: v,
  model,
  loading = false,
  showCache = false,
}: {
  verdict: DetectVerdict;
  model?: string;
  loading?: boolean;
  showCache?: boolean;
}) {
  const { t: tr } = useI18n();
  const a = v.injection,
    f = v.fingerprint,
    t = v.thinking,
    q = v.iq;
  const hidden = v.hidden;
  // v0.1.27/28 落库的报告里 signals/asks 可能是 null（后端 nil 切片），
  // 旧报告不会被重写，所以前端必须兜底。
  const hiddenSignals = hidden?.signals ?? [];
  const hiddenAsks = hidden?.asks ?? [];
  const knowledge = v.knowledge;
  const integrity = v.integrity;
  const claude = (model || "").toLowerCase().startsWith("claude-");
  const cache = v.cache;
  const cacheSamples = cache?.samples ?? [];
  const thirdCache = cacheSamples.length ? cacheSamples[cacheSamples.length - 1] : undefined;
  const previousCache = cacheSamples.length > 1 ? cacheSamples[cacheSamples.length - 2] : undefined;
  const thirdTotal = thirdCache && thirdCache.read_reported !== false ? thirdCache.total : null;
  const previousCacheTotal = previousCache
    ? cache?.third_read_baseline != null
      ? cache.third_read_baseline
      : previousCache.read_reported !== false
        ? previousCache.write_reported === true
          ? previousCache.read + previousCache.write
          : previousCache.read
        : null
    : null;
  const cacheKnown = cache?.status !== "unknown" && cache?.third_read_ratio != null;
  const reportedReads = cacheSamples.filter(
    (row) => row.read_reported !== false && !row.error,
  ).length;
  const readOnlyCache =
    cacheSamples.length > 1 &&
    cacheSamples.every((row) => row.write_reported === false && !row.error) &&
    reportedReads >= 2;
  const cacheToken = (value?: number | null, reported?: boolean) =>
    reported === false || value == null || !Number.isFinite(value) ? "—" : value;
  const cacheRate = (value?: number | null) =>
    value == null || !Number.isFinite(value) ? "—" : value.toFixed(2) + "%";
  const behavioral = f?.basis === "behavioral";
  const diluted = Boolean(
    f &&
      !f.error &&
      !f.low_confidence &&
      fingerprintDiluted(model || "", f.family, f.top_model),
  );
  const scored: DetectVerdict = v.total
    ? {
        ...v,
        total: {
          ...v.total,
          ...presentedTotal(v.total.score, v.total.parts, diluted),
        },
      }
    : v;
  return (
    <div
      className={"results " + (loading ? "results-loading" : "results-settled")}
    >
      {scored.total && (
        <section className="result-card total-card">
          <div className="rc-head">
            <h3>{tr("总分")}</h3>
            <b className="score-chip">{scored.total.score} / 100</b>
          </div>
          <div className="metric-line">
            <strong className={scored.total.score >= 90 ? "good" : scored.total.score >= 60 ? "score-mid" : "bad"}>
              <AnimatedNumber value={scored.total.score} />
            </strong>
            <span>/ 100</span>
          </div>
          <div className="integrity-signals">
            {scored.total.parts.filter((item) => item.applicable).map((item) => (
              <div className="integrity-signal" key={item.key}>
                <span>{tr(item.label)}</span>
                <b className={item.contribution === item.coefficient ? "good" : "bad"}>
                  {item.contribution}/{item.coefficient}
                </b>
              </div>
            ))}
          </div>
        </section>
      )}
      <section className={"result-card" + (diluted ? " fingerprint-bad" : "")}>
        <div className="rc-head">
          <CardTitle title={tr("模型指纹归因")} label={tr("指纹归因说明")} text={f?.note} />
          <span className="rc-score">
            <ScoreShare verdict={scored} part="fingerprint" />
            <span className="card-number">01</span>
          </span>
        </div>
        {diluted && (
          <div className="fake-seal" aria-hidden="true">
            <span>{tr("假")}</span>
          </div>
        )}
        {!f ? (
          <ResultSkeleton label={loading ? tr("指纹检测中…") : tr("暂无结果")} />
        ) : (
          <>
            {f.error ? (
              <ErrorFold text={f.error} />
            ) : (
              <>
                <div className="model-claim">
                  <p>
                    <span>{tr("声称")}</span>
                    <b>{model || tr("未填写")}</b>
                  </p>
                  <p>
                    <span>{tr("实际")}</span>
                    <b className={diluted ? "bad" : ""}>
                      {f.top_display || f.top_model || f.family_name || tr("未归因")}
                    </b>
                  </p>
                </div>
                <p className="basis-line">
                  <span className={"basis-chip" + (behavioral ? " behavioral" : "")}>
                    {behavioral ? tr("行为归因") : tr("数字归因")}
                  </span>
                  <span className="muted">
                    {behavioral
                      ? tr("数字库没有该家族，已用行为库 {n} 个候选直接比对。", {
                          n: f.behavioral_total ?? 34,
                        })
                      : tr("按数字基线候选比较。")}
                    {f.low_confidence ? " " + tr("证据不足，不判掺水。") : ""}
                  </span>
                </p>
                <div className="metric-line">
                  <strong className={diluted ? "bad family-title" : "family-title"}>
                    {f.family_name || f.family}
                  </strong>
                  <span>
                    {behavioral
                      ? tr("行为相似度 {n}%", {
                          n: ((f.top_similarity ?? f.top_prob ?? 0) * 100).toFixed(1),
                        })
                      : tr("家族概率 {n}%", {
                          n: (f.family_prob * 100).toFixed(1),
                        })}
                  </span>
                </div>
                {f.results
                  ?.filter((r) => probabilityVisible(r.probability))
                  .slice(0, 3)
                  .map((r) => (
                    <div className="candidate" key={r.model}>
                      <div>
                        <span>{r.display || r.model}</span>
                        <b>{(r.probability * 100).toFixed(1)}%</b>
                      </div>
                      <div className="prob-bar">
                        <div
                          style={{ transform: "scaleX(" + r.probability + ")" }}
                        />
                      </div>
                    </div>
                  ))}
                <p className="muted">
                  {behavioral
                    ? tr("行为探针 3 个 · 相似度按 6 维特征比对")
                    : <>
                        {tr("有效样本 {a} / 3", { a: f.used_outputs })}
                        {typeof f.requests === "number"
                          ? tr(" · 请求 {n} / 10", { n: f.requests })
                          : ""}
                      </>}
                </p>
                {!behavioral && f.behavioral_top3 && (
                  <Details
                    label={tr("行为相似候选（探索性参考）")}
                    value={{
                      candidates: f.behavioral_top3,
                      note: f.behavior_note,
                    }}
                  />
                )}
              </>
            )}
            {!behavioral && f.evidence?.length ? (
              <Details label={tr("数字挑战原始证据")} value={f.evidence} />
            ) : null}
          </>
        )}
      </section>
      <section className="result-card">
        <div className="rc-head">
          <CardTitle
            title={tr("隐藏指令检测")}
            label={tr("隐藏指令检测说明")}
            text={tr("按多种问法的自述一致性与自报知识截止日期判断上游是否在用户消息前注入了隐藏指令。不依赖上游上报的 token 数，因此不受计数抹平影响。")}
          />
          <span className="rc-score">
            <ScoreShare verdict={scored} part="hidden" />
            <span className="card-number">02</span>
          </span>
        </div>
        {!hidden ? (
          <ResultSkeleton label={loading ? tr("追问中…") : tr("暂无结果")} />
        ) : hidden.error ? (
          <ErrorFold text={hidden.error} />
        ) : (
          <>
            <div className="metric-line">
              <strong>{hidden.status === "injected" ? tr("检出隐藏指令") : hidden.status === "clean" ? tr("未检出") : hidden.status === "suspect" ? tr("存疑") : tr("无法判定")}</strong>
              <b className="score-chip">{hidden.score + " / " + hidden.max_score}</b>
            </div>
            <div className="mini-stats">
              <span>
                {tr("承认存在前置指令")} <b>{hidden.ack_count}</b>
              </span>
              <span>
                {tr("否认")} <b>{hidden.deny_count}</b>
              </span>
              {hidden.identity?.claimed_cutoff ? (
                <span>
                  {tr("自报截止")} <b>{hidden.identity.claimed_cutoff}</b>
                  {hidden.identity.cutoff_mismatch ? <> {tr("≠ 官方 {v}", { v: hidden.identity.expected_cutoff || "?" })}</> : null}
                </span>
              ) : null}
            </div>
            {hiddenSignals.length ? (
              <ul className="mini-list">
                {hiddenSignals.map((s, i) => (
                  <li key={i}>{s}</li>
                ))}
              </ul>
            ) : null}
            <details className="fold">
              <summary>{tr("各问法原始回答")}</summary>
              <div className="table-wrap">
                <table>
                  <thead>
                    <tr>
                      <th>{tr("问法")}</th>
                      <th>{tr("判读")}</th>
                      <th>{tr("回答")}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {hiddenAsks.map((row) => (
                      <tr key={row.id}>
                        <td>{row.id}</td>
                        <td>{row.stance}</td>
                        <td>{row.error ? row.error : row.answer}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </details>
          </>
        )}
      </section>
      <section className="result-card">
        <div className="rc-head">
          <CardTitle title={tr("注入大小")} label={tr("注入估计说明")} text={a?.note} />
          <span className="rc-score">
            <ScoreShare verdict={scored} part="injection" />
            <span className="card-number">02</span>
          </span>
        </div>
        {!a ? (
          <ResultSkeleton label={loading ? tr("采样中…") : tr("暂无结果")} />
        ) : a.error ? (
          <ErrorFold text={a.error} />
        ) : (
          <>
            <div className="metric-line">
              <strong>
                ≈ <AnimatedNumber value={a.inj} />
              </strong>
              <span>{tr("token / 请求")}</span>
              <b className="score-chip">
                {a.score == null ? tr("待定") : a.score + " / 10"}
              </b>
            </div>
            <div className="mini-stats">
              <span>
                {tr("斜率")} <b>{a.slope.toFixed(3)}</b>
              </span>
              <span>
                {tr("复用采样")} <b>{tr("{n} 次", { n: a.points.length })}</b>
              </span>
              {a.points.length > 2 && a.max_residual != null && (
                <span>
                  {tr("最大残差")} <b>{a.max_residual.toFixed(1)}</b>
                </span>
              )}
            </div>
            <details className="fold">
              <summary>{tr("采样明细与评分标准")}</summary>
              <div className="table-wrap">
                <table>
                  <thead>
                    <tr>
                      <th>{tr("轮次")}</th>
                      <th>{tr("样本字符")}</th>
                      <th>{tr("输入")}</th>
                      <th>{tr("合计")}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {a.points.map((p, index) => (
                      <tr key={p.round || p.n}>
                        {[
                          p.round || index + 1, p.n, p.input, p.total,
                        ].map((n, i) => (
                          <td key={i} title={JSON.stringify(p)}>
                            {n}
                          </td>
                        ))}
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
              {a.round_overhead != null && (
                <p className="muted">{tr("每轮新增包装修正 {n} token", { n: a.round_overhead.toFixed(2) })}</p>
              )}
              <p className="muted">
                {tr("<50：10分 · 50～1000：8分 · 1001～3000：6分 · 3001～6000：4分 · 6001～10000：2分 · >10000：0分")}
              </p>
            </details>
            <Details
              label={
                a.probe
                  ? tr("SSE 与缓存头诊断（不计分）")
                  : tr("缓存头诊断（不增加请求）")
              }
              value={{ ...a.probe, cache_header: a.cache_header }}
            />
          </>
        )}
      </section>
      {!v.total && integrity && (
        <section className="result-card integrity-card">
          <div className="rc-head">
            <CardTitle title={tr("接口完整性")} label={tr("接口完整性说明")} text={tr("只评价协议和证据链，不代表模型智力。未支持或缺字段会降低覆盖率，不会伪判为失败。")} />
            <span className="card-number">API</span>
          </div>
          <div className="metric-line">
            <strong className={integrity.score == null ? "warn" : "good"}>
              {integrity.score == null ? "—" : <AnimatedNumber value={integrity.score} />}
            </strong>
            <span>/ 100</span>
            <b className="score-chip">{tr("覆盖 {n}%", { n: integrity.coverage.toFixed(0) })}</b>
          </div>
          <p className="muted">{tr(integrity.reason || "")}</p>
          <div className="integrity-signals">
            {integrity.signals.filter(s => s.applicable).map(signal => (
              <div className="integrity-signal" key={signal.key}>
                <span>{tr(signal.label)}</span>
                <b className={!signal.known ? "warn" : signal.points && signal.points > 0 ? "good" : "bad"}>
                  {!signal.known ? tr("未验证") : signal.points && signal.max_points > 1 ? `${signal.points}/${signal.max_points}` : signal.points ? tr("通过") : tr("未通过")}
                </b>
              </div>
            ))}
          </div>
        </section>
      )}
      {claude && (
        <section className="result-card">
          <div className="rc-head">
            <h3>{tr("思维链回传")}</h3>
            <span className="rc-score">
              <ScoreShare verdict={scored} part="thinking" />
              <span className="card-number">03</span>
            </span>
          </div>
          {!t ? (
            <ResultSkeleton label={loading ? tr("等待思维链…") : tr("暂无结果")} />
          ) : t.error || t.err ? (
            <ErrorFold text={t.error || t.err || ""} />
          ) : (
            <>
              <div className="metric-line">
                <strong className={(t.summary_score ?? (t.think_len > 0 ? 5 : 0)) === 5 ? "good" : "warn"}>
                  {(t.summary_score ?? (t.think_len > 0 ? 5 : 0))} / 5
                </strong>
              </div>
              <Details label={tr("查看思维链全文")} value={t.thinking_text} />
              <Details label={tr("探针回答")} value={t.answer} />
            </>
          )}
        </section>
      )}
      <section className="result-card knowledge-card">
        <div className="rc-head">
          <h3>{tr("知识截止日期")}</h3>
          <span className="card-number">INFO</span>
        </div>
        {!knowledge ? (
          <ResultSkeleton label={loading ? tr("知识截止探针中…") : tr("暂无结果")} />
        ) : knowledge.error ? (
          <ErrorFold text={knowledge.error} />
        ) : (
          <>
            <div className="metric-line">
              <strong className={knowledge.correct ? "good" : "warn"}>
                {knowledgeLabel(knowledge, tr)}
              </strong>
              <span>{tr("客观事件")}</span>
            </div>
            <p className="muted">{tr(knowledge.reason || "")}</p>
            {knowledge.web_suspect && <p className="knowledge-note">{tr(knowledge.web_suspect)}</p>}
            {knowledge.gap_note && <p className="knowledge-note">{tr(knowledge.gap_note)}</p>}
            {!!knowledge.rows?.length && (
              <ul className="knowledge-ladder">
                {knowledge.rows.map((row) => (
                  <li key={row.id}>
                    <b className={knowledgeMark(row, tr).cls}>{knowledgeMark(row, tr).text}</b>
                    <span>{row.happened_at || row.quarter}</span>
                    <span>{row.event}</span>
                  </li>
                ))}
              </ul>
            )}
          </>
        )}
      </section>
      <section className="result-card">
        <div className="rc-head">
          <CardTitle title={tr("推理表现")} label={tr("评分说明")} text={q?.note} />
          <span className="rc-score">
            <ScoreShare verdict={scored} part="iq" />
            <span className="card-number">04</span>
          </span>
        </div>
        {!q ? (
          <ResultSkeleton label={loading ? tr("推理测试中…") : tr("暂无结果")} />
        ) : (
          <>
            <div className="metric-line">
              <strong>
                {q.score == null ? "—" : <AnimatedNumber value={q.score} />}
              </strong>
              <span>/ 100</span>
              <b className="score-chip">
                {q.complete ? tr("本次测评") : tr("尚未完成")}
              </b>
            </div>
            <div className="question-list">
              {q.per_question.map((x, index) => (
                <details key={x.id} className="question">
                  <summary>
                    <span>
                      Q{index + 1} · {tr(x.title)}
                    </span>
                    <b
                      className={
                        x.error ? "bad" : x.earned === x.max ? "good" : "warn"
                      }
                    >
                      {x.error
                        ? tr("请求失败")
                        : x.earned + " / " + x.max + (x.half ? tr("（半分）") : "")}
                    </b>
                  </summary>
                  {x.error && <ErrorFold text={x.error} />}
                  <h4>{tr("题目")}</h4>
                  <pre>{x.prompt}</pre>
                  <h4>{tr("标准结论")}</h4>
                  <pre>{x.expected}</pre>
                  <h4>{tr("模型回答")}</h4>
                  <pre>{x.answer || tr("正文为空，使用已返回思考文本判分")}</pre>
                  {claude && (
                    <Details
                      label={tr("思维链全文与签名")}
                      value={{
                        thinking: x.thinking_text,
                        signature: x.signature,
                      }}
                    />
                  )}
                  <Details
                    label={tr("用量与耗时")}
                    value={{
                      [tr("输入")]: x.metrics.InputTokens,
                      [tr("输出")]: x.metrics.OutputTokens,
                      [tr("缓存读")]: x.metrics.CacheRead,
                      [tr("缓存写")]: x.metrics.CacheCreation5m + x.metrics.CacheCreation1h,
                      [tr("耗时")]: x.ms,
                    }}
                  />
                  {x.tags?.length > 0 && <p>{x.tags.join(" · ")}</p>}
                </details>
              ))}
            </div>
          </>
        )}
      </section>
      {(showCache || cache) && <section className="result-card cache-card">
        <div className="rc-head">
          <h3>{tr("缓存读写验证")}</h3>
          <span className="rc-score">
            <ScoreShare verdict={scored} part="cache" />
            <span className="card-number">CACHE</span>
          </span>
        </div>
        {!v.cache ? (
          <ResultSkeleton label={loading ? tr("缓存验证中…") : tr("暂无结果")} />
        ) : v.cache.error ? (
          <ErrorFold text={v.cache.error} />
        ) : (
          <>
            <div className="metric-line">
              <strong className={v.cache.pass ? "good" : "warn"}>
                {v.cache.pass === true
                  ? tr("通过")
                  : v.cache.pass === false
                    ? tr("未对齐")
                    : tr("无法验证")}
              </strong>
            </div>
            <p className="muted">{v.cache.reason}</p>
            {v.cache.samples ? (
              <>
                <div className="cache-rates">
                  <div>
                    <span>{tr("末轮缓存率")}</span>
                    <strong>{cacheKnown ? cacheRate(v.cache.third_read_ratio) : "—"}</strong>
                  </div>
                  <div>
                    <span>{tr("末轮读取")}</span>
                    <strong>{cacheToken(thirdCache?.read, thirdCache?.read_reported)}</strong>
                  </div>
                  <div>
                    <span>{tr("末轮总输入")}</span>
                    <strong>{cacheToken(thirdTotal, thirdCache?.read_reported)}</strong>
                  </div>
                  <div>
                    <span>{readOnlyCache ? tr("上一轮读取") : tr("上一轮读 ＋ 写")}</span>
                    <strong>{cacheToken(previousCacheTotal)}</strong>
                  </div>
                </div>
                <p className="muted">
                  {cacheKnown && readOnlyCache
                    ? `${cacheToken(thirdCache?.read, thirdCache?.read_reported)} ÷ ${cacheToken(thirdTotal, thirdCache?.read_reported)}`
                    : cacheKnown && previousCacheTotal != null
                    ? `${cacheToken(thirdCache?.read, thirdCache?.read_reported)} ÷ ${cacheToken(previousCacheTotal)} × 100%`
                    : "—"}
                  <Hint
                    label={tr("末轮缓存率说明")}
                    text={
                      readOnlyCache
                        ? tr("上游没有缓存写入时，末轮缓存率 = 末轮读取 ÷ 末轮总输入。上一轮读取作为读取链参考，缺写不当成 0。")
                        : tr("上一条缓存读 + 写 = 下一条缓存读，就是 100%。末轮缓存率 = 末轮读取 ÷ 上一轮读加写。缺字段或请求失败时不计算。")
                    }
                  />
                </p>
                {v.cache.third_read_ratio != null && v.cache.third_read_ratio > 100 && (
                  <p className="muted">{tr("末轮读取超过上一轮读加写，保留原始比例。")}</p>
                )}
                <div className="table-wrap">
                  <table className="cache-table">
                    <thead>
                      <tr>
                        <th>{tr("请求")}</th>
                        <th>{tr("读")}</th>
                        <th>{tr("写")}</th>
                        <th>{tr("输入")}</th>
                        <th>{tr("输出")}</th>
                        <th>{tr("命中率")}</th>
                      </tr>
                    </thead>
                    <tbody>
                      {v.cache.samples.map((row) => (
                        <tr key={row.index}>
                          <td>#{row.index}</td>
                          <td>{cacheToken(row.read, row.read_reported)}</td>
                          <td>{cacheToken(row.write, row.write_reported)}</td>
                          <td>{cacheToken(row.input, row.input_reported)}</td>
                          <td>{cacheToken(row.output, row.output_reported)}</td>
                          <td>{cacheRate(row.hit_rate)}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </>
            ) : (
              <div className="cache-equation">
                <span>
                  {tr("前次读")} <b>{v.cache.first?.read ?? "—"}</b>
                </span>
                <span>
                  {tr("＋ 写")} <b>{v.cache.first?.write ?? "—"}</b>
                </span>
                <span>
                  {tr("→ 后次读")} <b>{v.cache.second?.read ?? "—"}</b>
                </span>
              </div>
            )}
            <Details
              label={v.cache.samples ? tr("多轮缓存明细与衔接") : tr("旧报告缓存明细")}
              value={v.cache}
            />
          </>
        )}
      </section>}
      {(
        [
          [tr("工具调用检测"), v.tools],
          [tr("联网检测"), v.web],
        ] as const
      ).map(([title, result]) => (
        <section className="result-card" key={title}>
          <div className="rc-head">
            <h3>{title}</h3>
            <span className="rc-score">
              {result?.protocol === "deepseek_search" && (
                <span className="basis-chip behavioral">DeepSeek Search API</span>
              )}
              <ScoreShare
                verdict={scored}
                part={title === tr("工具调用检测") ? "tools" : "web"}
              />
            </span>
          </div>
          {!result ? (
            <ResultSkeleton label={loading ? tr("检测中…") : tr("暂无结果")} />
          ) : result.error ? (
            <ErrorFold text={result.error} />
          ) : (
            <>
              <div className="metric-line">
                <strong className={result.pass === true ? "good" : "warn"}>
                  {result.pass === true
                    ? tr("通过")
                    : result.pass === false
                      ? tr("未通过")
                      : tr("未经验证")}
                </strong>
              </div>
              <p className="muted">{tr(result.reason || "")}</p>
              {result.evidence != null && (
                <Details label={tr("检测证据")} value={result.evidence} />
              )}
            </>
          )}
        </section>
      ))}
    </div>
  );
}
