// visitor.ts — 浏览器访客标识。
//
// 用途：把一个检测任务绑定到“提交它的这个浏览器”，这样同一个浏览器以后回来
// 仍能重试/查看自己的任务，即使当时那次会话的临时 jobkey 已经不在。
//
// 注意区分两样东西：
//   1. 用户填写的上游 API Key —— 绝不落盘、绝不存储（见 api.ts 的 sealJSON）。
//   2. 访客标识 —— 是“这台浏览器”的匿名编号，落盘是刻意的，用来证明任务归属。
//      它不是凭据，不携带任何密钥，无法用来访问上游。
//
// 生成用 crypto.randomUUID()；不可用时退回随机字节。整个值只在本地生成，
// 服务端只保存它的 sha256，不保存明文。

const key = "chanshui-visitor";
const validVisitor = (value: unknown): value is string =>
  typeof value === "string" && /^[a-f0-9]{32}$/.test(value);

function randomVisitor(): string {
  try {
    if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function") {
      return crypto.randomUUID().replace(/-/g, "").slice(0, 32);
    }
    if (typeof crypto !== "undefined" && crypto.getRandomValues) {
      const bytes = new Uint8Array(16);
      crypto.getRandomValues(bytes);
      return Array.from(bytes, (b) => b.toString(16).padStart(2, "0")).join("");
    }
  } catch {}
  // 最后兜底：不依赖 crypto 的弱随机，仅用于极老浏览器；仍然只当作匿名编号。
  let out = "";
  for (let i = 0; i < 32; i++) out += Math.floor(Math.random() * 16).toString(16);
  return out;
}

let cached = "";

// visitorID 返回本浏览器的稳定访客标识，首次调用时生成并落盘。
export function visitorID(): string {
  if (cached) return cached;
  try {
    if (typeof window !== "undefined") {
      const existing = window.localStorage.getItem(key);
      if (validVisitor(existing)) {
        cached = existing;
        return cached;
      }
      const created = randomVisitor();
      window.localStorage.setItem(key, created);
      cached = created;
      return cached;
    }
  } catch {}
  // localStorage 不可用（隐私模式等）：本次会话内保持稳定即可。
  cached = randomVisitor();
  return cached;
}
