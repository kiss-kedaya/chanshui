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

async function loadKey(base: string) {
  if (publicKey) return publicKey;
  const response = await fetch(base + "/api/seal", { cache: "no-store" });
  const data = await response.json();
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
