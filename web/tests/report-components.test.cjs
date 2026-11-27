const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const vm = require("node:vm");
const ts = require("typescript");
const React = require("react");
const { renderToStaticMarkup } = require("react-dom/server");
const appRequire = require("./app-require.cjs");

function loadWithMockedI18n(filename, lang = "zh", extraImports = {}) {
  const i18n = appRequire("@/lib/i18n");
  const mockI18n = {
    ...i18n,
    useI18n: () => ({
      lang,
      follow: false,
      locale: i18n.intlLocale(lang),
      t: (source, vars) => i18n.translate(lang, source, vars),
      setLang: () => {},
    }),
  };
  const source = fs.readFileSync(path.join(__dirname, filename), "utf8");
  const compiled = ts.transpileModule(source, {
    compilerOptions: {
      module: ts.ModuleKind.CommonJS,
      jsx: ts.JsxEmit.ReactJSX,
      esModuleInterop: true,
    },
  }).outputText;
  const scope = {
    exports: {},
    module: { exports: {} },
    require: (name) => {
      if (name === "@/lib/i18n") return mockI18n;
      if (extraImports[name]) return extraImports[name];
      return appRequire(name);
    },
    URL,
    URLSearchParams,
  };
  scope.module.exports = scope.exports;
  vm.runInNewContext(compiled, scope);
  return scope.module.exports;
}

const part = (key, contribution, coefficient, extra = {}) => ({
  key,
  label: key,
  applicable: true,
  known: true,
  points: 0,
  max_points: 1,
  weight: coefficient,
  coefficient,
  contribution,
  ...extra,
});

test("ScoreHero renders 270deg SVG gauge across all 4 verdict levels and progress ring in pending state", () => {
  const ScoreHero = loadWithMockedI18n("../components/ScoreHero.tsx", "zh").default;
  for (const [level, tone, score, fp, extra] of [
    ["ok", "ok", 92, "match", {}],
    ["caution", "warn", 88, "suspicious", {}],
    ["fake", "bad", 45, "fake", {}],
    [
      "unknown",
      "muted",
      20,
      "match",
      {
        fingerprint: { error: "timeout" },
        injection: { error: "timeout" },
      },
    ],
  ]) {
    const html = renderToStaticMarkup(
      React.createElement(ScoreHero, {
        verdict: {
          total: {
            score,
            max: 100,
            reason: "",
            parts: [part("fingerprint", 21, 21), part("injection", 15, 15)],
          },
          ...extra,
        },
        fp,
        claude: false,
        loading: false,
      }),
    );
    assert.match(html, new RegExp(`data-level="${level}"`));
    assert.match(html, new RegExp(`class="sh-gauge" data-gauge-tone="${tone}"`));
  }

  const settled = renderToStaticMarkup(
    React.createElement(ScoreHero, {
      verdict: {
        total: {
          score: 92,
          max: 100,
          reason: "",
          parts: [part("fingerprint", 21, 21), part("iq", 28, 28)],
        },
      },
      fp: "match",
      claude: false,
      loading: false,
    }),
  );
  assert.match(settled, /class="sh-gauge-svg"/);
  assert.match(settled, /class="sh-arc-track"/);
  assert.match(settled, /class="sh-arc-ticks"/);
  assert.match(settled, /class="sh-arc-fill"/);
  assert.match(settled, /class="sh-number tone-ok"/);
  assert.match(settled, /<i class="is-pill" aria-hidden="true"><\/i>/);

  const pending = renderToStaticMarkup(
    React.createElement(ScoreHero, {
      verdict: { total: null },
      fp: "match",
      claude: false,
      loading: true,
      progress: { done: 3, total: 6 },
    }),
  );
  assert.match(pending, /class="sh-pending-ring"/);
  assert.match(pending, /正在收集证据<span class="sh-count">3 \/ 6<\/span>/);
  assert.match(pending, /transform:scaleX\(0\.5\)/);
});

test("ProbeTimeline exposes data-status, continuous 44x44 SVG progress ring, and round probe dots", () => {
  const ProbeTimeline = loadWithMockedI18n("../components/ProbeTimeline.tsx", "en").default;
  const html = renderToStaticMarkup(
    React.createElement(ProbeTimeline, {
      status: "partial",
      probes: [
        { id: "fp", name: "模型指纹归因", status: "done", ms: 1200 },
        { id: "Q1", name: "安眠药", status: "error", ms: 400, error: "timeout" },
      ],
    }),
  );
  assert.match(html, /data-status="partial"/);
  assert.match(html, /class="timeline-symbol is-round "/);
  assert.match(html, /class="timeline-ring" viewBox="0 0 44 44"/);
  assert.match(html, /class="timeline-ring-fill"/);
  assert.match(html, /class="probe-dot is-round"/);
  assert.match(html, /Report ready, some requests failed/);
});

test("ReportStage, ReportShare, and ResultSkeleton render clean geometry, labels, and zero-shift status slot", () => {
  const ReportStage = loadWithMockedI18n("../components/ReportStage.tsx", "en").default;
  const stageHtml = renderToStaticMarkup(React.createElement(ReportStage));
  assert.match(stageHtml, /class="stage-emblem is-round"/);
  assert.match(stageHtml, /class="stage-emblem-svg"/);
  assert.match(stageHtml, /Public audit report/);

  const ReportShare = loadWithMockedI18n("../components/ReportShare.tsx", "en").default;
  const shareHtml = renderToStaticMarkup(
    React.createElement(ReportShare, { id: "a1".repeat(16), expiresAt: 1795780800 }),
  );
  assert.match(shareHtml, /type="button"/);
  assert.match(shareHtml, />Copy ID</);
  assert.match(shareHtml, />Copy link</);
  assert.match(shareHtml, />Standalone</);
  assert.match(shareHtml, /class="report-share-status" data-visible="false" role="status"/);

  const ResultSkeleton = loadWithMockedI18n("../components/ResultSkeleton.tsx", "en").default;
  const skelHtml = renderToStaticMarkup(React.createElement(ResultSkeleton, { label: "暂无结果" }));
  assert.match(skelHtml, /class="result-skeleton" aria-busy="true"/);
  assert.match(skelHtml, /No result yet/);
});

test("ResultCards renders .fp-claim-band, pill tags, .force-cache-rows, and translates backend notes in en", () => {
  const ConvPanelEn = loadWithMockedI18n("../components/ConversationPanel.tsx", "en");
  const ResultCardsEn = loadWithMockedI18n("../components/ResultCards.tsx", "en", {
    "@/components/ConversationPanel": ConvPanelEn,
  }).default;

  const html = renderToStaticMarkup(
    React.createElement(ResultCardsEn, {
      model: "claude-opus-4-6",
      verdict: {
        total: {
          score: 70,
          max: 100,
          reason: "",
          parts: [part("fingerprint", 0, 36), part("iq", 24, 24)],
        },
        anticheat: {
          detected: true,
          confirmed_count: 1,
          summary: "检测到 1 项针对性作假证据（回放），对应板块已按 0 分计入。",
          signals: [{ code: "c1", section: "thinking", severity: "confirmed", title: "检测到伪造思维链签名", detail: "detail" }],
        },
        fingerprint: {
          family: "gpt",
          family_name: "GPT",
          family_prob: 0.99,
          top_model: "gpt-5.4",
          top_prob: 1,
          used_outputs: 3,
          note: "有效样本不足 3 个，这项记 0 分。",
          results: [],
        },
        thinking: {
          pass: false,
          score: 0,
          max_score: 12,
          signature_present: true,
          sig_class: "伪造签名（密文 64 字节）",
          original_replay: false,
          tamper_rejected: true,
          think_len: 10,
          sig_len: 64,
          reason: "没有签名；只拿到思维摘要。",
          thinking_text: "",
          signature: "",
          answer: "",
        },
        claude_deep: {
          checks: [],
          note: "深度体检跑渠道指纹（hi 用量、行为注入、现签/复签、system 透传）加 beta 项（thinking 绑定、上下文压缩）。签名解码和协议严格性在常规检测里。",
        },
        force_cache: {
          steps: [{ key: "plain", label: "无 cache_control", status: 200, input: 100, write_5m: 0, write_1h: 0, read: 0 }],
          forced: false,
          tier_5m: true,
          tier_1h: false,
          conclusion: "透传：第 1 步不写不读；5 分钟档生效；1 小时档没有按参数写入",
          note: "每步换唯一 nonce，正文约 8000 token，max_tokens=16。无 cache_control 时 input 很小却出现大额 cache write/read = 强制写缓存；正常透传应是大额 input、write=0、read=0。",
        },
        cache: {
          status: "unknown",
          pass: null,
          reason: "上游没有缓存读取，无法判断。",
          first: null,
          second: null,
          expected_read: 0,
          actual_read: 0,
          delta: 0,
          protocol: "anthropic",
        },
        iq: {
          score: 80,
          max: 100,
          paper_max: 100,
          complete: true,
          note: "",
          per_question: [
            {
              id: "Q8",
              title: "糖果变体",
              max: 16,
              earned: 16,
              half: false,
              tags: ["最坏情况保证", "参数化", "库存:7,9,8:7,6,4"],
              prompt: "p",
              expected: "21",
              answer: "21",
              thinking_text: "",
              signature: "",
              ms: 100,
              metrics: { InputTokens: 10, OutputTokens: 5, CacheRead: 0, CacheCreation5m: 0, CacheCreation1h: 0, Status: 200, Ms: 100, Err: "", Answer: "21", ThinkingText: "", Signature: "", ThinkingTokens: 0, StopReason: "end_turn" },
            },
          ],
        },
      },
    }),
  );

  assert.match(html, /class="fp-claim-band has-seal"/);
  assert.match(html, /class="fake-seal is-round"/);
  assert.match(html, /class="question-tags"/);
  assert.match(html, /class="question-tag is-pill">Worst-case guarantee</);
  assert.match(html, /class="question-tag is-pill">Stock:7,9,8:7,6,4</);
  assert.match(html, /class="check-rows force-cache-rows"/);
  assert.match(html, /Detected 1 targeted anti-cheat signal/);
  assert.match(html, /Forged sig/);
  assert.match(html, /No signature returned; only the thinking summary arrived\./);
  assert.match(html, /Deep audit checks channel fingerprints/);
  assert.match(html, /Passthrough: step 1 neither writes nor reads cache; 5-minute tier honored; 1-hour tier ignored/);
  assert.match(html, /Upstream returned no cache reads; inconclusive\./);
});

test("ConversationPanel renders interactive chain, auto-selected break inspector, scroll hint, and en translation", () => {
  const ConversationPanel = loadWithMockedI18n("../components/ConversationPanel.tsx", "en").default;
  const html = renderToStaticMarkup(
    React.createElement(ConversationPanel, {
      data: {
        rounds: 3,
        hits: 1,
        drops: 1,
        status: "mismatch",
        pass: false,
        reason: "1 轮未接上前缀，疑似中途换号。",
        turns: [
          { index: 1, kind: "opener", read: 0, write: 1000, input: 20, output: 10, hit_rate: 0, dropped: false, ms: 300 },
          { index: 2, kind: "question", id: "Q1", title: "安眠药", read: 1000, write: 200, input: 25, output: 12, hit_rate: 81.6, expected: 1000, dropped: false, ms: 320 },
          { index: 3, kind: "smalltalk", read: 0, write: 1200, input: 30, output: 14, hit_rate: 0, expected: 1200, dropped: true, ms: 340 },
        ],
      },
    }),
  );

  assert.match(html, /class="conv-chain" role="group" aria-label="Multi-turn cache chain"/);
  assert.match(html, /<button type="button" class="conv-chain-seg is-round is-break is-selected"/);
  assert.match(html, /class="conv-turn-detail" role="status"/);
  assert.match(html, /#3 · Smalltalk/);
  assert.match(html, /aria-label="Hide turn details"/);
  assert.match(html, /class="conv-scroll-hint">Swipe horizontally to view full table</);
  assert.match(html, /class="table-wrap conv-table-wrap" data-scroll="none"/);
  assert.match(html, /1 turn\(s\) missed the prefix; suspected mid-thread route\/account switch\./);
});

test("history/page.tsx renders boot and index loading states without throwing", () => {
  const HistoryDetail = loadWithMockedI18n("../app/history/page.tsx", "en", {
    "next/navigation": {
      useRouter: () => ({ push: () => {} }),
      useSearchParams: () => new URLSearchParams(""),
    },
    "next/link": {
      __esModule: true,
      default: ({ href, children, ...rest }) => React.createElement("a", { href, ...rest }, children),
    },
  }).default;

  const html = renderToStaticMarkup(React.createElement(HistoryDetail));
  assert.match(html, /Public audit report/);
  assert.match(html, /class="report-index" aria-label="Recent audits" aria-busy="true"/);

  const indexStates = [
    {
      name: "error",
      states: [null, "", "", false, "", "", 0, [], "network fail", 0],
      check: /class="report-index-error"[\s\S]*network fail[\s\S]*>Reload<\/button>/,
    },
    {
      name: "empty",
      states: [null, "", "", false, "", "", 0, [], "", 0],
      check: /class="report-index-empty"[\s\S]*No audits yet[\s\S]*New audit<\/a>/,
    },
    {
      name: "populated",
      states: [
        null,
        "",
        "",
        false,
        "",
        "",
        0,
        [{ report_id: "b2".repeat(16), id: 7, created_at: "2026-11-27T12:30:00Z", model: "claude-opus-4-6", iq_score: 95, total_score: 96, family: "claude", thinking_pass: true }],
        "",
        0,
      ],
      check: /href="\/history\?id=b2b2[\s\S]*claude-opus-4-6/,
    },
    {
      name: "detail-export",
      id: "a1".repeat(16),
      states: [
        {
          status: "done",
          expires_at: 1795780800,
          probes: [],
          id: 9,
          report_id: "a1".repeat(16),
          created_at: "2026-11-27T12:30:00Z",
          model: "claude-opus-4-6",
          verdict_json: {},
        },
        "",
        "a1".repeat(16),
        false,
        "",
        "a1".repeat(16),
        0,
      ],
      check: /class="report-actions"[\s\S]*>Export report<\/span>/,
    },
  ];

  for (const scenario of indexStates) {
    let idx = 0;
    const mockReact = {
      ...React,
      useState: (init) => [
        idx < scenario.states.length ? scenario.states[idx++] : typeof init === "function" ? init() : init,
        () => {},
      ],
    };
    const HistoryScenario = loadWithMockedI18n("../app/history/page.tsx", "en", {
      react: mockReact,
      "react/jsx-runtime": require("react/jsx-runtime"),
      "next/navigation": {
        useRouter: () => ({ push: () => {} }),
        useSearchParams: () => new URLSearchParams(scenario.id ? `id=${scenario.id}` : ""),
      },
      "next/link": {
        __esModule: true,
        default: ({ href, children, ...rest }) => React.createElement("a", { href, ...rest }, children),
      },
    }).default;
    const out = renderToStaticMarkup(React.createElement(HistoryScenario));
    assert.match(out, scenario.check, `Scenario ${scenario.name} failed`);
    assert.doesNotMatch(out, /Retry failed/, `Scenario ${scenario.name} must not expose Retry`);
  }
});

test("ScoreHero in en translates protocol strictness, forged/cached sig class, and item.vars title", () => {
  const i18n = appRequire("@/lib/i18n");
  const HintProbe = {
    __esModule: true,
    default: ({ label, text }) =>
      React.createElement("span", {
        "data-hint-label": i18n.translate("en", label),
        "data-hint-text": i18n.translate("en", text),
      }),
  };
  const ScoreHeroEn = loadWithMockedI18n("../components/ScoreHero.tsx", "en", {
    "@/components/Hint": HintProbe,
  }).default;
  const html = renderToStaticMarkup(
    React.createElement(ScoreHeroEn, {
      model: "claude-opus-4-6",
      fp: "match",
      claude: true,
      loading: false,
      verdict: {
        total: {
          score: 40,
          max: 100,
          reason: "",
          parts: [part("fingerprint", 36, 36), part("thinking", 0, 7)],
        },
        anticheat: {
          detected: true,
          confirmed_count: 1,
          signals: [{ code: "thinking_forged_sig", section: "thinking", severity: "confirmed", title: "伪造思维链签名", detail: "bad sig" }],
        },
        thinking: {
          pass: false,
          score: 0,
          max_score: 10,
          signature_present: true,
          sig_class: "历史/缓存签(偏 900s)",
          original_replay: true,
          tamper_rejected: true,
          think_len: 10,
          sig_len: 10,
          reason: "",
          thinking_text: "",
          signature: "",
          answer: "",
          strictness: [
            { name: "missing_max_tokens", title: "缺 max_tokens", status: 200, pass: false },
            { name: "zero_max_tokens", title: "max_tokens=0", status: 200, pass: false },
          ],
        },
        claude_deep: {
          note: "",
          checks: [{ name: "injection", title: "行为注入", pass: false, detail: "detected" }],
        },
      },
    }),
  );

  assert.match(html, /Forged thinking signature: bad sig/);
  assert.match(html, /Unusual signature structure: Cached/);
  assert.match(html, /Behavioral injection: detected/);
  assert.match(html, /Protocol checks 0\/2: the channel is lenient with invalid requests/);
  assert.match(html, /data-hint-label="About the total"/);
  assert.doesNotMatch(html, /[\u4e00-\u9fff]/);
});

test("ResultCards in en translates multi-sentence fingerprint.note, iq.note, anticheat title, knowledge.reason, thinking.reason, web.reason, and behavior_note", () => {
  const i18n = appRequire("@/lib/i18n");
  const ConvPanelEn = loadWithMockedI18n("../components/ConversationPanel.tsx", "en");
  const HintProbe = {
    __esModule: true,
    default: ({ label, text }) =>
      React.createElement("span", {
        "data-hint-label": i18n.translate("en", label),
        "data-hint-text": i18n.translate("en", text),
      }),
  };
  const ScoreHeroEn = loadWithMockedI18n("../components/ScoreHero.tsx", "en", {
    "@/components/Hint": HintProbe,
  });
  const ResultCardsEn = loadWithMockedI18n("../components/ResultCards.tsx", "en", {
    "@/components/ConversationPanel": ConvPanelEn,
    "@/components/ScoreHero": ScoreHeroEn,
    "@/components/Hint": HintProbe,
  }).default;

  const html = renderToStaticMarkup(
    React.createElement(ResultCardsEn, {
      model: "claude-opus-4-6",
      verdict: {
        total: {
          score: 50,
          max: 100,
          reason: "",
          parts: [part("fingerprint", 36, 36), part("iq", 14, 24)],
        },
        anticheat: {
          detected: true,
          confirmed_count: 1,
          summary: "检测到 1 项针对性作假证据（伪造思维链签名），对应板块已按 0 分计入。",
          signals: [{ code: "thinking_forged_sig", section: "thinking", severity: "confirmed", title: "伪造思维链签名", detail: "检测到非官方伪造思维链签名" }],
        },
        fingerprint: {
          family: "claude",
          family_name: "Claude",
          family_prob: 0.99,
          top_model: "claude-opus-4-6",
          top_prob: 0.99,
          used_outputs: 5,
          low_confidence: true,
          note: "概率只在数字库内的候选之间比较。从 3 个有效样本开始，顶部概率达到 90% 就停；否则最多取 10 个样本。归因和所选模型一致，这项才记满分。 所选名称不是库内 ID，已按同家族候选直接比对。 此候选为单一接口采样基线；候选权重沿用原库温度，未完成跨供应商概率校准，不能单独证明模型身份。",
          behavioral_top3: [{ model_id: "claude-opus-4-6", display_name: "Claude Opus 4.6", family: "claude", similarity: 0.92 }],
          behavior_note: "行为库没有该家族候选，未启用行为比对",
          results: [],
        },
        thinking: {
          pass: true,
          score: 10,
          max_score: 10,
          signature_present: true,
          sig_class: "真签(现生成)",
          original_replay: true,
          tamper_rejected: true,
          think_len: 10,
          sig_len: 10,
          reason: "摘要 5/5；签名 2/2；原始续轮 1/1；篡改拒绝 2/2；签名结构 真签(现生成)（14 字段，infra_aws）；协议严格性 9/10（未拒绝：system_role）。",
          thinking_text: "",
          signature: "",
          answer: "",
          strictness: [{ name: "system_role", title: "system role 注入", status: 200, pass: false }],
        },
        web: {
          status: "pass",
          pass: true,
          protocol: "deepseek_search",
          reason: "DeepSeek Search API 返回 3 条有效结果，含标题、摘要和来源链接。",
        },
        knowledge: {
          status: "estimated",
          cutoff: "2026-01-01",
          correct: 2,
          total: 4,
          label: "知识≈2026-01-01 至 2026-03-01（4 问）",
          reason: "按 2025–2026 客观事件串行二分估计知识边界，不参与评分。有 2 题结果对不上，未用它们移动边界。答对了 5 天前的事件（Event X），疑似联网。",
          gap_note: "有 2 题结果对不上，未用它们移动边界",
          web_suspect: "答对了 5 天前的事件（Event X），疑似联网",
          rows: [],
        },
        force_cache: {
          steps: [],
          forced: false,
          conclusion: "透传：第 1 步不写不读；样本介于两者之间：input=10 write5m=0 write1h=0 read=0",
          note: "",
        },
        iq: {
          score: 80,
          max: 100,
          paper_max: 100,
          complete: false,
          note: "当前卷面 100；半分仅在卷面展示，不进入百分制通过分。失败请求标记未完成，但已答题目照常计分，不伪装为模型答错。",
          per_question: [],
        },
      },
    }),
  );

  assert.match(html, /<strong>Forged thinking signature<\/strong>/);
  assert.match(html, /Unofficial forged thinking signature detected/);
  assert.match(html, /Probabilities are compared only among candidates in the numeric bank\./);
  assert.match(html, /Selected name is not a bank ID; compared directly against same-family candidates\./);
  assert.match(html, /Single-endpoint baseline; candidate weights keep original temperature/);
  assert.match(html, /No candidates for this family in the behavior bank; behavior comparison stayed off/);
  assert.match(html, /Current paper max 100; half credits are shown on the sheet only/);
  assert.match(html, /Summary 5\/5; Signature 2\/2; Original replay 1\/1; Tamper rejection 2\/2; Signature structure: Genuine \(14 fields, infra_aws\); Protocol strictness 9\/10 \(accepted: system_role\)/);
  assert.match(html, /DeepSeek Search API returned 3 valid result\(s\) with title, snippet, and source URL\./);
  assert.match(html, /Estimates knowledge boundary via serial binary search.*2 question\(s\) were inconsistent and excluded from shifting the boundary\. Answered event from 5 day\(s\) ago \(Event X\); suspected web access\./);
  assert.match(html, /Passthrough: step 1 neither writes nor reads cache; Mixed sample: input=10 write5m=0 write1h=0 read=0/);
  assert.match(html, /class="check-rows strict-check-rows"/);
  const visibleTextHtml = html.replace(/<pre>[\s\S]*?<\/pre>/g, "").replace(/\s+title="[^"]*"/g, "");
  assert.doesNotMatch(visibleTextHtml, /[\u4e00-\u9fff]/);
});

test("ReportShare with expiresAt=0 does not render literal 0, and report.css contains cascade & geometry fixes", () => {
  const ReportShare = loadWithMockedI18n("../components/ReportShare.tsx", "en").default;
  const zeroHtml = renderToStaticMarkup(React.createElement(ReportShare, { id: "a1".repeat(16), expiresAt: 0 }));
  assert.doesNotMatch(zeroHtml, /<small/);
  assert.doesNotMatch(zeroHtml, />0</);

  const reportCss = fs.readFileSync(path.join(__dirname, "../app/styles/report.css"), "utf8");
  assert.match(reportCss, /\.conv-chain-seg\.is-ok/);
  assert.match(reportCss, /\.timeline-symbol\.is-working \.timeline-ring\s*\{\s*animation:\s*none;/);
  assert.match(reportCss, /\.model-claim span\s*\{[\s\S]*width:\s*auto;/);
  assert.match(reportCss, /\.fp-claim-band\.has-seal \+ \.basis-line\s*\{\s*margin-top:\s*6px;/);
});
