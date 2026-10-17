const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const vm = require("node:vm");
const ts = require("typescript");

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
