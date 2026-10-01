"use client";
import AnimatedNumber from "@/components/AnimatedNumber";
import ResultSkeleton from "@/components/ResultSkeleton";
import Hint from "@/components/Hint";
import { fingerprintDiluted, probabilityVisible } from "@/lib/report-view";
import { DetectVerdict } from "@/lib/api";
function ScoreShare({ verdict, part }: { verdict: DetectVerdict; part: string }) {
  const row = verdict.total?.parts?.find((item) => item.key === part && item.applicable);
  if (!row) return null;
  return (
    <b className="score-chip" title="计入总分 / 系数">
      {row.contribution}/{row.coefficient}
    </b>
  );
}
function Details({ label, value }: { label: string; value: unknown }) {
  return (
    <details className="fold">
      <summary>{label}</summary>
      <pre>
        {typeof value === "string"
          ? value || "未回传"
          : JSON.stringify(value, null, 2)}
      </pre>
    </details>
  );
}


function knowledgeMark(row: { kind?: string; ok: boolean; unsure: boolean; error?: string }) {
  if (row.kind === "know" || (row.ok && row.kind !== "wrong" && row.kind !== "error" && row.kind !== "invalid")) {
    return { text: "知道", cls: "good" };
  }
  if (row.kind === "wrong" || (!row.ok && !row.unsure && !row.error && row.kind !== "unknown")) {
    return { text: "答错", cls: "warn" };
  }
  if (row.kind === "invalid") return { text: "无效", cls: "warn" };
  if (row.kind === "error" || row.error) return { text: "失败", cls: "warn" };
  return { text: "不知道", cls: "muted" };
}

function knowledgeLabel(k: { label?: string; cutoff?: string; correct?: number; total?: number }) {
  if (k.label) return k.label;
  if (k.cutoff && /^\d{4}-\d{2}$/.test(k.cutoff)) return `旧报告自报 ${k.cutoff}`;
  if (k.cutoff) return `知识≈${k.cutoff}${k.total ? `（${k.correct ?? 0}/${k.total}）` : ""}`;
  return "知识≈未知";
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
  const a = v.injection,
    f = v.fingerprint,
    t = v.thinking,
    q = v.iq;
  const knowledge = v.knowledge;
  const integrity = v.integrity;
  const claude = (model || "").toLowerCase().startsWith("claude-");
  const cache = v.cache;
  const cacheSamples = cache?.samples ?? [];
  const thirdCache = cacheSamples.length ? cacheSamples[cacheSamples.length - 1] : undefined;
  const previousCache = cacheSamples.length > 1 ? cacheSamples[cacheSamples.length - 2] : undefined;
  const thirdTotal = thirdCache && thirdCache.read_reported !== false ? thirdCache.total : null;
  const previousCacheTotal = previousCache &&
    previousCache.read_reported !== false && previousCache.write_reported !== false
    ? (cache?.third_read_baseline ?? previousCache.read + previousCache.write) : null;
  const cacheKnown = cache?.status !== "unknown" && cache?.third_read_ratio != null;
  const cacheToken = (value?: number | null, reported?: boolean) =>
    reported === false || value == null || !Number.isFinite(value) ? "—" : value;
  const cacheRate = (value?: number | null) =>
    value == null || !Number.isFinite(value) ? "—" : value.toFixed(2) + "%";
  const diluted = Boolean(f && !f.error && fingerprintDiluted(model || "", f.family, f.top_model));
  return (
    <div
      className={"results " + (loading ? "results-loading" : "results-settled")}
    >
      {v.total && (
        <section className="result-card total-card">
          <div className="rc-head">
            <h3>总分</h3>
            <b className="score-chip">{v.total.score} / 100</b>
          </div>
          <div className="metric-line">
            <strong className={v.total.score === 100 ? "good" : "warn"}>
              <AnimatedNumber value={v.total.score} />
            </strong>
            <span>/ 100</span>
          </div>
          <div className="integrity-signals">
            {v.total.parts.filter((item) => item.applicable).map((item) => (
              <div className="integrity-signal" key={item.key}>
                <span>{item.label}</span>
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
          <h3>模型指纹归因</h3>
          <span className="rc-score">
            <ScoreShare verdict={v} part="fingerprint" />
            <span className="card-number">01</span>
          </span>
        </div>
        {!f ? (
          <ResultSkeleton label={loading ? "指纹检测中…" : "暂无结果"} />
        ) : (
          <>
            {f.error ? (
              <div className="error-box">{f.error}</div>
            ) : (
              <>
                {diluted && (
                  <p className="fingerprint-alert">归因和所选模型对不上。这是被路由到别的模型，掺水。</p>
                )}
                <div className="metric-line">
                  <strong className={diluted ? "bad family-title" : "family-title"}>
                    {f.family_name || f.family}
                  </strong>
                  <span>家族概率 {(f.family_prob * 100).toFixed(1)}%</span>
                </div>
                <div className="result-note">
                  <Hint label="指纹归因说明" text={f.note} />
                </div>
                {f.results
                  ?.filter((r) => probabilityVisible(r.probability))
                  .slice(0, 3)
                  .map((r) => (
                    <div className="candidate" key={r.model}>
                      <div>
                        <span>{r.model}</span>
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
                  有效样本 {f.used_outputs} / 3
                  {typeof f.requests === "number" ? " · 请求 " + f.requests + " / 10" : ""}
                </p>
                {f.behavioral_top3 && (
                  <Details
                    label="行为相似候选（探索性参考）"
                    value={{
                      candidates: f.behavioral_top3,
                      note: f.behavior_note,
                    }}
                  />
                )}
              </>
            )}
            <Details label="数字挑战原始证据" value={f.evidence} />
          </>
        )}
      </section>
      <section className="result-card">
        <div className="rc-head">
          <h3>注入大小</h3>
          <span className="rc-score">
            <ScoreShare verdict={v} part="injection" />
            <span className="card-number">02</span>
          </span>
        </div>
        {!a ? (
          <ResultSkeleton label={loading ? "采样中…" : "暂无结果"} />
        ) : a.error ? (
          <div className="error-box">{a.error}</div>
        ) : (
          <>
            <div className="metric-line">
              <strong>
                ≈ <AnimatedNumber value={a.inj} />
              </strong>
              <span>token / 请求</span>
              <b className="score-chip">
                {a.score == null ? "待定" : a.score + " / 10"}
              </b>
            </div>
            <div className="result-note">
              <Hint label="注入估计说明" text={a.note} />
            </div>
            <div className="mini-stats">
              <span>
                斜率 <b>{a.slope.toFixed(3)}</b>
              </span>
              <span>
                复用采样 <b>{a.points.length} 次</b>
              </span>
              {a.points.length > 2 && a.max_residual != null && (
                <span>
                  最大残差 <b>{a.max_residual.toFixed(1)}</b>
                </span>
              )}
            </div>
            <details className="fold">
              <summary>采样明细与评分标准</summary>
              <div className="table-wrap">
                <table>
                  <thead>
                    <tr>
                      <th>轮次</th>
                      <th>样本字符</th>
                      <th>输入</th>
                      <th>合计</th>
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
                <p className="muted">每轮新增包装修正 {a.round_overhead.toFixed(2)} token</p>
              )}
              <p className="muted">
                &lt;50：10分 · 50～1000：8分 · 1001～3000：6分 · 3001～6000：4分
                · 6001～10000：2分 · &gt;10000：0分
              </p>
            </details>
            <Details
              label={
                a.probe
                  ? "SSE 与缓存头诊断（不计分）"
                  : "缓存头诊断（不增加请求）"
              }
              value={{ ...a.probe, cache_header: a.cache_header }}
            />
          </>
        )}
      </section>
      {!v.total && integrity && (
        <section className="result-card integrity-card">
          <div className="rc-head">
            <h3>接口完整性</h3>
            <span className="card-number">API</span>
          </div>
          <div className="metric-line">
            <strong className={integrity.score == null ? "warn" : "good"}>
              {integrity.score == null ? "—" : <AnimatedNumber value={integrity.score} />}
            </strong>
            <span>/ 100</span>
            <b className="score-chip">覆盖 {integrity.coverage.toFixed(0)}%</b>
          </div>
          <div className="result-note"><Hint label="接口完整性说明" text="只评价协议和证据链，不代表模型智力；未支持或缺字段项目会降低覆盖率，不会伪判为失败。" /></div>
          <p className="muted">{integrity.reason}</p>
          <div className="integrity-signals">
            {integrity.signals.filter(s => s.applicable).map(signal => (
              <div className="integrity-signal" key={signal.key}>
                <span>{signal.label}</span>
                <b className={!signal.known ? "warn" : signal.points && signal.points > 0 ? "good" : "bad"}>
                  {!signal.known ? "未验证" : signal.points && signal.max_points > 1 ? `${signal.points}/${signal.max_points}` : signal.points ? "通过" : "未通过"}
                </b>
              </div>
            ))}
          </div>
        </section>
      )}
      {claude && (
        <section className="result-card">
          <div className="rc-head">
            <h3>思维链回传</h3>
            <span className="rc-score">
              <ScoreShare verdict={v} part="thinking" />
              <span className="card-number">03</span>
            </span>
          </div>
          {!t ? (
            <ResultSkeleton label={loading ? "等待思维链…" : "暂无结果"} />
          ) : t.error || t.err ? (
            <div className="error-box">{t.error || t.err}</div>
          ) : (
            <>
              <div className="metric-line">
                <strong className={(t.score ?? ((t.think_len > 0 ? 5 : 0) + (t.sig_len > 0 ? 5 : 0))) === 10 ? "good" : "warn"}>
                  {(t.score ?? ((t.think_len > 0 ? 5 : 0) + (t.sig_len > 0 ? 5 : 0)))} / 10
                </strong>
              </div>
              <div className="mini-stats">
                <span>
                  思维摘要 <b>{t.summary_score ?? (t.think_len > 0 ? 5 : 0)} / 5</b>
                </span>
                <span>
                  signature <b>{t.signature_score ?? (t.sig_len > 0 ? 5 : 0)} / 5</b>
                </span>
              </div>
              <Details label="查看思维链全文" value={t.thinking_text} />
              <Details label="查看完整 signature 签名" value={t.signature} />
              <Details label="探针回答" value={t.answer} />
            </>
          )}
        </section>
      )}
      <section className="result-card knowledge-card">
        <div className="rc-head">
          <h3>知识截止日期</h3>
          <span className="card-number">INFO</span>
        </div>
        {!knowledge ? (
          <ResultSkeleton label={loading ? "知识截止探针中…" : "暂无结果"} />
        ) : knowledge.error ? (
          <div className="error-box">{knowledge.error}</div>
        ) : (
          <>
            <div className="metric-line">
              <strong className={knowledge.correct ? "good" : "warn"}>
                {knowledgeLabel(knowledge)}
              </strong>
              <span>客观事件</span>
            </div>
            <p className="muted">{knowledge.reason}</p>
            {knowledge.web_suspect && <p className="knowledge-note">{knowledge.web_suspect}</p>}
            {knowledge.gap_note && <p className="knowledge-note">{knowledge.gap_note}</p>}
            {!!knowledge.rows?.length && (
              <ul className="knowledge-ladder">
                {knowledge.rows.map((row) => (
                  <li key={row.id}>
                    <b className={knowledgeMark(row).cls}>{knowledgeMark(row).text}</b>
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
          <h3>推理表现</h3>
          <ScoreShare verdict={v} part="iq" />
          <span className="card-number">04</span>
        </div>
        {!q ? (
          <ResultSkeleton label={loading ? "推理测试中…" : "暂无结果"} />
        ) : (
          <>
            <div className="metric-line">
              <strong>
                {q.score == null ? "—" : <AnimatedNumber value={q.score} />}
              </strong>
              <span>/ 100</span>
              <b className="score-chip">
                {q.complete ? "本次测评" : "尚未完成"}
              </b>
            </div>
            <div className="result-note">
              <Hint label="评分说明" text={q.note} />
            </div>
            <div className="question-list">
              {q.per_question.map((x, index) => (
                <details key={x.id} className="question">
                  <summary>
                    <span>
                      Q{index + 1} · {x.title}
                    </span>
                    <b
                      className={
                        x.error ? "bad" : x.earned === x.max ? "good" : "warn"
                      }
                    >
                      {x.error
                        ? "请求失败"
                        : x.earned + " / " + x.max + (x.half ? "（半分）" : "")}
                    </b>
                  </summary>
                  {x.error && <p className="error-box">{x.error}</p>}
                  <h4>题目</h4>
                  <pre>{x.prompt}</pre>
                  <h4>标准结论</h4>
                  <pre>{x.expected}</pre>
                  <h4>模型回答</h4>
                  <pre>{x.answer || "正文为空，使用已返回思考文本判分"}</pre>
                  {claude && (
                    <Details
                      label="思维链全文与签名"
                      value={{
                        thinking: x.thinking_text,
                        signature: x.signature,
                      }}
                    />
                  )}
                  <Details
                    label="用量与耗时"
                    value={{
                      输入: x.metrics.InputTokens,
                      输出: x.metrics.OutputTokens,
                      缓存读: x.metrics.CacheRead,
                      缓存写:
                        x.metrics.CacheCreation5m + x.metrics.CacheCreation1h,
                      耗时: x.ms,
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
          <h3>缓存读写验证</h3>
          <ScoreShare verdict={v} part="cache" />
          <span className="card-number">CACHE</span>
        </div>
        {!v.cache ? (
          <ResultSkeleton label={loading ? "缓存验证中…" : "暂无结果"} />
        ) : v.cache.error ? (
          <div className="error-box">{v.cache.error}</div>
        ) : (
          <>
            <div className="metric-line">
              <strong className={v.cache.pass ? "good" : "warn"}>
                {v.cache.pass === true
                  ? "通过"
                  : v.cache.pass === false
                    ? "未对齐"
                    : "无法验证"}
              </strong>
            </div>
            <p className="muted">{v.cache.reason}</p>
            {v.cache.samples ? (
              <>
                <div className="cache-rates">
                  <div>
                    <span>末轮缓存率</span>
                    <strong>{cacheKnown ? cacheRate(v.cache.third_read_ratio) : "—"}</strong>
                  </div>
                  <div>
                    <span>末轮读取</span>
                    <strong>{cacheToken(thirdCache?.read, thirdCache?.read_reported)}</strong>
                  </div>
                  <div>
                    <span>末轮总输入</span>
                    <strong>{cacheToken(thirdTotal, thirdCache?.read_reported)}</strong>
                  </div>
                  <div>
                    <span>上一轮读 ＋ 写</span>
                    <strong>{cacheToken(previousCacheTotal)}</strong>
                  </div>
                </div>
                <p className="muted">
                  {cacheKnown && previousCacheTotal != null
                    ? `${cacheToken(thirdCache?.read, thirdCache?.read_reported)} ÷ ${cacheToken(previousCacheTotal)} × 100%`
                    : "—"}
                  <Hint
                    label="末轮缓存率说明"
                    text="上一条缓存读 + 写 = 下一条缓存读，就是 100%。末轮缓存率 = 末轮读取 ÷ 上一轮读加写。缺字段或请求失败时不计算。"
                  />
                </p>
                {v.cache.third_read_ratio != null && v.cache.third_read_ratio > 100 && (
                  <p className="muted">末轮读取超过上一轮读加写，保留原始比例。</p>
                )}
                <div className="table-wrap">
                  <table className="cache-table">
                    <thead>
                      <tr>
                        <th>请求</th>
                        <th>读</th>
                        <th>写</th>
                        <th>输入</th>
                        <th>输出</th>
                        <th>命中率</th>
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
                  前次读 <b>{v.cache.first?.read ?? "—"}</b>
                </span>
                <span>
                  ＋ 写 <b>{v.cache.first?.write ?? "—"}</b>
                </span>
                <span>
                  → 后次读 <b>{v.cache.second?.read ?? "—"}</b>
                </span>
              </div>
            )}
            <Details
              label={v.cache.samples ? "多轮缓存明细与衔接" : "旧报告缓存明细"}
              value={v.cache}
            />
          </>
        )}
      </section>}
      {(
        [
          ["工具调用检测", v.tools],
          ["联网检测", v.web],
        ] as const
      ).map(([title, result]) => (
        <section className="result-card" key={title}>
          <div className="rc-head">
            <h3>{title}</h3>
            <ScoreShare verdict={v} part={title === "工具调用检测" ? "tools" : "web"} />
          </div>
          {!result ? (
            <ResultSkeleton label={loading ? "检测中…" : "暂无结果"} />
          ) : result.error ? (
            <div className="error-box">{result.error}</div>
          ) : (
            <>
              <div className="metric-line">
                <strong className={result.pass === true ? "good" : "warn"}>
                  {result.pass === true
                    ? "通过"
                    : result.pass === false
                      ? "未通过"
                      : "未经验证"}
                </strong>
              </div>
              <p className="muted">{result.reason}</p>
              {result.evidence != null && (
                <Details label="检测证据" value={result.evidence} />
              )}
            </>
          )}
        </section>
      ))}
    </div>
  );
}
