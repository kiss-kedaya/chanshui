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

test("Hint computeHintPosition flips upward near viewport bottom and clamps horizontally", () => {
  const { computeHintPosition } = appRequire("@/components/Hint");

  const bottomEdge = computeHintPosition(
    { left: 340, right: 365, top: 760, bottom: 785, width: 25 },
    { width: 390, height: 844 },
    { width: 280, height: 120 },
  );
  assert.equal(bottomEdge.placement, "top");
  assert.ok(bottomEdge.top < 760);
  assert.ok(bottomEdge.left >= 8 && bottomEdge.left + 280 <= 382);

  const normalPos = computeHintPosition(
    { left: 10, right: 35, top: 80, bottom: 105, width: 25 },
    { width: 1440, height: 900 },
    { width: 280, height: 120 },
  );
  assert.equal(normalPos.placement, "bottom");
  assert.equal(normalPos.left, 8);
  assert.equal(normalPos.top, 113);
});

test("Hint nextHintVisibility pins on hover-then-click and toggles cleanly", () => {
  const { nextHintVisibility } = appRequire("@/components/Hint");

  // Desktop hover -> focus -> click upgrades to pinned instead of closing
  let s = { open: false, pinned: false };
  s = nextHintVisibility(s, "hover-enter", false);
  assert.deepEqual({ ...s }, { open: true, pinned: false });
  s = nextHintVisibility(s, "focus", false);
  assert.deepEqual({ ...s }, { open: true, pinned: false });
  s = nextHintVisibility(s, "click", false);
  assert.deepEqual({ ...s }, { open: true, pinned: true });

  // Mouse leaving after pinning keeps tooltip open
  s = nextHintVisibility(s, "hover-leave", false);
  assert.deepEqual({ ...s }, { open: true, pinned: true });

  // Second click closes pinned tooltip
  s = nextHintVisibility(s, "click", false);
  assert.deepEqual({ ...s }, { open: false, pinned: false });

  // Touch ignores synthetic hover/focus and toggles cleanly on click
  let touch = { open: false, pinned: false };
  touch = nextHintVisibility(touch, "hover-enter", true);
  assert.deepEqual({ ...touch }, { open: false, pinned: false });
  touch = nextHintVisibility(touch, "click", true);
  assert.deepEqual({ ...touch }, { open: true, pinned: true });
  touch = nextHintVisibility(touch, "click", true);
  assert.deepEqual({ ...touch }, { open: false, pinned: false });

  // Blur preserves pinned tooltips while closing unpinned ones; dismiss closes both
  assert.deepEqual(
    { ...nextHintVisibility({ open: true, pinned: true }, "blur", false) },
    { open: true, pinned: true },
  );
  assert.deepEqual(
    { ...nextHintVisibility({ open: true, pinned: false }, "blur", false) },
    { open: false, pinned: false },
  );
  const dismissed = nextHintVisibility({ open: true, pinned: true }, "dismiss", false);
  assert.deepEqual({ ...dismissed }, { open: false, pinned: false });

  const hintSrc = fs.readFileSync(path.join(__dirname, "../components/Hint.tsx"), "utf8");
  assert.match(hintSrc, /e\.preventDefault\(\)/);
  assert.match(hintSrc, /e\.stopPropagation\(\)/);
});

test("LanguageMenu uses valid listbox/option ARIA and nextOptionIndex wraps cleanly", () => {
  const { nextOptionIndex } = appRequire("@/components/LanguageMenu");
  assert.equal(nextOptionIndex(0, "ArrowDown", 6), 1);
  assert.equal(nextOptionIndex(5, "ArrowDown", 6), 0);
  assert.equal(nextOptionIndex(0, "ArrowUp", 6), 5);
  assert.equal(nextOptionIndex(3, "Home", 6), 0);
  assert.equal(nextOptionIndex(1, "End", 6), 5);

  const source = fs.readFileSync(path.join(__dirname, "../components/LanguageMenu.tsx"), "utf8");
  assert.match(source, /aria-haspopup="listbox"/);
  assert.match(source, /role="option"/);
  assert.match(source, /aria-selected=/);
  assert.doesNotMatch(source, /aria-pressed/);
});

test("ThemeMenu renders trigger and popover controls cleanly", () => {
  const ThemeMenu = appRequire("@/components/ThemeMenu").default;
  const html = renderToStaticMarkup(React.createElement(ThemeMenu));
  assert.match(html, /class="theme-menu"/);
  assert.match(html, /class="theme-trigger"/);
  assert.match(html, /theme-dot-mono/);
  assert.match(html, /外观/);
  const source = fs.readFileSync(path.join(__dirname, "../components/ThemeMenu.tsx"), "utf8");
  assert.match(source, /pointerdown/);
  assert.match(source, /暖橙/);
  assert.match(source, /薄荷绿/);
  assert.match(source, /黑白灰/);
});

test("Feedback uses explicit type=button on close/done actions and includes Select", () => {
  const Feedback = appRequire("@/components/Feedback").default;
  const html = renderToStaticMarkup(React.createElement(Feedback));
  assert.match(html, /class="feedback-trigger"/);
  assert.match(html, /class="feedback-dialog"/);
  const source = fs.readFileSync(path.join(__dirname, "../components/Feedback.tsx"), "utf8");
  assert.match(source, /<button type="button" className="btn-primary" onClick=\{close\}>/);
  assert.match(source, /<Select/);
});

test("Sidebar helpers handle pending badges, active report matching, and scroll cue", () => {
  const {
    default: Sidebar,
    sidebarBadgeTone,
    isActiveReportItem,
    computeScrollCue,
  } = loadComponent("components/Sidebar.tsx", {
    "next/navigation": {
      usePathname: () => "/history",
      useSearchParams: () => new URLSearchParams("id=rpt-42"),
    },
    "next/link": {
      default: ({ href, children, ...props }) =>
        React.createElement("a", { href, ...props }, children),
    },
  });

  assert.equal(sidebarBadgeTone({ anticheat_detected: true, total_score: 95, iq_score: 95 }), " is-bad");
  assert.equal(sidebarBadgeTone({ anticheat_detected: false, total_score: null, iq_score: null }), " is-pending");
  assert.equal(sidebarBadgeTone({ anticheat_detected: false, total_score: 88, iq_score: 88 }), " is-ok");
  assert.equal(sidebarBadgeTone({ anticheat_detected: false, total_score: 65, iq_score: 65 }), " is-warn");
  assert.equal(sidebarBadgeTone({ anticheat_detected: false, total_score: 40, iq_score: 40 }), " is-bad");

  assert.equal(isActiveReportItem("/history", "rpt-42", { id: 1, report_id: "rpt-42" }), true);
  assert.equal(isActiveReportItem("/history", "1", { id: 1, report_id: "rpt-42" }), true);
  assert.equal(isActiveReportItem("/", "rpt-42", { id: 1, report_id: "rpt-42" }), false);

  assert.equal(computeScrollCue({ scrollTop: 0, scrollHeight: 200, clientHeight: 200 }), "none");
  assert.equal(computeScrollCue({ scrollTop: 0, scrollHeight: 400, clientHeight: 200 }), "bottom");
  assert.equal(computeScrollCue({ scrollTop: 40, scrollHeight: 400, clientHeight: 200 }), "both");
  assert.equal(computeScrollCue({ scrollTop: 200, scrollHeight: 400, clientHeight: 200 }), "top");

  const html = renderToStaticMarkup(React.createElement(Sidebar));
  assert.match(html, /aria-controls="sidebar-history-list"/);
  assert.match(html, /id="sidebar-history-list"/);
});

test("TopBar marks the active navigation item with aria-current=page", () => {
  const TopBar = loadComponent("components/TopBar.tsx", {
    "next/navigation": { usePathname: () => "/docs" },
    "next/link": {
      default: ({ href, children, ...props }) =>
        React.createElement("a", { href, ...props }, children),
    },
  }).default;

  const html = renderToStaticMarkup(React.createElement(TopBar));
  assert.match(html, /href="\/docs"[^>]*aria-current="page"/);
});

test("M1 CSS tokens, squircle exclusions, layout imports, pure-vector icon, and dead file removal", () => {
  const css = fs.readFileSync(path.join(__dirname, "../app/globals.css"), "utf8");
  assert.match(css, /html\[data-accent="orange"\]\[data-mode="light"\]/);
  assert.match(css, /html\[data-accent="orange"\]\[data-mode="dark"\]/);
  assert.match(css, /html\[data-accent="mint"\]\[data-mode="light"\]/);
  assert.match(css, /html\[data-accent="mint"\]\[data-mode="dark"\]/);
  assert.match(css, /html\[data-accent="mono"\]\[data-mode="light"\]/);
  assert.match(css, /html\[data-accent="mono"\]\[data-mode="dark"\]/);
  assert.match(css, /--z-tooltip:\s*250/);
  assert.match(css, /@supports \(corner-shape: squircle\)/);
  assert.match(css, /corner-shape: inherit/);
  assert.match(css, /button:not\(\.hint-button\):not\(\.agent-onboard\):not\(\.is-round\):not\(\.is-pill\)/);
  assert.match(
    css,
    /\.topbar-tools \.theme-menu,\s*\.topbar-tools \.lang-menu\s*\{\s*position:\s*static;/,
  );
  assert.match(css, /\.local-badge i::after,\s*\.live-dot::after,\s*\.status-pulse::after\s*\{\s*animation: none !important;\s*opacity: 0\.7;/);
  assert.doesNotMatch(css, /\.top-tools|\.docs-entry|Blacklist/);

  const layout = fs.readFileSync(path.join(__dirname, "../app/layout.tsx"), "utf8");
  assert.match(layout, /import "\.\/globals\.css";\s*import "\.\/styles\/home\.css";\s*import "\.\/styles\/report\.css";\s*import "\.\/styles\/docs-admin\.css";/);
  assert.match(layout, /viewportFit:\s*"cover"/);
  assert.doesNotMatch(layout, /黑榜/);

  const icon = fs.readFileSync(path.join(__dirname, "../app/icon.svg"), "utf8");
  assert.doesNotMatch(icon, /<text/);
  assert.match(icon, /<path/);

  assert.equal(fs.existsSync(path.join(__dirname, "../components/DocsLink.tsx")), false);
  assert.equal(fs.existsSync(path.join(__dirname, "../components/AuditGuide.tsx")), false);
});
