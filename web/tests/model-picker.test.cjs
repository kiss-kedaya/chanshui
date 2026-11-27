const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const vm = require("node:vm");
const ts = require("typescript");
const React = require("react");
const { renderToStaticMarkup } = require("react-dom/server");
const appRequire = require("./app-require.cjs");

const source = fs.readFileSync(
  path.join(__dirname, "../lib/model-picker.ts"),
  "utf8",
);
const compiled = ts.transpileModule(source, {
  compilerOptions: { module: ts.ModuleKind.CommonJS },
}).outputText;
const scope = { exports: {} };
vm.runInNewContext(compiled, scope);
const { filterModels, modelFamilyLabel } = scope.exports;

test("model families are labelled for the picker", () => {
  assert.equal(modelFamilyLabel("claude-opus-5-5"), "Claude");
  assert.equal(modelFamilyLabel("openai/gpt-5.6-luna"), "GPT");
  assert.equal(modelFamilyLabel("o3-mini"), "GPT");
  assert.equal(modelFamilyLabel("google/gemini-3.1-pro-preview"), "Gemini");
  assert.equal(modelFamilyLabel("x-ai/grok-4.5"), "Grok");
  assert.equal(modelFamilyLabel("deepseek-v4.1-flash"), "DeepSeek");
  assert.equal(modelFamilyLabel("z-ai/glm-5.2"), "GLM");
  assert.equal(modelFamilyLabel("qwen3-max"), "Qwen");
  assert.equal(modelFamilyLabel("kimi-k2"), "Kimi");
  assert.equal(modelFamilyLabel("custom-model"), "Model");
});

test("picker filters across separators and keeps exact matches first", () => {
  const models = [
    "claude-opus-5-5",
    "claude-opus-5",
    "claude-sonnet-5-5",
    "openai/gpt-5.6-luna",
  ];
  const ids = (query) => Array.from(filterModels(models, query)).join("\n");
  assert.equal(ids(""), models.join("\n"));
  assert.equal(ids("claude opus"), "claude-opus-5-5\nclaude-opus-5");
  assert.equal(ids("openai/gpt"), "openai/gpt-5.6-luna");
  assert.equal(ids("claude-opus-5"), "claude-opus-5\nclaude-opus-5-5");
  assert.equal(ids("missing"), "");
});

test("computeModelPickerLayout respects TopBar clearance and clamps maxHeight", () => {
  const { computeModelPickerLayout } = appRequire("@/components/ModelPicker");

  // Plenty of space below -> opens downward capped at 420
  const normal = computeModelPickerLayout({ top: 200, bottom: 248 }, 900, {
    topInset: 58,
  });
  assert.equal(normal.placement, "bottom");
  assert.equal(normal.maxHeight, 420);

  // Near bottom with ample space below TopBar -> flips upward and respects TopBar inset
  const nearBottom = computeModelPickerLayout({ top: 680, bottom: 728 }, 844, {
    topInset: 58,
  });
  assert.equal(nearBottom.placement, "top");
  assert.equal(nearBottom.maxHeight, 420);

  // Constrained space above TopBar -> clamps available height below TopBar
  const tightTop = computeModelPickerLayout({ top: 290, bottom: 338 }, 480, {
    topInset: 58,
  });
  assert.equal(tightTop.placement, "top");
  assert.equal(tightTop.maxHeight, 216);

  // Tight mobile landscape / keyboard viewport (390x340 with wrapped 108px TopBar) -> never overflows TopBar
  const mobileTight = computeModelPickerLayout({ top: 210, bottom: 258 }, 340, {
    topInset: 108,
  });
  assert.equal(mobileTight.placement, "top");
  assert.equal(mobileTight.maxHeight, 86);

  // Very short viewport -> respects available > 0 without forcing 160px overflow
  const tiny = computeModelPickerLayout({ top: 120, bottom: 168 }, 240, {
    topInset: 58,
  });
  assert.equal(tiny.placement, "bottom");
  assert.equal(tiny.maxHeight, 56);

  // Zero/negative available space -> falls back to 48px minimum shell
  const zeroSpace = computeModelPickerLayout({ top: 70, bottom: 118 }, 120, {
    topInset: 58,
  });
  assert.equal(zeroSpace.maxHeight, 48);
});

test("ModelPicker SSR renders quick-clear button and status feedback cleanly", () => {
  const ModelPicker = appRequire("@/components/ModelPicker").default;

  const populatedHtml = renderToStaticMarkup(
    React.createElement(ModelPicker, {
      id: "model",
      value: "deepseek-v4.1-flash",
      onChange: () => {},
      options: ["deepseek-v4.1-flash", "claude-sonnet-5-5"],
      statusTone: "ok",
      statusMessage: "已获取 2 个模型",
    }),
  );
  assert.match(populatedHtml, /class="model-picker has-clear"/);
  assert.match(populatedHtml, /class="model-action model-clear is-round"/);
  assert.match(populatedHtml, /aria-label="清空模型名称"/);
  assert.match(populatedHtml, /class="model-status is-ok"/);
  assert.match(populatedHtml, /已获取 2 个模型/);

  const emptyDisabledHtml = renderToStaticMarkup(
    React.createElement(ModelPicker, {
      id: "model",
      value: "",
      onChange: () => {},
      options: [],
      disabled: true,
      statusTone: "warn",
      statusMessage: "上游未返回模型，可直接输入实际调用名称。",
    }),
  );
  assert.doesNotMatch(emptyDisabledHtml, /model-clear/);
  assert.doesNotMatch(emptyDisabledHtml, /has-clear/);
  assert.match(emptyDisabledHtml, /class="model-status is-warn"/);

  const multilineErrorHtml = renderToStaticMarkup(
    React.createElement(ModelPicker, {
      id: "model",
      value: "gpt-5",
      onChange: () => {},
      options: [],
      statusTone: "error",
      statusMessage: "拉取模型失败\nHTTP 502 Bad Gateway",
    }),
  );
  assert.match(multilineErrorHtml, /error-fold error-compact/);
  assert.match(multilineErrorHtml, /拉取模型失败/);
});
