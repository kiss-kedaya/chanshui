"use client";
import { createContext, ReactNode, useContext, useEffect, useMemo, useRef, useState } from "react";
import { rows } from "@/lib/i18n-rows";

export const LANGS = [
  { id: "zh", label: "简体中文", short: "简体", html: "zh-CN" },
  { id: "zh-Hant", label: "繁體中文", short: "繁體", html: "zh-Hant" },
  { id: "en", label: "English", short: "EN", html: "en" },
  { id: "ja", label: "日本語", short: "日本語", html: "ja" },
  { id: "ko", label: "한국어", short: "한국어", html: "ko" },
  { id: "ru", label: "Русский", short: "RU", html: "ru" },
] as const;

export type Lang = (typeof LANGS)[number]["id"];
type Dict = Record<string, string>;

const INTL_LOCALES: Record<Lang, string> = {
  zh: "zh-CN",
  "zh-Hant": "zh-TW",
  en: "en-US",
  ja: "ja-JP",
  ko: "ko-KR",
  ru: "ru-RU",
};

const tails = [
  " 本组发生重试，首次成功样本可能不是冷缓存；重试用量未计入样本命中率。",
  " 检测发生过重试，首次成功回包可能不再进缓存；缓存率未因重试被改写。",
];
const heads = ["行为探针失败："];

function build(index: number): Dict {
  const out: Dict = Object.create(null);
  for (const row of rows) out[row[0]] = row[index];
  return out;
}

const dict: Record<Exclude<Lang, "zh">, Dict> = {
  en: build(1),
  "zh-Hant": build(2),
  ja: build(3),
  ko: build(4),
  ru: build(5),
};

export function isLang(value: string | null): value is Lang {
  return LANGS.some((item) => item.id === value);
}

export function detectLang(list: readonly string[]): Lang {
  for (const raw of list) {
    const n = raw.toLowerCase();
    if (n.startsWith("zh")) return /hant|tw|hk|mo/.test(n) ? "zh-Hant" : "zh";
    if (n.startsWith("en")) return "en";
    if (n.startsWith("ja")) return "ja";
    if (n.startsWith("ko")) return "ko";
    if (n.startsWith("ru")) return "ru";
  }
  return "zh";
}

export function htmlLang(lang: Lang) {
  return LANGS.find((item) => item.id === lang)?.html || "zh-CN";
}

export function intlLocale(value?: string | null): string {
  if (!value) return "zh-CN";
  if (value in INTL_LOCALES) return INTL_LOCALES[value as Lang];
  const lower = value.toLowerCase();
  if (lower === "zh-cn" || lower.startsWith("zh-hans")) return "zh-CN";
  if (
    lower === "zh-tw" ||
    lower === "zh-hant" ||
    lower.startsWith("zh-hk") ||
    lower.startsWith("zh-mo")
  ) {
    return "zh-TW";
  }
  if (lower.startsWith("en")) return "en-US";
  if (lower.startsWith("ja")) return "ja-JP";
  if (lower.startsWith("ko")) return "ko-KR";
  if (lower.startsWith("ru")) return "ru-RU";
  return "zh-CN";
}

export function formatLocaleNumber(
  value: number,
  localeOrLang?: string | null,
  options?: Intl.NumberFormatOptions,
): string {
  const safe = Number.isFinite(value) ? value : 0;
  return new Intl.NumberFormat(intlLocale(localeOrLang), options).format(safe);
}

export function formatLocaleDate(
  input: string | number | Date,
  localeOrLang?: string | null,
  options?: Intl.DateTimeFormatOptions,
): string {
  const date = input instanceof Date ? input : new Date(input);
  if (Number.isNaN(date.getTime())) return typeof input === "string" ? input : "";
  return new Intl.DateTimeFormat(intlLocale(localeOrLang), options).format(date);
}

export function translate(lang: Lang, source: string, vars?: Record<string, string | number>) {
  let text = source || "";
  if (lang !== "zh" && text) {
    const table = dict[lang];
    const direct = Object.hasOwn(table, text) ? table[text] : undefined;
    if (typeof direct === "string" && direct) text = direct;
    else {
      const head = heads.find((item) => text.startsWith(item) && text !== item);
      const tail = tails.find((item) => text.endsWith(item) && text !== item);
      if (head) {
        const headVal = Object.hasOwn(table, head) ? table[head] : undefined;
        text = (typeof headVal === "string" && headVal ? headVal : head) + text.slice(head.length);
      } else if (tail) {
        const tailVal = Object.hasOwn(table, tail) ? table[tail] : undefined;
        text = translate(lang, text.slice(0, -tail.length)) + (typeof tailVal === "string" && tailVal ? tailVal : tail);
      }
    }
  }
  if (vars) {
    for (const [key, value] of Object.entries(vars)) text = text.split("{" + key + "}").join(String(value));
  }
  return text;
}

type I18nValue = {
  lang: Lang;
  follow: boolean;
  t: (source: string, vars?: Record<string, string | number>) => string;
  setLang: (lang: Lang | "auto") => void;
  locale: string;
};

const I18nContext = createContext<I18nValue>({
  lang: "zh",
  follow: true,
  t: (source, vars) => {
    let text = source;
    if (vars) {
      for (const [key, value] of Object.entries(vars)) {
        text = text.split("{" + key + "}").join(String(value));
      }
    }
    return text;
  },
  setLang: () => {},
  locale: "zh-CN",
});

export function I18nProvider({ children }: { children: ReactNode }) {
  const [lang, setLangState] = useState<Lang>("zh");
  const [follow, setFollow] = useState(true);
  const mounted = useRef(false);
  useEffect(() => {
    let saved: string | null = null;
    try { saved = localStorage.getItem("chanshui-lang"); } catch {}
    const next = isLang(saved)
      ? saved
      : detectLang(navigator.languages?.length ? navigator.languages : [navigator.language || "zh"]);
    mounted.current = true;
    setLangState(next);
    setFollow(!isLang(saved));
    document.documentElement.lang = htmlLang(next);
    document.documentElement.dataset.lang = next;
  }, []);
  useEffect(() => {
    if (!mounted.current) return;
    document.documentElement.lang = htmlLang(lang);
    document.documentElement.dataset.lang = lang;
  }, [lang]);
  const value = useMemo<I18nValue>(() => ({
    lang,
    follow,
    locale: intlLocale(lang),
    t: (source, vars) => translate(lang, source, vars),
    setLang: (next) => {
      if (next === "auto") {
        try { localStorage.removeItem("chanshui-lang"); } catch {}
        const detected = detectLang(navigator.languages?.length ? navigator.languages : [navigator.language || "zh"]);
        setFollow(true);
        setLangState(detected);
        return;
      }
      try { localStorage.setItem("chanshui-lang", next); } catch {}
      setFollow(false);
      setLangState(next);
    },
  }), [lang, follow]);
  return <I18nContext.Provider value={value}>{children}</I18nContext.Provider>;
}

export function useI18n() {
  return useContext(I18nContext);
}
