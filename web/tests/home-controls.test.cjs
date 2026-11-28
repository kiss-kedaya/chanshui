const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const vm = require("node:vm");
const ts = require("typescript");
const React = require("react");
const { renderToStaticMarkup } = require("react-dom/server");
const appRequire = require("./app-require.cjs");

function loadComponent(relPath, mocks = {}) {
  const file = path.join(__dirname, "..", relPath);
  const compiled = ts.transpileModule(fs.readFileSync(file, "utf8"), {
    compilerOptions: {
      module: ts.ModuleKind.CommonJS,
      jsx: ts.JsxEmit.ReactJSX,
    },
  }).outputText;
  const mod = { exports: {} };
  const scope = {
    exports: mod.exports,
    module: mod,
    require: (name) => mocks[name] || appRequire(name, path.dirname(file)),
  };
  vm.runInNewContext(compiled, scope, { filename: file });
  return mod.exports;
}

test("Select computeSelectLayout flips upward near viewport bottom and distinguishes dialog inset", () => {
  const { computeSelectLayout, nextSelectOptionIndex } = appRequire("@/components/Select");

  const bottomEdge = computeSelectLayout({ top: 700, bottom: 747 }, 844, {
    hasFilter: true,
  });
  assert.equal(bottomEdge.placement, "top");
  assert.ok(bottomEdge.optionsMaxHeight >= 140 && bottomEdge.optionsMaxHeight <= 260);

  const dialogTight = computeSelectLayout({ top: 260, bottom: 307 }, 420, {
    isInDialog: true,
  });
  assert.equal(dialogTight.placement, "top");
  assert.ok(dialogTight.optionsMaxHeight > 180);

  const topbarTight = computeSelectLayout({ top: 260, bottom: 307 }, 420, {
    isInDialog: false,
    topInset: 58,
  });
  assert.equal(topbarTight.placement, "top");
  assert.ok(topbarTight.optionsMaxHeight < dialogTight.optionsMaxHeight);

  // Tight viewport (390x340) with filter input -> popoverMaxHeight never exceeds available clearance
  const mobileTight = computeSelectLayout({ top: 253, bottom: 300 }, 340, {
    isInDialog: false,
    topInset: 50,
    hasFilter: true,
  });
  assert.equal(mobileTight.placement, "top");
  assert.equal(mobileTight.popoverMaxHeight, 189);
  assert.equal(mobileTight.optionsMaxHeight, 125);

  const zeroSpace = computeSelectLayout({ top: 64, bottom: 110 }, 112, {
    isInDialog: false,
    topInset: 58,
  });
  assert.equal(zeroSpace.popoverMaxHeight, 48);
  assert.equal(zeroSpace.optionsMaxHeight, 32);

  assert.equal(nextSelectOptionIndex(-1, "ArrowDown", 5), 0);
  assert.equal(nextSelectOptionIndex(4, "ArrowDown", 5), 0);
  assert.equal(nextSelectOptionIndex(0, "ArrowUp", 5), 4);
  assert.equal(nextSelectOptionIndex(3, "Home", 5), 0);
  assert.equal(nextSelectOptionIndex(1, "End", 5), 4);
  assert.equal(nextSelectOptionIndex(0, "ArrowDown", 0), -1);
});

test("Select SSR treats valid empty value as selected value and replaces raw checkmark with SVG Icon", () => {
  const Select = appRequire("@/components/Select").default;

  const effortHtml = renderToStaticMarkup(
    React.createElement(
      Select,
      { id: "effort", value: "", onChange: () => {} },
      React.createElement("option", { value: "" }, "不传"),
      React.createElement("option", { value: "high" }, "高"),
    ),
  );
  assert.match(effortHtml, /class="select-value">不传<\/span>/);
  assert.doesNotMatch(effortHtml, /select-placeholder/);

  const unmatchedHtml = renderToStaticMarkup(
    React.createElement(
      Select,
      { id: "effort", value: "missing", onChange: () => {} },
      React.createElement("option", { value: "high" }, "高"),
    ),
  );
  assert.match(unmatchedHtml, /class="select-placeholder">请选择<\/span>/);

  const src = fs.readFileSync(path.join(__dirname, "../components/Select.tsx"), "utf8");
  assert.doesNotMatch(src, />✓</);
  assert.doesNotMatch(src, /\bautoFocus\b/);
  assert.match(src, /<Icon[\s\S]*name="check"[\s\S]*className="select-check"/);
  assert.match(src, /pointerdown/);
  assert.match(src, /filterInput\.current/);
  assert.match(src, /filterLabel \|\| tr\("筛选选项"\)/);
  assert.match(src, /filterPlaceholder \|\| tr\("输入关键字筛选…"\)/);
  assert.doesNotMatch(src, /tr\("搜索模型"\)/);
  assert.doesNotMatch(src, /tr\("输入名称筛选模型…"\)/);
});

test("AuditHero renders all 6 numbered probe points", () => {
  const AuditHero = appRequire("@/components/AuditHero").default;
  const html = renderToStaticMarkup(React.createElement(AuditHero));
  assert.match(html, /class="audit-hero"/);
  assert.match(html, /<small>01<\/small>/);
  assert.match(html, /<small>06<\/small>/);
  assert.match(html, /反作弊拦截/);
});

test("DetectPage wraps 高级设置, decouples Hint from label, and keeps 缓存读写 mounted", () => {
  const DetectPage = loadComponent("app/page.tsx", {
    "next/navigation": { useRouter: () => ({ push: () => {} }) },
    "next/link": {
      default: ({ href, children, ...props }) =>
        React.createElement("a", { href, ...props }, children),
    },
  }).default;

  const html = renderToStaticMarkup(React.createElement(DetectPage));
  assert.match(html, /高级设置/);
  assert.match(html, /缓存读写/);
  assert.match(html, /多轮对话/);
  assert.match(
    html,
    /<label class="opt-chip-toggle">[\s\S]*?缓存读写<\/span><\/label><span class="hint"/,
  );
  assert.match(
    html,
    /<label class="opt-chip-toggle">[\s\S]*?多轮对话<\/span><\/label><span class="hint"/,
  );

  const src = fs.readFileSync(path.join(__dirname, "../app/page.tsx"), "utf8");
  assert.match(src, /\{tr\("高级设置"\)\}/);
  // Hint must not be nested inside any <label>
  assert.doesNotMatch(src, /<label\b(?:(?!<\/label>)[\s\S])*?<Hint\b(?:(?!<\/label>)[\s\S])*?<\/label>/);
  // 缓存读写 chip remains mounted even when conversationOn is true
  assert.doesNotMatch(src, /\{!conversationOn\s*&&/);
  assert.match(src, /conversationOn \? " is-locked" : cacheOn \? " is-on" : ""/);
  assert.match(src, /disabled=\{busy \|\| conversationOn\}/);
  assert.match(src, /tr\("多轮已含"\)/);
  assert.match(src, /id="concurrency"[\s\S]*?disabled=\{busy\}/);
  assert.match(src, /id="timeout"[\s\S]*?disabled=\{busy\}/);
  assert.match(src, /id="thinking-mode"[\s\S]*?disabled=\{busy \|\| asking\}/);
  assert.match(src, /id="effort"[\s\S]*?disabled=\{busy \|\| asking\}/);
  assert.match(src, /id="question"[\s\S]*?disabled=\{busy \|\| asking\}/);
  assert.match(src, /type="button"[\s\S]*?aria-busy=\{asking\}/);
});

test("home.css establishes non-trapping stacking clearance, wrapped ModelPicker status, calm ErrorFold, wider ModelPicker family, and squircle pill opt-out", () => {
  const css = fs.readFileSync(path.join(__dirname, "../app/styles/home.css"), "utf8");
  assert.match(css, /@keyframes card-appear\s*\{[\s\S]*?transform:\s*none;/);
  assert.match(css, /\.opening:has\(\.custom-select > button\[aria-expanded="true"\], \.model-picker\[data-open="true"\]\)\s*\{[\s\S]*?z-index:\s*30;/);
  assert.match(css, /\.opening > \.connection-card\s*\{[\s\S]*?z-index:\s*auto;[\s\S]*?animation-name:\s*card-appear;[\s\S]*?animation-fill-mode:\s*backwards;/);
  assert.match(css, /\.ask-panel:has\(\.custom-select > button\[aria-expanded="true"\]\)\s*\{[\s\S]*?z-index:\s*30;[\s\S]*?content-visibility:\s*visible;/);
  assert.match(css, /\.model-picker\[data-open="true"\]\s*\{[\s\S]*?z-index:\s*var\(--z-dropdown\);/);
  assert.match(css, /\.field:has\(\.custom-select > button\[aria-expanded="true"\]\)\s*\{[\s\S]*?z-index:\s*var\(--z-dropdown\);/);
  assert.match(css, /\.model-status\s*\{[\s\S]*?max-width:\s*100%;/);
  assert.match(css, /\.model-status > span\s*\{[\s\S]*?overflow-wrap:\s*anywhere;/);
  assert.match(css, /\.model-options\s*\{[\s\S]*?min-height:\s*0;/);
  assert.match(css, /\.model-family\s*\{[\s\S]*?min-width:\s*66px;/);
  assert.match(css, /\.error-mark,\s*\.error-mark \.error-ring\s*\{\s*animation:\s*none;/);
  assert.match(css, /\.select-popover\[data-placement="top"\]/);
  assert.match(css, /@supports \(corner-shape: squircle\)\s*\{\s*\.opt-chip\s*\{\s*corner-shape:\s*round;/);
});

test("M2 i18n keys resolve across all 5 locales with zero duplicates", () => {
  const { rows } = appRequire("@/lib/i18n-rows");
  const { homeRows } = appRequire("@/lib/i18n-rows-home");

  assert.ok(Array.isArray(homeRows) && homeRows.length >= 16);
  const seen = new Map();
  for (const row of rows) {
    assert.equal(row.length, 5);
    for (let i = 0; i < 5; i++) {
      assert.ok(typeof row[i] === "string" && row[i].length > 0, `Empty col ${i} for "${row[0]}"`);
    }
    if (!seen.has(row[0])) seen.set(row[0], row);
  }
  const homeKeys = new Set();
  for (const row of homeRows) {
    assert.equal(homeKeys.has(row[0]), false, `Duplicate homeRows key: ${row[0]}`);
    homeKeys.add(row[0]);
  }

  const m2Files = [
    "app/page.tsx",
    "components/AuditHero.tsx",
    "components/ModelPicker.tsx",
    "components/Select.tsx",
    "components/ErrorFold.tsx",
  ];
  for (const rel of m2Files) {
    const src = fs.readFileSync(path.join(__dirname, "..", rel), "utf8");
    for (const match of src.matchAll(/\b(?:t|tr)\(\s*"([^"]+)"/g)) {
      const key = match[1];
      const row = seen.get(key);
      assert.ok(row, `Missing i18n row for "${key}" in ${rel}`);
    }
  }
  for (const label of ["模型指纹", "隐藏注入", "签名验真", "推理表现", "识图能力", "反作弊拦截"]) {
    assert.ok(seen.has(label), `Missing AuditHero point key: ${label}`);
  }
});
