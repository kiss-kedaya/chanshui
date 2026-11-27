const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const appRequire = require("./app-require.cjs");

test("intlLocale maps supported languages and locale tags", () => {
  const { intlLocale } = appRequire("@/lib/i18n");
  assert.equal(intlLocale("zh"), "zh-CN");
  assert.equal(intlLocale("zh-CN"), "zh-CN");
  assert.equal(intlLocale("zh-Hant"), "zh-TW");
  assert.equal(intlLocale("zh-HK"), "zh-TW");
  assert.equal(intlLocale("en"), "en-US");
  assert.equal(intlLocale("en-GB"), "en-US");
  assert.equal(intlLocale("ja"), "ja-JP");
  assert.equal(intlLocale("ko"), "ko-KR");
  assert.equal(intlLocale(null), "zh-CN");
});

test("formatLocaleNumber and formatLocaleDate format safely across locales", () => {
  const { formatLocaleNumber, formatLocaleDate } = appRequire("@/lib/i18n");
  assert.equal(formatLocaleNumber(1234.5, "en", { minimumFractionDigits: 1, maximumFractionDigits: 1 }), "1,234.5");
  assert.equal(formatLocaleNumber(Number.NaN, "zh", { maximumFractionDigits: 0 }), "0");
  assert.equal(formatLocaleNumber(Number.POSITIVE_INFINITY, "ja", { maximumFractionDigits: 0 }), "0");

  const formattedDate = formatLocaleDate("2026-11-27T12:30:00Z", "en", {
    month: "2-digit",
    day: "2-digit",
    timeZone: "UTC",
  });
  assert.equal(formattedDate, "11/27");
  assert.equal(formatLocaleDate("not-a-date", "en"), "not-a-date");
});

test("translate supports Reload key across all 5 locales and interpolation", () => {
  const { translate } = appRequire("@/lib/i18n");
  assert.equal(translate("zh", "重新加载"), "重新加载");
  assert.equal(translate("en", "重新加载"), "Reload");
  assert.equal(translate("zh-Hant", "重新加载"), "重新載入");
  assert.equal(translate("ja", "重新加载"), "再読み込み");
  assert.equal(translate("ko", "重新加载"), "다시 불러오기");
  assert.equal(translate("en", "{n} 分", { n: 92 }), "92 pts");
});

test("translate ignores Object.prototype keys and returns source safely", () => {
  const { translate } = appRequire("@/lib/i18n");
  assert.equal(translate("en", "constructor"), "constructor");
  assert.equal(translate("en", "toString"), "toString");
  assert.equal(translate("en", "valueOf"), "valueOf");
  assert.equal(translate("en", "hasOwnProperty"), "hasOwnProperty");
  assert.equal(translate("en", "constructor", { n: 1 }), "constructor");
  assert.equal(translate("ja", "__proto__"), "__proto__");
});

test("modular i18n-rows slices load cleanly and cover all M1 chrome/overlay strings", () => {
  const { rows } = appRequire("@/lib/i18n-rows");
  const { homeRows } = appRequire("@/lib/i18n-rows-home");
  const { reportRows } = appRequire("@/lib/i18n-rows-report");
  const { docsAdminRows } = appRequire("@/lib/i18n-rows-docs-admin");

  assert.ok(Array.isArray(rows) && rows.length >= 619);
  assert.ok(Array.isArray(homeRows));
  assert.ok(Array.isArray(reportRows));
  assert.ok(Array.isArray(docsAdminRows));

  const map = new Map();
  for (const row of rows) {
    assert.equal(row.length, 5);
    map.set(row[0], row);
  }

  const m1Files = [
    "components/TopBar.tsx",
    "components/Sidebar.tsx",
    "components/ThemeMenu.tsx",
    "components/LanguageMenu.tsx",
    "components/Feedback.tsx",
    "components/Hint.tsx",
    "components/GitHubLink.tsx",
  ];
  for (const rel of m1Files) {
    const src = fs.readFileSync(path.join(__dirname, "..", rel), "utf8");
    for (const match of src.matchAll(/\b(?:t|tr)\(\s*"([^"]+)"/g)) {
      const key = match[1];
      const row = map.get(key);
      assert.ok(row, `Missing i18n row for "${key}" in ${rel}`);
      for (let i = 0; i < 5; i++) {
        assert.ok(typeof row[i] === "string" && row[i].length > 0, `Empty column ${i} for "${key}"`);
      }
    }
  }
});
