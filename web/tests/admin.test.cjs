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

test("AdminPage SSR renders checking skeleton cleanly without custom named exports", () => {
  const mod = loadComponent("app/admin/page.tsx");
  assert.deepEqual(Object.keys(mod), ["default"]);
  const AdminPage = mod.default;

  const html = renderToStaticMarkup(React.createElement(AdminPage));
  assert.match(html, /class="admin-shell"/);
  assert.match(html, /admin-skeleton/);
  assert.match(html, /role="status"/);
  assert.match(html, /aria-busy="true"/);
  assert.match(html, /正在检查管理员会话…/);
});

test("AdminPage translates across locales and i18n-rows has zero missing or duplicate keys", () => {
  const { rows } = appRequire("@/lib/i18n-rows");
  const { docsAdminRows } = appRequire("@/lib/i18n-rows-docs-admin");

  const baseRows = rows.slice(0, rows.length - docsAdminRows.length);
  const baseKeys = new Set(baseRows.map((r) => r[0]));
  const docsAdminSeen = new Set();
  for (const row of docsAdminRows) {
    assert.equal(row.length, 6);
    assert.equal(docsAdminSeen.has(row[0]), false, `Duplicate key inside docsAdminRows: "${row[0]}"`);
    assert.equal(baseKeys.has(row[0]), false, `docsAdminRows duplicates existing i18n key: "${row[0]}"`);
    docsAdminSeen.add(row[0]);
  }

  const map = new Map(rows.map((r) => [r[0], r]));
  const src = fs.readFileSync(path.join(__dirname, "../app/admin/page.tsx"), "utf8");
  const keys = new Set();
  for (const m of src.matchAll(/\btr\(\s*"([^"]+)"/g)) keys.add(m[1]);
  const labelsMatch = src.match(/const captureLabels[^=]*=\s*\{([\s\S]*?)\};/);
  assert.ok(labelsMatch, "captureLabels object should exist");
  for (const m of labelsMatch[1].matchAll(/:\s*"([^"]+)"/g)) keys.add(m[1]);

  for (const key of keys) {
    const row = map.get(key);
    assert.ok(row, `Missing i18n row for "${key}" in admin/page.tsx`);
    for (let i = 0; i < 6; i++) {
      assert.ok(typeof row[i] === "string" && row[i].length > 0, `Empty locale column ${i} for "${key}"`);
    }
  }

  const realI18n = appRequire("@/lib/i18n");
  for (const lang of ["en", "zh-Hant", "ja", "ko", "ru"]) {
    const AdminPage = loadComponent("app/admin/page.tsx", {
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
    const html = renderToStaticMarkup(React.createElement(AdminPage));
    assert.doesNotMatch(html, /正在检查管理员会话…/, `Locale ${lang} leaked Simplified Chinese checking label`);
  }
});

test("AdminPage and docs-admin.css enforce auth/error/combobox/a11y/theme contracts", () => {
  const src = fs.readFileSync(path.join(__dirname, "../app/admin/page.tsx"), "utf8");
  assert.doesNotMatch(src, /^export\s+(?:type|function|const|class)\s+/m);
  assert.match(src, /type AuthState =/);
  assert.match(src, /"disabled"/);
  assert.match(src, /"unauthenticated"/);
  assert.match(src, /"load_error"/);
  assert.match(src, /res\.status === 429/);
  assert.match(src, /disabled=\{!items \|\| items\.length === 0\}/);
  assert.match(src, /formatLocaleDate/);
  assert.doesNotMatch(src, /"zh-CN"/);
  assert.doesNotMatch(src, /window\.confirm/);
  assert.doesNotMatch(src, /<datalist/);
  assert.match(src, /role="tablist"/);
  assert.match(src, /role="tab"/);
  assert.match(src, /role="tabpanel"/);
  assert.match(src, /aria-selected=/);
  assert.match(src, /idEdited/);
  assert.match(src, /confirmDeleteId/);
  assert.match(src, /deletingId/);
  assert.match(src, /AbortController/);
  assert.match(src, /filterModels/);
  assert.match(src, /modelFamilyLabel/);
  assert.match(src, /id="admin-panel-fingerprint"[\s\S]*?\{error &&/);
  assert.match(src, /id="admin-panel-fingerprint"[\s\S]*?\{refError &&/);

  const css = fs.readFileSync(path.join(__dirname, "../app/styles/docs-admin.css"), "utf8");
  assert.doesNotMatch(css, /#bb5432|#b23b21|#267a4b/i);
  assert.match(css, /\.admin-shell \.admin-tabs \[role="tab"\]:focus-visible/);
  assert.match(css, /\.admin-shell \.admin-button\.ghost:hover:not\(:disabled\)/);
  assert.match(css, /\.admin-shell \.admin-button\.danger/);
  assert.match(css, /\.admin-shell \.admin-progress\.running/);
  assert.match(css, /\.admin-shell \.admin-progress\.validating/);
  assert.match(css, /\.admin-shell \.admin-progress\.done/);
  assert.match(css, /\.admin-shell \.admin-progress\.failed/);
  assert.match(css, /\.admin-shell \.admin-progress\.cancelled/);
  assert.match(css, /\.admin-shell \.admin-progress-ring/);
  assert.match(css, /\.admin-shell \.is-pill/);
  assert.match(css, /@media \(max-width:\s*760px\)/);

  // Iteration 2 regressions: inline delete confirmation layout, 404/transient capture recovery, AA contrast, model input/button alignment
  assert.match(css, /\.admin-shell \.admin-ref-list article > div:first-child\s*\{[^}]*flex:\s*1 1 200px;/s);
  assert.doesNotMatch(css, /\.admin-shell \.admin-ref-list article > div\s*\{/);
  assert.match(
    css,
    /\.admin-shell \.admin-ref-confirm\s*\{[^}]*flex:\s*0 1 auto;[^}]*flex-direction:\s*row;/s,
  );
  assert.match(
    css,
    /@media \(max-width:\s*760px\)[\s\S]*\.admin-shell \.admin-ref-confirm\s*\{[^}]*justify-content:\s*flex-start;/s,
  );
  assert.match(
    css,
    /\.admin-shell \.admin-tabs \[role="tab"\]\[aria-selected="true"\]\s*\{[^}]*color:\s*color-mix\(in srgb,\s*var\(--accent\) 66%,\s*var\(--ink\)\);/s,
  );
  assert.match(
    css,
    /\.admin-shell \.admin-progress\.running \.admin-status-pill\s*\{[^}]*color:\s*color-mix\(in srgb,\s*var\(--accent\) 66%,\s*var\(--ink\)\);/s,
  );
  assert.match(
    css,
    /\.admin-shell \.admin-progress\.running \.admin-status-pill b\s*\{[^}]*color:\s*color-mix\(in srgb,\s*var\(--accent\) 66%,\s*var\(--ink\)\);/s,
  );
  assert.match(
    css,
    /\.admin-shell \.admin-progress\.cancelled \.admin-status-pill\s*\{[^}]*color:\s*var\(--ink-soft\);/s,
  );
  assert.match(
    css,
    /\.admin-shell \.admin-progress\.cancelled \.admin-status-pill b\s*\{[^}]*color:\s*var\(--ink-soft\);/s,
  );
  assert.match(css, /\.admin-shell \.admin-model-field-row\s*\{[^}]*align-items:\s*center;/s);
  assert.match(css, /\.admin-shell \.admin-model-combobox > input:focus\s*\{[^}]*transform:\s*none;/s);
  assert.match(css, /\.admin-shell \.admin-model-combobox > input\s*\{[^}]*height:\s*47px;/s);
  assert.match(css, /\.admin-shell \.admin-fetch-models\s*\{[^}]*height:\s*47px;/s);

  const terminalTransitions = src.match(
    /prev\.status === "running" \|\| prev\.status === "validating"\)\s*\?\s*\{\s*\.\.\.prev,\s*status:\s*"failed",\s*error:\s*msg\s*\}/g,
  );
  assert.ok(
    terminalTransitions && terminalTransitions.length >= 4,
    "cancelCapture 404, poll 404, poll HTTP transientErrors>=3, and poll catch transientErrors>=3 must transition active capture out of running/validating",
  );
});
