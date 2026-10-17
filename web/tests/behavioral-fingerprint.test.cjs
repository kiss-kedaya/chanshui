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
    compilerOptions: {
      module: ts.ModuleKind.CommonJS,
      jsx: ts.JsxEmit.ReactJSX,
    },
  }).outputText;
  const scope = {
    exports: {},
    require: (name) => imports[name] || require("./app-require.cjs")(name),
  };
  vm.runInNewContext(compiled, scope);
  return scope.exports;
}

function renderFingerprint(fingerprint) {
  const ResultCards = loadModule("../components/ResultCards.tsx", {
    "@/components/AnimatedNumber": { default: ({ value }) => value },
    "@/components/ResultSkeleton": { default: () => null },
    "@/components/Hint": {
      default: ({ label }) => React.createElement("button", { "aria-label": label }),
    },
    "@/components/ErrorFold": { default: () => null },
    "@/lib/report-view": loadModule("../lib/report-view.ts"),
  }).default;
  return renderToStaticMarkup(React.createElement(ResultCards, {
    model: "deepseek-flash",
    verdict: {
      total: {
        score: 85,
        max: 100,
        reason: "",
        parts: [
          {
            key: "fingerprint",
            label: "模型指纹归因",
            applicable: true,
            known: true,
            points: 10,
            max_points: 10,
            weight: 30,
            coefficient: 32,
            contribution: 32,
          },
        ],
      },
      fingerprint,
    },
  }));
}

test("behavioral attribution uses the dated display name and marks a confident mismatch", () => {
  const html = renderFingerprint({
    basis: "behavioral",
    family: "openai",
    family_name: "GPT",
    family_prob: 0.91,
    top_model: "openai/gpt-4o",
    top_display: "GPT-4o",
    top_prob: 0.91,
    top_similarity: 0.91,
    results: [{ model: "openai/gpt-4o", display: "GPT-4o", probability: 0.91, similarity: 0.91 }],
    behavioral_total: 34,
    behavioral_top3: [{ modelId: "openai/gpt-4o", displayName: "GPT-4o", family: "openai", similarity: 0.91 }],
    used_outputs: 3,
    requests: 3,
    low_confidence: false,
    note: "behavior note",
    evidence: [],
  });
  assert.match(html, /行为归因/);
  assert.match(html, /GPT-4o/);
  assert.match(html, /行为相似度 91\.0%/);
  assert.match(html, /数字库没有该家族/);
  assert.match(html, /fake-seal/);
});

test("weak behavioral evidence is shown without a fake seal", () => {
  const html = renderFingerprint({
    basis: "behavioral",
    family: "deepseek",
    family_name: "DeepSeek",
    family_prob: 0.81,
    top_model: "deepseek/deepseek-v3.2",
    top_display: "DeepSeek V3.2",
    top_prob: 0.81,
    top_similarity: 0.81,
    results: [{ model: "deepseek/deepseek-v3.2", display: "DeepSeek V3.2", probability: 0.81, similarity: 0.81 }],
    behavioral_total: 34,
    behavioral_top3: [{ modelId: "deepseek/deepseek-v3.2", displayName: "DeepSeek V3.2", family: "deepseek", similarity: 0.81 }],
    used_outputs: 3,
    requests: 3,
    low_confidence: true,
    note: "behavior note",
    evidence: [],
  });
  assert.match(html, /证据不足/);
  assert.doesNotMatch(html, /fake-seal/);
});
