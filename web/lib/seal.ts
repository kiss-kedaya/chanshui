function bytesToBase64(bytes: Uint8Array) {
  let text = "";
  for (let i = 0; i < bytes.length; i++) text += String.fromCharCode(bytes[i]);
  return btoa(text);
}

function base64ToBytes(value: string) {
  const text = atob(value);
  const bytes = new Uint8Array(text.length);
  for (let i = 0; i < text.length; i++) bytes[i] = text.charCodeAt(i);
  return bytes;
}

let publicKey: CryptoKey | null = null;

export function resetSealKey() {
  publicKey = null;
}

function isNetworkFailure(error: unknown): boolean {
  if (!error || typeof error !== "object") return false;
  const err = error as { name?: string; message?: string };
  const msg = (err.message || "").toLowerCase();
  return (
    err.name === "TypeError" ||
    err.name === "AbortError" ||
    msg.includes("networkerror") ||
    msg.includes("failed to fetch") ||
    msg.includes("load failed")
  );
}

async function fetchSealWithRetry(url: string): Promise<Response> {
  const delays = [0, 350, 900];
  let lastError: unknown;
  for (let attempt = 0; attempt < delays.length; attempt++) {
    if (delays[attempt] > 0) {
      await new Promise((resolve) => setTimeout(resolve, delays[attempt]));
    }
    const controller = typeof AbortController === "function" ? new AbortController() : null;
    const timer = controller ? setTimeout(() => controller.abort(), 12000) : undefined;
    try {
      const response = await fetch(url, {
        cache: "no-store",
        ...(controller ? { signal: controller.signal } : {}),
      });
      if (timer !== undefined) clearTimeout(timer);
      return response;
    } catch (error) {
      if (timer !== undefined) clearTimeout(timer);
      lastError = error;
      if (!isNetworkFailure(error) || attempt === delays.length - 1) {
        throw Error("网络连接中断或超时，请稍后重试");
      }
    }
  }
  throw lastError instanceof Error ? lastError : Error("网络连接中断或超时，请稍后重试");
}

async function loadKey(base: string) {
  if (publicKey) return publicKey;
  const response = await fetchSealWithRetry(base + "/api/seal");
  let data: { spki?: string; error?: string };
  try {
    data = await response.json();
  } catch {
    throw Error("加密通道未就绪，请稍后重试");
  }
  if (!response.ok || !data.spki) throw Error(data.error || "加密通道未就绪");
  publicKey = await crypto.subtle.importKey(
    "spki",
    base64ToBytes(data.spki),
    { name: "RSA-OAEP", hash: "SHA-256" },
    false,
    ["wrapKey"],
  );
  return publicKey;
}

export async function sealJSON(base: string, value: unknown) {
  const key = await loadKey(base);
  const aes = await crypto.subtle.generateKey({ name: "AES-GCM", length: 256 }, true, ["encrypt"]);
  const iv = crypto.getRandomValues(new Uint8Array(12));
  const ciphertext = await crypto.subtle.encrypt(
    { name: "AES-GCM", iv },
    aes,
    new TextEncoder().encode(JSON.stringify(value)),
  );
  const wrapped = await crypto.subtle.wrapKey("raw", aes, key, { name: "RSA-OAEP" });
  return JSON.stringify({
    v: 1,
    ek: bytesToBase64(new Uint8Array(wrapped)),
    iv: bytesToBase64(iv),
    ct: bytesToBase64(new Uint8Array(ciphertext)),
  });
}
