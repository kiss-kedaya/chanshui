// keyvault.ts — 上游 API Key 的本地加固存储。
//
// 目的：让用户不必每次回报告页重填 Key，但又不把明文 Key 直接摊在
// localStorage 里（任何同源脚本、扩展、随手一读都能拿到）。
//
// 做法：
//   1. 在 IndexedDB 里生成一把 AES-GCM 密钥，extractable=false。
//      「不可提取」意味着脚本拿不到密钥字节——crypto.subtle 只肯用它
//      加解密，不肯把它导出。所以就算有人翻 IndexedDB，也只看到一个
//      CryptoKey 句柄，不是原始密钥。
//   2. 密文（含随机 IV）存 localStorage，可以随便看，没有密钥解不开。
//
// 必须说清的边界：这挡的是「顺手读 localStorage 的脚本/扩展」。能在本页
// 执行 JS 的攻击者仍然可以调用本模块的解密函数拿到明文——这是浏览器
// 模型的限制，任何纯前端方案都躲不掉。真要强隔离，Key 就不该存。
//
// Key 只在本地加解密，永远不会随请求发到本站服务端。

const DB_NAME = "chanshui-vault";
const STORE = "keys";
const KEY_ID = "upstream-key";
const CIPHER_KEY = "chanshui-key-cipher";

let cachedKey: CryptoKey | null = null;
let cachedPlain: string | null = null;
let loadPromise: Promise<string> | null = null;

function hasSubtle(): boolean {
  return (
    typeof window !== "undefined" &&
    typeof crypto !== "undefined" &&
    typeof crypto.subtle !== "undefined" &&
    typeof indexedDB !== "undefined"
  );
}

function openDB(): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    const req = indexedDB.open(DB_NAME, 1);
    req.onupgradeneeded = () => {
      const db = req.result;
      if (!db.objectStoreNames.contains(STORE)) db.createObjectStore(STORE);
    };
    req.onsuccess = () => resolve(req.result);
    req.onerror = () => reject(req.error);
  });
}

function idbGet<T>(db: IDBDatabase, key: string): Promise<T | undefined> {
  return new Promise((resolve, reject) => {
    const tx = db.transaction(STORE, "readonly");
    const req = tx.objectStore(STORE).get(key);
    req.onsuccess = () => resolve(req.result as T | undefined);
    req.onerror = () => reject(req.error);
  });
}

function idbPut(db: IDBDatabase, key: string, value: unknown): Promise<void> {
  return new Promise((resolve, reject) => {
    const tx = db.transaction(STORE, "readwrite");
    tx.objectStore(STORE).put(value, key);
    tx.oncomplete = () => resolve();
    tx.onerror = () => reject(tx.error);
  });
}

// 取（或首次创建）不可提取的 AES-GCM 密钥。
async function deviceKey(): Promise<CryptoKey> {
  if (cachedKey) return cachedKey;
  const db = await openDB();
  let key = await idbGet<CryptoKey>(db, KEY_ID);
  if (!key) {
    key = await crypto.subtle.generateKey({ name: "AES-GCM", length: 256 }, false, [
      "encrypt",
      "decrypt",
    ]);
    await idbPut(db, KEY_ID, key);
  }
  cachedKey = key;
  return key;
}

function toB64(bytes: Uint8Array): string {
  let s = "";
  for (let i = 0; i < bytes.length; i++) s += String.fromCharCode(bytes[i]);
  return btoa(s);
}

function fromB64(value: string): Uint8Array<ArrayBuffer> {
  const text = atob(value);
  const out = new Uint8Array(new ArrayBuffer(text.length));
  for (let i = 0; i < text.length; i++) out[i] = text.charCodeAt(i);
  return out;
}

// saveKey 加密并落盘。返回是否成功；失败时调用方应视为「没记住」。
export async function saveKey(plain: string): Promise<boolean> {
  const value = plain.trim();
  cachedPlain = value;
  if (!value) {
    await clearKey();
    return true;
  }
  if (!hasSubtle()) return false;
  try {
    const key = await deviceKey();
    const iv = crypto.getRandomValues(new Uint8Array(12));
    const ct = await crypto.subtle.encrypt(
      { name: "AES-GCM", iv },
      key,
      new TextEncoder().encode(value),
    );
    window.localStorage.setItem(
      CIPHER_KEY,
      JSON.stringify({ v: 1, iv: toB64(iv), ct: toB64(new Uint8Array(ct)) }),
    );
    return true;
  } catch {
    return false;
  }
}

// loadKey 读出并解密。密钥丢失（换了浏览器/清了 IndexedDB）时返回空串，
// 调用方回退到让用户重新输入。
export async function loadKey(): Promise<string> {
  if (cachedPlain !== null) return cachedPlain;
  if (!hasSubtle()) return "";
  if (loadPromise) return loadPromise;
  loadPromise = (async () => {
    try {
      const raw = window.localStorage.getItem(CIPHER_KEY);
      if (!raw) return "";
      const env = JSON.parse(raw) as { v?: number; iv?: string; ct?: string };
      if (env?.v !== 1 || !env.iv || !env.ct) return "";
      const key = await deviceKey();
      const plain = await crypto.subtle.decrypt(
        { name: "AES-GCM", iv: fromB64(env.iv) },
        key,
        fromB64(env.ct),
      );
      cachedPlain = new TextDecoder().decode(plain);
      return cachedPlain;
    } catch {
      // 解密失败通常是密钥被清掉或密文损坏，当作「没存过」。
      window.localStorage.removeItem(CIPHER_KEY);
      return "";
    } finally {
      loadPromise = null;
    }
  })();
  return loadPromise;
}

// clearKey 抹掉密文与内存缓存。用户点「清除已保存的 Key」时调用。
export async function clearKey(): Promise<void> {
  cachedPlain = "";
  try {
    window.localStorage.removeItem(CIPHER_KEY);
  } catch {}
  try {
    const db = await openDB();
    await new Promise<void>((resolve) => {
      const tx = db.transaction(STORE, "readwrite");
      tx.objectStore(STORE).delete(KEY_ID);
      tx.oncomplete = () => resolve();
      tx.onerror = () => resolve();
    });
  } catch {}
  cachedKey = null;
}
