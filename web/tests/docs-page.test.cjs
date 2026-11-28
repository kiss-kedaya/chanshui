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
    compilerOptions: { module: ts.ModuleKind.CommonJS, jsx: ts.JsxEmit.ReactJSX },
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

const ASCII_ALLOWLIST = new Set(["OpenAPI", "OpenAPI 3.1", "Body", "Path"]);

test("DocsPage SSR renders loading skeleton with aria-busy and complete tablist/table ARIA", () => {
  const mod = loadComponent("app/docs/page.tsx");
  assert.deepEqual(Object.keys(mod), ["default"]);
  const DocsPage = mod.default;

  const html = renderToStaticMarkup(React.createElement(DocsPage));
  assert.match(html, /class="api-doc"/);
  assert.match(html, /class="api-doc-count-skeleton is-pill"/);
  assert.match(html, /aria-busy="true"/);
  assert.match(html, /aria-live="polite"/);
  assert.doesNotMatch(html, /次数暂不可用/);
  assert.match(html, /data-label="必填"/);
  assert.match(html, /data-copied="0"/);
  assert.match(html, /role="tablist"/);
  assert.match(html, /role="tab"/);
  assert.match(html, /role="tabpanel"/);
  assert.match(html, /aria-labelledby="status-tab-create-202"/);
  assert.match(html, /aria-controls="status-panel-create"/);
});

test("DocsPage translates line 117 vision + DeepSeek note across all non-zh locales", () => {
  const realI18n = appRequire("@/lib/i18n");
  for (const lang of ["en", "zh-Hant", "ja", "ko", "ru"]) {
    const DocsPage = loadComponent("app/docs/page.tsx", {
      "@/lib/i18n": {
        ...realI18n,
        useI18n: () => ({
          lang,
          follow: false,
          locale: realI18n.intlLocale(lang),
          t: (source, vars) => realI18n.translate(lang, source, vars),
          setLang: () => {},
        }),
      },
    }).default;

    const html = renderToStaticMarkup(React.createElement(DocsPage));
    assert.doesNotMatch(
      html,
      /DeepSeek 的 web 会直接调用/,
      `Line 117 leaked Simplified Chinese in locale ${lang}`,
    );
    assert.match(html, /\/v1\/tools\/search/);
  }
});

test("DocsPage 6-locale i18n coverage is complete with zero missing keys", () => {
  const { rows } = appRequire("@/lib/i18n-rows");
  const map = new Map();
  for (const row of rows) map.set(row[0], row);

  const src = fs.readFileSync(path.join(__dirname, "../app/docs/page.tsx"), "utf8");
  const keys = new Set();

  for (const m of src.matchAll(/\bt\(\s*"([^"]+)"/g)) keys.add(m[1]);
  for (const m of src.matchAll(/\b(?:summary|paramsLabel|note|title):\s*"([^"]+)"/g)) {
    if (m[1]) keys.add(m[1]);
  }
  for (const m of src.matchAll(/^\s{4}name:\s*"([^"]+)"/gm)) {
    keys.add(m[1]);
  }
  const sectionsBlock = src.match(/const sections = useMemo\(\s*\(\)\s*=>\s*\[([\s\S]*?)\],\s*\[\],\s*\)/);
  assert.ok(sectionsBlock, "sections useMemo block should exist");
  for (const m of sectionsBlock[1].matchAll(/\[\s*"[^"]+"\s*,\s*"([^"]+)"\s*\]/g)) {
    keys.add(m[1]);
  }

  for (const key of keys) {
    if (ASCII_ALLOWLIST.has(key)) continue;
    const row = map.get(key);
    assert.ok(row, `Missing i18n row for "${key}" in docs/page.tsx`);
    assert.equal(row.length, 6);
    for (let i = 0; i < 6; i++) {
      assert.ok(typeof row[i] === "string" && row[i].length > 0, `Empty locale column ${i} for "${key}"`);
    }
  }
});

test("DocsPage and docs-admin.css satisfy M4 contract & responsive cascade rules", () => {
  const pageSrc = fs.readFileSync(path.join(__dirname, "../app/docs/page.tsx"), "utf8");
  assert.match(pageSrc, /formatLocaleNumber/);
  assert.doesNotMatch(pageSrc, /toLocaleString\(\s*"zh-CN"/);
  assert.match(pageSrc, /clearTimeout/);
  assert.match(pageSrc, /hashchange/);
  assert.doesNotMatch(pageSrc, /^export\s+(?:type|function|const|class)\s+/m);

  const cssSrc = fs.readFileSync(path.join(__dirname, "../app/styles/docs-admin.css"), "utf8");
  assert.match(cssSrc, /\.api-doc \.api-table\s*\{[^}]*margin-top:\s*0;[^}]*font-family:\s*var\(--font-sans\);/s);
  assert.match(cssSrc, /\.api-doc \.agent-onboard-tip\s*\{[^}]*calc\(100vw - 32px\)[^}]*white-space:\s*normal;/s);
  assert.match(cssSrc, /@media \(hover:\s*none\)/);
  assert.match(cssSrc, /\.api-doc \.api-url\s*\{[^}]*flex-wrap:\s*wrap;/s);
  assert.match(cssSrc, /\.api-doc \.api-url button,\s*\.api-doc \.api-block-head button\s*\{[^}]*white-space:\s*nowrap;/s);
  const selectedTabMatch = cssSrc.match(/\.api-doc \.api-status-tabs button\[aria-selected="true"\]\s*\{([^}]*)\}/);
  assert.ok(selectedTabMatch, "selected status tab rule should exist");
  assert.doesNotMatch(selectedTabMatch[1], /--focus-ring/);
  assert.match(cssSrc, /html\[data-mode="dark"\] \.api-doc \.api-detail pre/);
  assert.match(cssSrc, /@media \(max-width:\s*640px\)/);
  assert.match(cssSrc, /@supports \(corner-shape:\s*squircle\)/);
  assert.doesNotMatch(cssSrc, /#17191c|#101214/i);

  // Iteration 2 regressions: AA contrast on active nav & section code, tooltip downward placement, hashchange empty-hash reset
  assert.match(
    cssSrc,
    /\.api-doc \.api-tree button\[aria-current="page"\],\s*\.api-doc \.api-tree button\[aria-current="page"\]:hover\s*\{[^}]*color:\s*color-mix\(in srgb,\s*var\(--accent\) 66%,\s*var\(--ink\)\);/s,
  );
  assert.match(
    cssSrc,
    /\.api-doc \.api-sections code\s*\{[^}]*color:\s*color-mix\(in srgb,\s*var\(--accent\) 66%,\s*var\(--ink\)\);/s,
  );
  assert.match(
    cssSrc,
    /\.api-doc \.agent-onboard-tip\s*\{[^}]*top:\s*calc\(100% \+ 8px\);[^}]*bottom:\s*auto;/s,
  );
  assert.match(
    pageSrc,
    /const syncFromHash = \(\) => \{\s*setCurrent\(resolveDocsItemId\(window\.location\.hash\)\);\s*\};/,
  );
});
