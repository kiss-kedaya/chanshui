const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const vm = require("node:vm");
const ts = require("typescript");
const React = require("react");
const { renderToStaticMarkup } = require("react-dom/server");

function loadModule(filename, imports = {}) {
  const source = fs.readFileSync(path.join(__dirname, filename), "utf8");
  const compiled = ts.transpileModule(source, {
    compilerOptions: { module: ts.ModuleKind.CommonJS, jsx: ts.JsxEmit.ReactJSX },
  }).outputText;
  const scope = {
    exports: {},
    require: (name) => imports[name] || require("./app-require.cjs")(name),
  };
  vm.runInNewContext(compiled, scope);
  return scope.exports;
}

test("history entries do not expose thinking summary status", () => {
  const source = fs.readFileSync(
    path.join(__dirname, "../components/Sidebar.tsx"),
    "utf8",
  );
  assert.doesNotMatch(source, /思考待定|摘要已回传|摘要未回传/);
});

test("thinking card does not render delivery status summaries", () => {
  const ResultCards = loadModule("../components/ResultCards.tsx", {
    "@/components/AnimatedNumber": { default: ({ value }) => value },
    "@/components/ResultSkeleton": { default: () => null },
    "@/components/Hint": { default: () => null },
  "@/components/ErrorFold": loadModule("../components/ErrorFold.tsx"),
    "@/lib/report-view": loadModule("../lib/report-view.ts"),
  }).default;
  const html = renderToStaticMarkup(React.createElement(ResultCards, {
    model: "claude-opus-5",
    verdict: {
      thinking: {
        pass: true,
        reason: "思考文本正常回传",
        think_len: 16,
        sig_len: 24,
        thinking_text: "visible thinking",
        signature: "visible signature",
        answer: "answer",
      },
    },
  }));
  assert.match(html, /思维链回传/);
  assert.match(html, /5 \/ 5/);
  assert.doesNotMatch(html, /signature/);
  assert.doesNotMatch(html, /已回传|未回传|思考待定|摘要已回传|摘要未回传/);
});

test("mismatched fingerprint scores zero and stamps the card", () => {
  const ResultCards = loadModule("../components/ResultCards.tsx", {
    "@/components/AnimatedNumber": { default: ({ value }) => value },
    "@/components/ResultSkeleton": { default: () => null },
    "@/components/Hint": { default: ({ label }) => React.createElement("button", { "aria-label": label }) },
    "@/components/ErrorFold": { default: () => null },
    "@/lib/report-view": loadModule("../lib/report-view.ts"),
  }).default;
  const html = renderToStaticMarkup(React.createElement(ResultCards, {
    model: "gpt-6-astra",
    verdict: {
      total: {
        score: 85,
        max: 100,
        reason: "",
        parts: [
          { key: "fingerprint", label: "模型指纹归因", applicable: true, known: true, points: 10, max_points: 10, weight: 30, coefficient: 32, contribution: 32 },
          { key: "iq", label: "推理表现", applicable: true, known: true, points: 100, max_points: 100, weight: 25, coefficient: 27, contribution: 27 },
        ],
      },
      fingerprint: {
        family: "gpt",
        family_name: "GPT",
        family_prob: 0.99,
        top_model: "gpt-5.6-luna",
        top_prob: 0.99,
        used_outputs: 3,
        note: "说明",
        results: [],
      },
      injection: { inj: 12, score: 10, slope: 1, points: [], note: "注入说明" },
    },
  }));
  assert.match(html, /声称/);
  assert.match(html, /gpt-6-astra/);
  assert.match(html, /实际/);
  assert.match(html, /gpt-5\.6-luna/);
  assert.match(html, /fake-seal/);
  assert.match(html, /0\/32/);
  assert.match(html, />53</);
  assert.match(html, /aria-label="注入估计说明"/);
  const head = html.slice(html.indexOf("注入大小") - 80, html.indexOf("注入大小") + 220);
  assert.match(head, /rc-title[\s\S]*注入大小[\s\S]*注入估计说明/);
});
