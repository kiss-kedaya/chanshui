const familyRules: Array<[RegExp, string]> = [
  [/claude|anthropic/, "Claude"],
  [/gemini|(^|\/)google[\/-]/, "Gemini"],
  [/grok|(^|\/)x-ai[\/-]|xai\//, "Grok"],
  [/deepseek/, "DeepSeek"],
  [/glm|(^|\/)z-ai[\/-]|zhipu/, "GLM"],
  [/qwen|tongyi/, "Qwen"],
  [/kimi|moonshot/, "Kimi"],
  [/gpt|openai|codex|(^|[-_/.\s])o[1-9]([-_/.\s]|$)/, "GPT"],
];

export function modelFamilyLabel(model: string): string {
  const id = model.trim().toLowerCase();
  for (const [pattern, label] of familyRules) {
    if (pattern.test(id)) return label;
  }
  return "Model";
}

function normalize(value: string): string {
  return value
    .trim()
    .toLowerCase()
    .replace(/[\s/_.]+/g, "-")
    .replace(/^-+|-+$/g, "");
}

export function filterModels(models: string[], query: string): string[] {
  const q = normalize(query);
  if (!q) return models;
  const terms = q.split("-").filter(Boolean);
  const scored: Array<{ model: string; score: number; index: number }> = [];
  models.forEach((model, index) => {
    const id = normalize(model);
    if (!terms.every((term) => id.includes(term))) return;
    let score = 0;
    if (id === q) score = 3;
    else if (id.startsWith(q)) score = 2;
    else if (id.split("-").some((part) => part.startsWith(q))) score = 1;
    scored.push({ model, score, index });
  });
  return scored
    .sort((a, b) => b.score - a.score || a.index - b.index)
    .map((entry) => entry.model);
}
