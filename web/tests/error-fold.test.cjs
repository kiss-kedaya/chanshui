const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const vm = require("node:vm");
const ts = require("typescript");
const React = require("react");
const { renderToStaticMarkup } = require("react-dom/server");

function load(filename) {
  const source = fs.readFileSync(path.join(__dirname, filename), "utf8");
  const compiled = ts.transpileModule(source, {
    compilerOptions: { module: ts.ModuleKind.CommonJS, jsx: ts.JsxEmit.ReactJSX },
  }).outputText;
  const scope = { exports: {}, require };
  vm.runInNewContext(compiled, scope);
  return scope.exports;
}

test("upstream error stays collapsed until opened", () => {
  const { default: ErrorFold } = load("../components/ErrorFold.tsx");
  const text = "上游响应格式异常或流式响应中断（HTTP 200）；已尝试 4 次\nSSE stream ended without message_stop";
  const html = renderToStaticMarkup(React.createElement(ErrorFold, { text }));
  assert.match(html, /上游响应格式异常/);
  assert.match(html, /aria-expanded="false"/);
  assert.match(html, /error-mark/);
  assert.doesNotMatch(html, /SSE stream ended/);
});

test("short error has no toggle", () => {
  const { default: ErrorFold } = load("../components/ErrorFold.tsx");
  const html = renderToStaticMarkup(React.createElement(ErrorFold, { text: "请填写密钥" }));
  assert.match(html, /请填写密钥/);
  assert.doesNotMatch(html, /error-toggle/);
});
