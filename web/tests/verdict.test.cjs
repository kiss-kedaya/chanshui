const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const vm = require("node:vm");
const ts = require("typescript");
const source = fs.readFileSync(path.join(__dirname, "../lib/report-view.ts"), "utf8");
const compiled = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.CommonJS } }).outputText;
const scope = { exports: {}, URL };
vm.runInNewContext(compiled, scope);
// Results are built inside the vm realm; a JSON round trip makes them plain
// objects so deepStrictEqual does not trip over foreign prototypes.
const plain = (value) => (value === undefined ? undefined : JSON.parse(JSON.stringify(value)));
const reportVerdict = (...args) => plain(scope.exports.reportVerdict(...args));
const channelChips = (...args) => plain(scope.exports.channelChips(...args));

const part = (key, contribution, coefficient, extra = {}) => ({
  key, label: key, applicable: true, known: true, points: 0, max_points: 1,
  weight: coefficient, coefficient, contribution, ...extra,
});
const clean = (over = {}) => ({
  total: {
    score: 96, max: 100, reason: "",
    parts: [part("fingerprint", 21, 21), part("iq", 28, 28), part("vision", 17, 17), part("injection", 15, 15), part("tools", 11, 11), part("thinking", 8, 8)],
  },
  fingerprint: { family: "claude", family_name: "Claude", family_prob: 1, top_model: "claude-opus-5-5", top_prob: 1, results: [], used_outputs: 3, low_confidence: false, note: "", evidence: [] },
  injection: { inj: 0, slope: 0, r2: null, max_residual: null, reliable: true, score: 10, max_score: 10, note: "", points: [], cache_header: "" },
  thinking: { pass: true, score: 12, max_score: 12, signature_present: true, original_replay: true, tamper_rejected: true, tamper_status: 400, think_len: 10, sig_len: 10, reason: "", thinking_text: "", signature: "", answer: "" },
  iq: { score: 100, max: 100, paper_max: 100, complete: true, per_question: [], note: "" },
  tools: { status: "pass", pass: true, reason: "", protocol: "anthropic" },
  ...over,
});
const texts = (r) => r.findings.map((f) => f.text);

test("a clean report is ok and green", () => {
  const r = reportVerdict(clean(), "match");
  assert.equal(r.level, "ok");
  assert.equal(r.tone, "ok");
  assert.deepEqual(r.findings, []);
  assert.equal(r.applicable, 6);
  assert.equal(r.scored, 6);
});

test("a report without a total has no verdict", () => {
  assert.equal(reportVerdict({ ...clean(), total: null }, "match"), null);
  assert.equal(reportVerdict({ ...clean(), total: undefined }, "match"), null);
});

test("a confirmed swap is fake whatever the score says", () => {
  const r = reportVerdict(clean({ fingerprint: { ...clean().fingerprint, top_model: "claude-haiku-4-5", top_prob: 0.9997 } }), "fake");
  assert.equal(r.level, "fake");
  assert.equal(r.tone, "bad");
  assert.deepEqual(r.findings[0], { tone: "bad", text: "指纹显示实际是 {model}（{p}%）", vars: { model: "claude-haiku-4-5", p: "100.0" } });
});

test("a suspicious name mismatch is caution, never fake", () => {
  const r = reportVerdict(clean(), "suspicious");
  assert.equal(r.level, "caution");
  assert.equal(r.tone, "warn");
  assert.equal(r.findings[0].tone, "warn");
});

test("hidden injection warns above 1000 tokens and goes red above 6000", () => {
  const injection = (score, inj, extra = {}) => ({ ...clean().injection, score, inj, ...extra });
  assert.deepEqual(texts(reportVerdict(clean({ injection: injection(8, 144) }), "match")), []);
  const warn = reportVerdict(clean({ injection: injection(6, 1500) }), "match");
  assert.equal(warn.findings[0].tone, "warn");
  assert.equal(warn.findings[0].vars.n, 1500);
  assert.equal(reportVerdict(clean({ injection: injection(2, 7000) }), "match").findings[0].tone, "bad");
  // An unreliable fit has no score: no finding, no guess.
  assert.deepEqual(texts(reportVerdict(clean({ injection: injection(null, 0, { reliable: false }) }), "match")), []);
  assert.deepEqual(texts(reportVerdict(clean({ injection: injection(0, 0, { error: "boom" }) }), "match")), ["{n} 项请求失败，按 0 分计入"]);
});

test("only a 200 on the tampered signature counts as accepted", () => {
  const thinking = (extra) => ({ ...clean().thinking, ...extra });
  const accepted = reportVerdict(clean({ thinking: thinking({ tamper_rejected: false, tamper_status: 200 }) }), "match");
  assert.deepEqual(texts(accepted), ["篡改过的签名被接受，渠道没校验签名"]);
  // 429 / 503 / timeout on the probe says nothing about signature checking.
  for (const status of [429, 503, 408, 0]) {
    assert.deepEqual(texts(reportVerdict(clean({ thinking: thinking({ tamper_rejected: false, tamper_status: status }) }), "match")), []);
  }
  // If the genuine signature could not even be replayed, tamper is not blamed.
  assert.deepEqual(
    texts(reportVerdict(clean({ thinking: thinking({ original_replay: false, tamper_rejected: false, tamper_status: 200 }) }), "match")),
    ["原始签名无法续轮"],
  );
  assert.deepEqual(texts(reportVerdict(clean({ thinking: thinking({ signature_present: false }) }), "match")), ["没有拿到思维链签名"]);
  assert.deepEqual(texts(reportVerdict(clean({ thinking: thinking({ signed_model_match: false }) }), "match")), ["签名里的模型名与声称不一致"]);
  assert.deepEqual(texts(reportVerdict(clean({ thinking: thinking({ sig_class: "未识别格式（320 字节）" }) }), "match")), ["签名结构异常：{cls}"]);
  assert.deepEqual(texts(reportVerdict(clean({ thinking: thinking({ sig_class: "标准信封（无时间戳，密文 692 字节）" }) }), "match")), []);
});

test("lenient protocol checks are a note, not a reason for caution", () => {
  const strict = (passes, total) => Array.from({ length: total }, (_, i) => ({ name: "c" + i, status: 200, pass: i < passes }));
  const thinking = (strictness) => ({ ...clean().thinking, strictness });
  const lenient = reportVerdict(clean({ thinking: thinking(strict(2, 10)) }), "match");
  assert.equal(lenient.level, "ok");
  assert.deepEqual(lenient.findings, [{ tone: "note", text: "协议校验 {a}/{b}：渠道对非法请求偏宽松", vars: { a: 2, b: 10 } }]);
  assert.deepEqual(texts(reportVerdict(clean({ thinking: thinking(strict(8, 10)) }), "match")), []);
  // Skipped checks are not failures.
  const skipped = strict(0, 4).map((c) => ({ ...c, skipped: true }));
  assert.deepEqual(texts(reportVerdict(clean({ thinking: thinking(skipped) }), "match")), []);
});

test("deep audit failures warn, skips stay silent, beta features are only notes", () => {
  const deep = (checks) => ({ checks, note: "" });
  const check = (name, pass, extra = {}) => ({ name, title: name + "-title", pass, ...extra });
  const warned = reportVerdict(clean({ claude_deep: deep([check("hi_usage", false, { detail: "input_tokens=12 高于参考 10，疑似注入" })]) }), "match");
  assert.equal(warned.level, "caution");
  assert.deepEqual(warned.findings[0].vars, { title: "hi_usage-title", detail: "input_tokens=12 高于参考 10，疑似注入" });
  assert.deepEqual(texts(reportVerdict(clean({ claude_deep: deep([check("hi_usage", false, { skipped: true })]) }), "match")), []);
  const soft = reportVerdict(clean({ claude_deep: deep([check("context_compact", false), check("thinking_binding", false)]) }), "match");
  assert.equal(soft.level, "ok");
  assert.deepEqual(soft.findings.map((f) => f.tone), ["note", "note"]);
  const long = reportVerdict(clean({ claude_deep: deep([check("injection", false, { detail: "x".repeat(200) })]) }), "match");
  assert.equal(long.findings[0].vars.detail.length, 61);
  assert.equal(reportVerdict(clean({ force_cache: { steps: [], forced: true, conclusion: "", note: "" } }), "match").level, "caution");
  assert.equal(reportVerdict(clean({ force_cache: { steps: [], forced: false, conclusion: "", note: "" } }), "match").level, "ok");
});

test("weak reasoning, tools and cache findings", () => {
  assert.deepEqual(texts(reportVerdict(clean({ iq: { ...clean().iq, score: 60 } }), "match")), ["推理表现偏低：{n}/100"]);
  assert.deepEqual(texts(reportVerdict(clean({ iq: { ...clean().iq, score: 61 } }), "match")), []);
  assert.deepEqual(texts(reportVerdict(clean({ tools: { ...clean().tools, pass: false } }), "match")), ["工具调用没通过"]);
  assert.deepEqual(texts(reportVerdict(clean({ tools: { ...clean().tools, pass: null } }), "match")), []);
  assert.deepEqual(texts(reportVerdict(clean({ cache: { status: "mismatch", pass: false, reason: "", first: null, second: null, expected_read: 0, actual_read: 0, delta: 0, protocol: "" } }), "match")), ["缓存读写没有衔接"]);
});

test("findings are ordered worst first", () => {
  const r = reportVerdict(
    clean({
      thinking: { ...clean().thinking, signature_present: false, strictness: [{ name: "a", status: 200, pass: false }] },
      injection: { ...clean().injection, score: 2, inj: 9000 },
    }),
    "match",
  );
  assert.deepEqual(r.findings.map((f) => f.tone), ["bad", "warn", "note"]);
});

test("many failed probes make the score untrustworthy instead of damning", () => {
  const err = { error: "HTTP 503" };
  const r = reportVerdict(
    clean({
      fingerprint: { ...clean().fingerprint, ...err },
      iq: { ...clean().iq, score: 0, per_question: [{ id: "Q1", error: "x" }, { id: "Q2", error: "x" }, { id: "Q3", skipped: true }] },
    }),
    "match",
  );
  assert.equal(r.level, "unknown");
  assert.equal(r.tone, "muted");
  // Answers that never came back are not "weak reasoning".
  assert.ok(!texts(r).includes("推理表现偏低：{n}/100"));
  assert.equal(r.failed, 2);
  assert.equal(r.scored, 4);
  assert.ok(texts(r).includes("{n} 项请求失败，按 0 分计入"));
  // One failed small section is not enough to give up on the report.
  const small = reportVerdict(clean({ tools: { ...clean().tools, error: "x" } }), "match");
  assert.equal(small.level, "ok");
  assert.equal(small.failed, 1);
  // A confirmed swap is still reported as fake even if other probes failed.
  assert.equal(reportVerdict(clean({ iq: { ...clean().iq, per_question: [{ id: "Q1", error: "x" }] } }), "fake").level, "fake");
});

test("an untrustworthy score is grey even when it is low", () => {
  const broke = clean({
    total: { ...clean().total, score: 31 },
    tools: { ...clean().tools, error: "x" },
    fingerprint: { ...clean().fingerprint, error: "x" },
    iq: { ...clean().iq, score: 0, per_question: [{ id: "Q1", error: "x" }] },
  });
  const r = reportVerdict(broke, "match");
  assert.equal(r.level, "unknown");
  assert.equal(r.tone, "muted");
});

test("a low score is red even when nothing specific is wrong", () => {
  const r = reportVerdict(clean({ total: { ...clean().total, score: 59 } }), "match");
  assert.equal(r.tone, "bad");
  assert.equal(r.level, "ok");
  assert.equal(reportVerdict(clean({ total: { ...clean().total, score: 84 } }), "match").tone, "ink");
  assert.equal(reportVerdict(clean({ total: { ...clean().total, score: 85 } }), "match").tone, "ok");
});

test("old or odd reports do not crash the verdict", () => {
  const bare = { total: { score: 73, max: 100 }, injection: null, hidden: null, fingerprint: null, thinking: null, iq: null };
  const r = reportVerdict(bare, "match");
  assert.equal(r.level, "ok");
  assert.equal(r.applicable, 0);
  assert.deepEqual(r.findings, []);
  assert.doesNotThrow(() => reportVerdict({ ...bare, total: { score: NaN, max: 100, parts: [] }, iq: { score: null, per_question: null } }, "suspicious"));
});

const deep = (...checks) => ({ claude_deep: { checks, note: "" } });
const ck = (name, pass, extra = {}) => ({ name, title: name, pass, ...extra });
const chip = (list, key) => list.find((item) => item.key === key);

test("no Claude deep data means no channel chips", () => {
  assert.deepEqual(channelChips({}), []);
  assert.deepEqual(channelChips({ thinking: { think_len: 0 } }), []);
});

test("fresh signature chip follows the two-signature check", () => {
  assert.deepEqual(chip(channelChips(deep(ck("signature_fresh", true))), "fresh"), { key: "fresh", label: "现签", value: "是", state: "ok" });
  assert.equal(chip(channelChips(deep(ck("signature_fresh", false))), "fresh").value, "复签");
  assert.equal(chip(channelChips(deep(ck("signature_fresh", false, { skipped: true }))), "fresh").state, "unsure");
});

test("zero injection needs both the usage reference and the behaviour probe", () => {
  const zero = (...checks) => chip(channelChips(deep(...checks)), "zero");
  assert.equal(zero(ck("hi_usage", true), ck("injection", true)).value, "是");
  assert.equal(zero(ck("hi_usage", false), ck("injection", true)).value, "否");
  assert.equal(zero(ck("hi_usage", true), ck("injection", false)).value, "否");
  // Below the reference is not proof of zero injection.
  assert.equal(zero(ck("hi_usage", false, { skipped: true }), ck("injection", true)).value, "不定");
  assert.equal(zero(ck("injection", true)).value, "不定");
  assert.equal(zero(ck("hi_usage", true), ck("injection", false, { skipped: true })).value, "不定");
});

test("ccmax, Agent SDK and claude.ai come from the detected prompt", () => {
  const kind = (detail, extra = {}) => chip(channelChips(deep(ck("injection", extra.pass ?? false, { detail, ...extra }))), "ccmax");
  assert.equal(kind("检出 Claude Code（ccmax）；NOTE=…").value, "是");
  assert.equal(kind("检出 Agent SDK；NOTE=…").value, "Agent SDK");
  assert.equal(kind("检出 claude.ai 系统提示；NOTE=…").value, "claude.ai");
  assert.deepEqual(kind("未检出固定系统提示；NOTE=NONE", { pass: true }), { key: "ccmax", label: "ccmax", value: "否", state: "info" });
  assert.equal(kind("四问都被上游拒绝", { skipped: true }).state, "unsure");
});

test("AWS-B uses the bad-signature response, and a 200 means no", () => {
  assert.equal(chip(channelChips({ thinking: { aws_b: true } }), "awsb").value, "是");
  assert.equal(chip(channelChips({ thinking: { aws_b: false } }), "awsb").value, "否");
  assert.equal(chip(channelChips({ thinking: { tamper_status: 200 } }), "awsb").value, "否");
  assert.equal(chip(channelChips({ thinking: { tamper_status: 503 } }), "awsb"), undefined);
});

test("system passthrough and forced cache chips", () => {
  assert.equal(chip(channelChips(deep(ck("system_passthrough", true))), "system").state, "ok");
  assert.equal(chip(channelChips(deep(ck("system_passthrough", false))), "system").state, "no");
  assert.equal(chip(channelChips(deep(ck("system_passthrough", false, { skipped: true }))), "system").value, "不定");
  const force = (forced) => chip(channelChips({ force_cache: { steps: [], forced, conclusion: "", note: "" } }), "force");
  assert.deepEqual(force(true), { key: "force", label: "强制缓存", value: "强制", state: "no" });
  assert.equal(force(false).value, "透传");
  assert.equal(force(undefined).value, "不定");
});

test("chips follow the order of the channel spec", () => {
  const all = channelChips({
    ...deep(ck("signature_fresh", true), ck("hi_usage", true), ck("injection", true, { detail: "ok" }), ck("system_passthrough", true)),
    thinking: { aws_b: true },
    force_cache: { steps: [], forced: false, conclusion: "", note: "" },
  });
  assert.deepEqual(all.map((c) => c.key), ["fresh", "zero", "ccmax", "awsb", "system", "force"]);
});
