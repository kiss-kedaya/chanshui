const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const appRequire = require("./app-require.cjs");

const M3_FILES = [
  "app/history/page.tsx",
  "components/ScoreHero.tsx",
  "components/ResultCards.tsx",
  "components/ProbeTimeline.tsx",
  "components/ConversationPanel.tsx",
  "components/ReportShare.tsx",
  "components/ReportStage.tsx",
  "components/ResultSkeleton.tsx",
];

test("reportRows has valid 5-locale entries and no duplicate keys with base rows", () => {
  const { rows } = appRequire("@/lib/i18n-rows");
  const { homeRows } = appRequire("@/lib/i18n-rows-home");
  const { reportRows } = appRequire("@/lib/i18n-rows-report");
  const { docsAdminRows } = appRequire("@/lib/i18n-rows-docs-admin");

  assert.ok(Array.isArray(reportRows) && reportRows.length >= 80);
  const reportKeys = new Set();
  for (const row of reportRows) {
    assert.equal(row.length, 5, `Expected 5 columns for "${row[0]}"`);
    for (let i = 0; i < 5; i++) {
      assert.ok(
        typeof row[i] === "string" && row[i].trim().length > 0,
        `Empty column ${i} for "${row[0]}"`,
      );
    }
    assert.ok(!reportKeys.has(row[0]), `Duplicate key in reportRows: "${row[0]}"`);
    reportKeys.add(row[0]);
  }

  const baseCount = rows.length - homeRows.length - reportRows.length - docsAdminRows.length;
  const otherKeys = new Set([
    ...rows.slice(0, baseCount).map((r) => r[0]),
    ...homeRows.map((r) => r[0]),
    ...docsAdminRows.map((r) => r[0]),
  ]);
  for (const key of reportKeys) {
    assert.ok(!otherKeys.has(key), `Key "${key}" in reportRows already exists in base/home/docsAdmin rows`);
  }
});

test("all M3 files have 0 missing i18n keys and 0 hardcoded zh-CN locale calls", () => {
  const { rows } = appRequire("@/lib/i18n-rows");
  const map = new Map();
  for (const row of rows) {
    map.set(row[0], row);
  }

  const extraStaticKeys = [
    // ScoreHero HEADLINE & LEDE
    "未发现掺水迹象",
    "存在 {n} 处疑点",
    "疑似掺水",
    "证据不足",
    "本次范围内指纹、注入和工具都没有异常。这不是型号证明。",
    "总分只是加权平均，下面这些发现值得你再看一眼。",
    "指纹在 99.9% 以上置信度下指向另一个模型，总分已扣掉指纹分。",
    "多项请求失败，分数只反映拿到的部分，建议重新检测后再看结论。",
    // ReportStage
    "正在读取报告",
    "全站检测报告",
    "模型指纹归因",
    "注入大小",
    "推理表现",
    "接口完整性",
    // ResultCards sigShort + ANTICHEAT_SECTION_LABEL
    "伪造签名",
    "真签",
    "历史缓存",
    "标准信封",
    "空信封",
    "半成品",
    "未识别",
    "无签",
    "站点规则",
    "模型指纹",
    "思维链",
    "识图能力",
    "工具调用",
  ];

  for (const key of extraStaticKeys) {
    const row = map.get(key);
    assert.ok(row, `Missing extra M3 static key "${key}"`);
    for (let i = 0; i < 5; i++) {
      assert.ok(typeof row[i] === "string" && row[i].length > 0);
    }
  }

  for (const rel of M3_FILES) {
    const src = fs.readFileSync(path.join(__dirname, "..", rel), "utf8");
    assert.doesNotMatch(
      src,
      /toLocaleString\(\s*["']zh-CN["']/,
      `Hardcoded zh-CN toLocaleString found in ${rel}`,
    );
    for (const match of src.matchAll(/\b(?:t|tr)\(\s*"([^"]+)"/g)) {
      const key = match[1];
      const row = map.get(key);
      assert.ok(row, `Missing i18n row for "${key}" in ${rel}`);
      for (let i = 0; i < 5; i++) {
        assert.ok(
          typeof row[i] === "string" && row[i].length > 0,
          `Empty column ${i} for "${key}" in ${rel}`,
        );
      }
    }
  }
});

test("backend static and template report strings translate across all non-zh locales", () => {
  const { translate } = appRequire("@/lib/i18n");
  const samples = [
    ["深度体检跑渠道指纹（hi 用量、行为注入、现签/复签、system 透传）加 beta 项（thinking 绑定、上下文压缩）。签名解码和协议严格性在常规检测里。"],
    ["每步换唯一 nonce，正文约 8000 token，max_tokens=16。无 cache_control 时 input 很小却出现大额 cache write/read = 强制写缓存；正常透传应是大额 input、write=0、read=0。"],
    ["缓存链没有建立：后续轮次没有比首轮多出一段可复用缓存。"],
    ["上游没有缓存读取，无法判断。"],
    ["最坏情况保证"],
    ["联网证据"],
    ["流式完整性"],
    ["{n} 轮未接上前缀，疑似中途换号。", { n: 2 }],
    ["检测到 {n} 项针对性作假证据（{titles}），对应板块已按 0 分计入。", { n: 1, titles: "回放" }],
    ["伪造"],
    ["历史"],
    ["伪造思维链签名"],
    ["协议校验 {a}/{b}：渠道对非法请求偏宽松", { a: 1, b: 2 }],
    ["当前卷面 {n}；半分仅在卷面展示，不进入百分制通过分。失败请求标记未完成，但已答题目照常计分，不伪装为模型答错。", { n: 100 }],
    ["行为库没有该家族候选，未启用行为比对"],
    ["行为库候选缺少所需字段，未产生比对结果"],
    ["DeepSeek Search API 返回 {n} 条有效结果，含标题、摘要和来源链接。", { n: 3 }],
    ["协议严格性 {a}/{b}（未拒绝：{items}）", { a: 9, b: 10, items: "system_role" }],
  ];
  for (const lang of ["en", "zh-Hant", "ja", "ko"]) {
    for (const [key, vars] of samples) {
      const out = translate(lang, key, vars);
      assert.ok(out && out !== key, `Expected ${lang} translation for "${key}", got "${out}"`);
    }
  }
});
