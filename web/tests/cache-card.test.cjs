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

test("cache card uses the previous read for a read-only chain", () => {
  const html = renderCache({
    status: "pass",
    pass: true,
    samples: [
      { index: 2, input: 4, output: 2, input_reported: true, output_reported: true, read: 1000, write: 0, read_reported: true, write_reported: false, hit_rate: null },
      { index: 3, input: 4, output: 2, input_reported: true, output_reported: true, read: 1000, write: 0, read_reported: true, write_reported: false, hit_rate: null },
    ],
    third_read_baseline: 1000,
    third_read_ratio: 99.6,
  });
  assert.match(html, /末轮缓存率<\/span><strong>99\.60%<\/strong>/);
  assert.match(html, /上一轮读取<\/span><strong>1000<\/strong>/);
});

test("read-only cold start with an unreported first read still passes", () => {
  const rows = [
    { index: 1, input: 1909, output: 4, total: 1909, input_reported: true, output_reported: true, read: 0, write: 0, read_reported: false, write_reported: false, hit_rate: null },
    { index: 2, input: 143, output: 4, total: 1935, input_reported: true, output_reported: true, read: 1792, write: 0, read_reported: true, write_reported: false, hit_rate: 92.61 },
    { index: 3, input: 169, output: 4, total: 1961, input_reported: true, output_reported: true, read: 1792, write: 0, read_reported: true, write_reported: false, hit_rate: 91.38 },
    { index: 4, input: 195, output: 4, total: 1987, input_reported: true, output_reported: true, read: 1792, write: 0, read_reported: true, write_reported: false, hit_rate: 90.19 },
    { index: 5, input: 221, output: 4, total: 2013, input_reported: true, output_reported: true, read: 1792, write: 0, read_reported: true, write_reported: false, hit_rate: 89.05 },
    { index: 6, input: 247, output: 4, total: 2039, input_reported: true, output_reported: true, read: 1792, write: 0, read_reported: true, write_reported: false, hit_rate: 87.89 },
  ];
  const html = renderCache({
    status: "pass",
    pass: true,
    third_read_baseline: 1792,
    third_read_ratio: 87.89,
    samples: rows,
  });
  assert.match(html, /末轮缓存率<\/span><strong>87\.89%<\/strong>/);
  assert.match(html, /末轮读取<\/span><strong>1792<\/strong>/);
  assert.match(html, /末轮总输入<\/span><strong>2039<\/strong>/);
  assert.match(html, /上一轮读取<\/span><strong>1792<\/strong>/);
  assert.match(html, /<td>#1<\/td><td>—<\/td><td>—<\/td><td>1909<\/td><td>4<\/td><td>—<\/td>/);
  assert.match(html, /<td>#6<\/td><td>1792<\/td><td>—<\/td><td>247<\/td><td>4<\/td><td>87\.89%<\/td>/);
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
