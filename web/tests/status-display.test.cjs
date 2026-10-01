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
    require: (name) => imports[name] || require(name),
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
  assert.match(html, /思维摘要/);
  assert.match(html, /signature/);
  assert.doesNotMatch(html, /已回传|未回传|思考待定|摘要已回传|摘要未回传/);
});
