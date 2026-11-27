const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const vm = require("node:vm");
const ts = require("typescript");
const React = require("react");
const { renderToStaticMarkup } = require("react-dom/server");
const appRequire = require("./app-require.cjs");

function loadAnimatedNumber(locale = "zh-CN") {
  const i18n = appRequire("@/lib/i18n");
  const file = path.join(__dirname, "../components/AnimatedNumber.tsx");
  const compiled = ts.transpileModule(fs.readFileSync(file, "utf8"), {
    compilerOptions: { module: ts.ModuleKind.CommonJS, jsx: ts.JsxEmit.ReactJSX },
  }).outputText;
  const mod = { exports: {} };
  const scope = {
    exports: mod.exports,
    module: mod,
    require: (name) => {
      if (name === "@/lib/i18n") {
        return {
          ...i18n,
          useI18n: () => ({
            lang: locale.startsWith("en") ? "en" : "zh",
            follow: false,
            locale,
            t: (s) => s,
            setLang: () => {},
          }),
        };
      }
      return appRequire(name);
    },
  };
  vm.runInNewContext(compiled, scope, { filename: file });
  return mod.exports.default;
}

test("AnimatedNumber formats decimals using context locale without hardcoded zh-CN", () => {
  const AnimatedNumberEn = loadAnimatedNumber("en-US");
  const html = renderToStaticMarkup(
    React.createElement(AnimatedNumberEn, { value: 1234.56, decimals: 1 }),
  );
  assert.match(html, /1,234\.6/);
  const source = fs.readFileSync(
    path.join(__dirname, "../components/AnimatedNumber.tsx"),
    "utf8",
  );
  assert.doesNotMatch(source, /toLocaleString\("zh-CN"/);
});

test("AnimatedNumber guards NaN and Infinity to zero", () => {
  const AnimatedNumber = loadAnimatedNumber("zh-CN");
  const nanHtml = renderToStaticMarkup(
    React.createElement(AnimatedNumber, { value: Number.NaN, decimals: 2 }),
  );
  const infHtml = renderToStaticMarkup(
    React.createElement(AnimatedNumber, { value: Number.POSITIVE_INFINITY, decimals: 0 }),
  );
  assert.match(nanHtml, />0\.00</);
  assert.match(infHtml, />0</);
});
