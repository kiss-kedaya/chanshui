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

const ResultCards = loadModule("../components/ResultCards.tsx", {
  "@/components/AnimatedNumber": { default: ({ value }) => value },
  "@/components/ResultSkeleton": { default: () => null },
  "@/components/Hint": { default: () => null },
  "@/components/ErrorFold": loadModule("../components/ErrorFold.tsx"),
  "@/lib/report-view": loadModule("../lib/report-view.ts"),
}).default;

function renderCache(overrides) {
  return renderToStaticMarkup(React.createElement(ResultCards, {
    verdict: {
      cache: {
        status: "mismatch", pass: false, reason: "fixture",
        samples: [
          { index: 1, input: 4, output: 2, read: 0, write: 1000, input_reported: true, output_reported: true, read_reported: true, write_reported: true, hit_rate: 0 },
          { index: 2, input: 4, output: 2, read: 1000, write: 500, input_reported: true, output_reported: true, read_reported: true, write_reported: true, hit_rate: 66.49 },
          { index: 3, input: 4, output: 2, read: 1800, write: 0, input_reported: true, output_reported: true, read_reported: true, write_reported: false, hit_rate: null },
        ],
        third_read_ratio: 120,
        ...overrides,
      },
    },
  }));
}

test("cache card shows per-request rates, missing writes, and unbounded third-round ratio", () => {
  const html = renderCache({});
  assert.match(html, /末轮缓存率<\/span><strong>120\.00%<\/strong>/);
  assert.match(html, /上一轮读 ＋ 写<\/span><strong>1500<\/strong>/);
  assert.match(html, /<th>命中率<\/th>/);
  assert.match(html, /<td>#1<\/td><td>0<\/td><td>1000<\/td><td>4<\/td><td>2<\/td><td>0\.00%<\/td>/);
  assert.match(html, /<td>#3<\/td><td>1800<\/td><td>—<\/td><td>4<\/td><td>2<\/td><td>—<\/td>/);
  assert.match(html, /保留原始比例/);
});

test("cache card does not fabricate a denominator when previous write is missing", () => {
  const html = renderCache({
    samples: [
      { index: 2, input: 4, output: 2, input_reported: true, output_reported: true, read: 1000, write: 0, read_reported: true, write_reported: false, hit_rate: null },
      { index: 3, input: 4, output: 2, input_reported: true, output_reported: true, read: 1000, write: 0, read_reported: true, write_reported: false, hit_rate: null },
    ],
    third_read_ratio: null,
  });
  assert.match(html, /末轮缓存率<\/span><strong>—<\/strong>/);
  assert.match(html, /上一轮读 ＋ 写<\/span><strong>—<\/strong>/);
});

test("cache card distinguishes absent input from a reported zero", () => {
  const html = renderCache({
    samples: [
      { index: 1, input: 0, input_reported: false, output: 0, output_reported: false, read: 0, write: 0, read_reported: false, write_reported: false, hit_rate: null },
      { index: 2, input: 0, input_reported: true, output: 2, output_reported: true, read: 1000, write: 0, read_reported: true, write_reported: true, hit_rate: 100 },
    ],
    third_read_ratio: null,
  });
  assert.match(html, /<td>#1<\/td><td>—<\/td><td>—<\/td><td>—<\/td>/);
  assert.match(html, /<td>#2<\/td><td>1000<\/td><td>0<\/td><td>0<\/td>/);
});
