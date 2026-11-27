const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const vm = require("node:vm");
const ts = require("typescript");
const React = require("react");
const { renderToStaticMarkup } = require("react-dom/server");

function loadModule(filename) {
  const source = fs.readFileSync(path.join(__dirname, filename), "utf8");
  const compiled = ts.transpileModule(source, {
    compilerOptions: { module: ts.ModuleKind.CommonJS, jsx: ts.JsxEmit.ReactJSX },
  }).outputText;
  const scope = { exports: {}, require: (name) => require("./app-require.cjs")(name) };
  vm.runInNewContext(compiled, scope);
  return scope.exports;
}
const ResultCards = loadModule("../components/ResultCards.tsx").default;

const part = (key, contribution, coefficient, extra = {}) => ({
  key, label: key, applicable: true, known: true, points: 0, max_points: 1,
  weight: coefficient, coefficient, contribution, ...extra,
});
const verdict = (over = {}) => ({
  total: {
    score: 96, max: 100, reason: "",
    parts: [part("fingerprint", 21, 21), part("iq", 28, 28), part("injection", 15, 15), part("tools", 11, 11), part("thinking", 8, 8)],
  },
  fingerprint: { family: "claude", family_name: "Claude", family_prob: 1, top_model: "claude-opus-5-5", top_prob: 1, results: [], used_outputs: 3, low_confidence: false, note: "", evidence: [] },
  injection: { inj: 0, slope: 0, r2: null, max_residual: null, reliable: true, score: 10, max_score: 10, note: "", points: [], cache_header: "" },
  thinking: { pass: true, score: 12, max_score: 12, signature_present: true, original_replay: true, tamper_rejected: true, tamper_status: 400, think_len: 10, sig_len: 10, reason: "", thinking_text: "", signature: "", answer: "" },
  iq: { score: 100, max: 100, paper_max: 100, complete: true, per_question: [], note: "" },
  tools: { status: "pass", pass: true, reason: "", protocol: "anthropic" },
  ...over,
});
const render = (v, props = {}) =>
  renderToStaticMarkup(React.createElement(ResultCards, { verdict: v, model: "claude-opus-5-5", ...props }));

test("the verdict hero leads the report with a headline, number and one row per scored section", () => {
  const html = render(verdict());
  assert.match(html, /<section class="result-card score-hero" data-level="ok"/);
  assert.match(html, /<p class="sh-headline">未发现掺水迹象<\/p>/);
  assert.match(html, /sh-number tone-ok/);
  assert.equal((html.match(/class="sh-part"/g) || []).length, 5);
  assert.ok(html.indexOf("score-hero") < html.indexOf("group-identity"));
});

test("evidence is grouped in a fixed order and the cache group only exists when there is cache evidence", () => {
  const html = render(verdict());
  assert.ok(html.indexOf("group-identity") < html.indexOf("group-ability"));
  assert.ok(!html.includes("group-cache"));
  assert.ok(!html.includes("card-number"));
  const cache = { status: "pass", pass: true, reason: "", first: null, second: null, expected_read: 0, actual_read: 0, delta: 0, protocol: "" };
  const withCache = render(verdict({ cache }));
  assert.ok(withCache.indexOf("group-ability") < withCache.indexOf("group-cache"));
  const forced = render(verdict({ force_cache: { steps: [], forced: true, conclusion: "强制写缓存", note: "" } }));
  assert.ok(forced.includes("group-cache"));
  // Non-Claude models never see Claude-only cards or the channel row.
  const gpt = render(verdict(), { model: "gpt-6-astra" });
  assert.ok(!gpt.includes("sh-channel"));
  assert.ok(!gpt.includes("思维链回传"));
});

test("a confirmed swap turns the hero red and takes the fingerprint points off the row", () => {
  const swapped = verdict({
    fingerprint: { ...verdict().fingerprint, top_model: "claude-haiku-4-5", top_prob: 0.9999 },
  });
  const html = render(swapped, { model: "claude-opus-4-6" });
  assert.match(html, /data-level="fake"/);
  assert.match(html, /<p class="sh-headline">疑似掺水<\/p>/);
  assert.match(html, /指纹显示实际是 claude-haiku-4-5（100\.0%）/);
  assert.match(html, /sh-number tone-bad/);
  assert.match(html, /<span>fingerprint<\/span><b>0\/21<\/b>/);
});

test("while the audit runs the hero slot is reserved; without a total on a finished report it is absent", () => {
  const running = render({ ...verdict(), total: null }, { loading: true });
  assert.match(running, /score-hero is-pending/);
  assert.match(running, /aria-busy="true"/);
  assert.ok(!render({ ...verdict(), total: null }).includes("score-hero"));
  // With probe counts the slot doubles as the progress readout.
  const counted = render({ ...verdict(), total: null }, { loading: true, progress: { done: 12, total: 26 } });
  assert.match(counted, /正在收集证据<span class="sh-count">12 \/ 26<\/span>/);
  assert.match(counted, /aria-valuenow="12"/);
  assert.match(counted, /transform:scaleX\(0\.4615/);
  // Zero probes (not created yet) falls back to the skeleton instead of dividing by zero.
  const empty = render({ ...verdict(), total: null }, { loading: true, progress: { done: 0, total: 0 } });
  assert.ok(!empty.includes("progressbar"));
});

test("Claude reports show the channel line, or tell the reader how to get it", () => {
  const without = render(verdict());
  assert.match(without, /sh-channel-label">Claude 渠道</);
  assert.match(without, /勾选「Claude 深度体检」/);
  const deep = verdict({
    claude_deep: {
      note: "",
      checks: [
        { name: "signature_fresh", title: "现签 / 复签", pass: true },
        { name: "hi_usage", title: "hi 用量指纹", pass: true },
        { name: "injection", title: "行为注入", pass: false, detail: "检出 Claude Code（ccmax）；NOTE=x" },
      ],
    },
  });
  const html = render(deep);
  assert.ok(!html.includes("勾选「Claude 深度体检」"));
  assert.match(html, /data-state="ok"><i>现签<\/i><b>是<\/b>/);
  assert.match(html, /data-state="no"><i>0 注入<\/i><b>否<\/b>/);
  assert.match(html, /data-state="warn"><i>ccmax<\/i><b>是<\/b>/);
  assert.match(html, /data-level="caution"/);
});

test("worst case: many findings are capped, hostile text stays inert, unknown sections show a dash", () => {
  const checks = Array.from({ length: 6 }, (_, i) => ({
    name: "c" + i, title: "检查" + i, pass: false, detail: i === 0 ? "<script>alert(1)</script>" : "x".repeat(300),
  }));
  const html = render(verdict({ claude_deep: { checks, note: "" } }));
  assert.equal((html.match(/data-tone="warn"/g) || []).length, 4);
  assert.match(html, /另有 2 项，见下方各卡片/);
  assert.ok(!html.includes("<script>alert(1)"));
  assert.ok(html.includes("&lt;script&gt;"));
  // The hero clips long upstream text; the full text lives in the card below.
  assert.ok(!html.slice(0, html.indexOf("group-identity")).includes("x".repeat(100)));
  const unknown = verdict();
  unknown.total.parts[1] = part("iq", 0, 28, { known: false });
  const dash = render(unknown);
  assert.match(dash, /data-state="none"[^>]*><span>iq<\/span><b>—<\/b>/);
});

test("a tampered-signature probe that hit an outage is not reported as accepted", () => {
  const outage = verdict({ thinking: { ...verdict().thinking, tamper_rejected: false, tamper_status: 503 } });
  const html = render(outage);
  assert.match(html, /<i>篡改<\/i><b class="skip">HTTP 503<\/b>/);
  assert.ok(!html.includes("被接受"));
  const accepted = verdict({ thinking: { ...verdict().thinking, tamper_rejected: false, tamper_status: 200 } });
  assert.match(render(accepted), /<i>篡改<\/i><b class="no">被接受<\/b>/);
});

test("the protocol checks are their own scored card, with the pass count and one row per probe", () => {
  const strictness = [
    { name: "missing_max_tokens", title: "缺 max_tokens", status: 400, pass: true },
    { name: "zero_max_tokens", title: "max_tokens=0", status: 200, pass: false },
    { name: "system_role", title: "system role 注入", status: 0, pass: false, skipped: true },
  ];
  const v = verdict({ thinking: { ...verdict().thinking, strictness } });
  v.total.parts.push(part("strictness", 6, 8));
  const html = render(v);
  assert.match(html, /<h3>协议校验<\/h3>/);
  assert.match(html, /<strong class="warn">1 \/ 2<\/strong>/);
  assert.match(html, /<i>缺 max_tokens<\/i><b class="ok">已拒绝<\/b>/);
  assert.match(html, /<i>max_tokens=0<\/i><b class="no">未拒绝 200<\/b>/);
  assert.match(html, /<i>system role 注入<\/i><b class="skip">跳过<\/b>/);
  assert.match(html, /score-chip[^>]*>6\/8<\/b>/);
  // The thinking card no longer carries a duplicate chip or fold for them.
  assert.ok(!html.includes("协议校验明细"));
  // No probes recorded, no card.
  assert.ok(!render(verdict()).includes("<h3>协议校验</h3>"));
});

test("deep audit and forced cache show their share of the total and the measured tiers", () => {
  const v = verdict({
    claude_deep: { note: "", checks: [{ name: "hi_usage", title: "hi 用量指纹", pass: true }] },
    force_cache: { steps: [], forced: false, tier_5m: true, tier_1h: false, conclusion: "透传", note: "" },
  });
  v.total.parts.push(part("deep", 8, 10), part("forcecache", 3, 4));
  const html = render(v);
  assert.match(html, /<h3>Claude 深度体检<\/h3><span class="rc-score"><b class="score-chip"[^>]*>8\/10<\/b>/);
  assert.match(html, /<h3>强制缓存检测<\/h3><span class="rc-score"><b class="score-chip"[^>]*>3\/4<\/b>/);
  assert.match(html, /<i>无参数写缓存<\/i><b class="ok">不会<\/b>/);
  assert.match(html, /<i>5 分钟档<\/i><b class="ok">生效<\/b>/);
  assert.match(html, /<i>1 小时档<\/i><b class="no">未生效<\/b>/);
  // A forced upstream has no tier measurements to show.
  const forced = render(verdict({ force_cache: { steps: [], forced: true, conclusion: "强制", note: "" } }));
  assert.match(forced, /<i>无参数写缓存<\/i><b class="no">会（强制）<\/b>/);
  assert.ok(!forced.includes("5 分钟档"));
});

test("anti-cheat verdict switches hero headline and renders dedicated evidence card", () => {
  const v = verdict({
    anticheat: {
      detected: true,
      confirmed_count: 1,
      penalized_parts: ["fingerprint"],
      summary: "检测到 1 项针对性作假证据",
      signals: [
        { code: "fp_static_replay", section: "fingerprint", severity: "confirmed", title: "数字指纹跨样本静态数组回放", detail: "numeric-1 与 numeric-2 出现连续 300 个相同数字" },
      ],
    },
  });
  v.total.parts[0] = part("fingerprint", 0, 21);
  v.total.score = 75;
  const html = render(v);
  assert.match(html, /data-level="fake"/);
  assert.match(html, /<p class="sh-headline">检测到针对性作假<\/p>/);
  assert.match(html, /class="result-card anticheat-card"/);
  assert.ok(!html.includes("/blacklist"));
  assert.match(html, /<span>fingerprint<\/span><b>0\/21<\/b>/);
});

test("blocked-domain zero-request verdict renders cleanly without throwing", () => {
  const blocked = {
    total: {
      score: 0, max: 100, reason: "",
      parts: [part("fingerprint", 0, 36), part("iq", 0, 24), part("vision", 0, 15), part("injection", 0, 13), part("tools", 0, 12)],
    },
    anticheat: {
      detected: true,
      confirmed_count: 1,
      penalized_parts: ["fingerprint", "iq", "vision", "injection", "tools", "web", "thinking", "cache"],
      summary: "目标域名命中已确认针对本站作假规则（jaaak.cc），未发起上游请求，直接按 0 分处置。",
      signals: [{ code: "domain_blocked_zero_score", section: "domain", severity: "confirmed", title: "已知针对性作假站点（直接判 0 分）", detail: "命中域名规则 jaaak.cc" }],
    },
    fingerprint: { basis: "numeric", family: "unknown", family_name: "命中作假站点规则", family_prob: 0, top_model: "", top_prob: 0, results: [], used_outputs: 0, low_confidence: false, evidence: [], note: "已跳过" },
    injection: { inj: 0, slope: 0, r2: null, max_residual: null, cache_header: "", score: 0, max_score: 10, reliable: false, note: "已跳过", points: [] },
    iq: { score: 0, max: 100, paper_max: 0, complete: true, per_question: [], note: "已跳过" },
    vision: { score: 0, max: 100, recognized: false, recognized_score: 0, recognized_max: 50, answer_score: 0, answer_max: 50, correct: false, note: "已跳过" },
    tools: { status: "blocked", pass: false, reason: "已跳过", protocol: "anthropic" },
    thinking: { pass: false, score: 0, max_score: 12, signature_present: false, think_len: 0, sig_len: 0, reason: "已跳过", thinking_text: "", signature: "", answer: "" },
  };
  const html = render(blocked);
  assert.match(html, /data-level="fake"/);
  assert.match(html, /站点规则/);
  assert.match(html, /已知针对性作假站点（直接判 0 分）/);
  assert.ok(!html.includes("/blacklist"));
});
