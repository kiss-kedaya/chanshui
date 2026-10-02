"use client";
import { useEffect, useRef, useState } from "react";
import Icon from "@/components/Icon";
import { LANGS, useI18n } from "@/lib/i18n";

export default function LanguageMenu() {
  const { lang, follow, t, setLang } = useI18n();
  const [open, setOpen] = useState(false);
  const root = useRef<HTMLDivElement>(null);
  const button = useRef<HTMLButtonElement>(null);
  const current = LANGS.find((item) => item.id === lang);
  useEffect(() => {
    const close = (e: MouseEvent) => {
      if (!root.current?.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener("mousedown", close);
    return () => document.removeEventListener("mousedown", close);
  }, []);
  return (
    <div
      className="lang-menu"
      ref={root}
      onKeyDown={(e) => {
        if (e.key === "Escape") {
          setOpen(false);
          button.current?.focus();
        }
      }}
    >
      <button
        ref={button}
        className="theme-trigger"
        type="button"
        aria-expanded={open}
        aria-controls="lang-options"
        aria-label={t("切换语言")}
        title={t("切换语言")}
        onClick={() => setOpen(!open)}
      >
        <span>{current?.short || "简体"}</span>
        <Icon name="chevron" width="12" height="12" />
      </button>
      {open && (
        <div id="lang-options" className="theme-popover lang-popover" role="listbox" aria-label={t("切换语言")}>
          <button type="button" aria-pressed={follow} onClick={() => { setLang("auto"); setOpen(false); }}>
            {t("跟随浏览器")}
          </button>
          {LANGS.map((item) => (
            <button
              key={item.id}
              type="button"
              aria-pressed={!follow && lang === item.id}
              onClick={() => { setLang(item.id); setOpen(false); }}
            >
              {item.label}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}

export function SkipLink() {
  const { t } = useI18n();
  return (
    <a href="#main-content" className="skip-link">
      {t("跳到主内容")}
    </a>
  );
}
