"use client";
import { useEffect, useRef, useState } from "react";
import Icon from "@/components/Icon";
import { LANGS, useI18n } from "@/lib/i18n";

export function nextOptionIndex(current: number, key: string, total: number): number {
  if (total <= 0) return -1;
  if (key === "Home") return 0;
  if (key === "End") return total - 1;
  if (key === "ArrowDown") return current < 0 ? 0 : (current + 1) % total;
  if (key === "ArrowUp") return current <= 0 ? total - 1 : current - 1;
  return current;
}

export default function LanguageMenu() {
  const { lang, follow, t, setLang } = useI18n();
  const [open, setOpen] = useState(false);
  const [pendingFocusIndex, setPendingFocusIndex] = useState<number | null>(null);
  const root = useRef<HTMLDivElement>(null);
  const button = useRef<HTMLButtonElement>(null);
  const optionRefs = useRef<(HTMLButtonElement | null)[]>([]);
  const current = LANGS.find((item) => item.id === lang);
  const totalOptions = LANGS.length + 1;
  const selectedIndex = follow ? 0 : Math.max(0, LANGS.findIndex((item) => item.id === lang) + 1);

  useEffect(() => {
    const close = (e: PointerEvent) => {
      if (!root.current?.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener("pointerdown", close);
    return () => document.removeEventListener("pointerdown", close);
  }, []);

  useEffect(() => {
    if (!open || pendingFocusIndex == null) return;
    optionRefs.current[pendingFocusIndex]?.focus();
    setPendingFocusIndex(null);
  }, [open, pendingFocusIndex]);

  function openWithFocus(targetIndex: number) {
    setPendingFocusIndex(targetIndex);
    setOpen(true);
  }

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
        aria-haspopup="listbox"
        aria-expanded={open}
        aria-controls="lang-options"
        aria-label={t("切换语言")}
        title={t("切换语言")}
        onClick={() => setOpen(!open)}
        onKeyDown={(e) => {
          if (e.key === "ArrowDown") {
            e.preventDefault();
            if (!open) openWithFocus(selectedIndex);
            else optionRefs.current[nextOptionIndex(selectedIndex, "ArrowDown", totalOptions)]?.focus();
          } else if (e.key === "ArrowUp") {
            e.preventDefault();
            if (!open) openWithFocus(totalOptions - 1);
            else optionRefs.current[nextOptionIndex(selectedIndex, "ArrowUp", totalOptions)]?.focus();
          }
        }}
      >
        <span>{current?.short || "简体"}</span>
        <Icon name="chevron" width="12" height="12" />
      </button>
      {open && (
        <div
          id="lang-options"
          className="theme-popover lang-popover"
          role="listbox"
          aria-label={t("切换语言")}
          onKeyDown={(e) => {
            if (e.key === "Tab") {
              setOpen(false);
              return;
            }
            if (e.key === "ArrowDown" || e.key === "ArrowUp" || e.key === "Home" || e.key === "End") {
              e.preventDefault();
              const activeIndex = optionRefs.current.findIndex((el) => el === document.activeElement);
              const base = activeIndex >= 0 ? activeIndex : selectedIndex;
              const next = nextOptionIndex(base, e.key, totalOptions);
              optionRefs.current[next]?.focus();
            }
          }}
        >
          <button
            ref={(el) => {
              optionRefs.current[0] = el;
            }}
            type="button"
            role="option"
            aria-selected={follow}
            onClick={() => {
              setLang("auto");
              setOpen(false);
              button.current?.focus();
            }}
          >
            <span>{t("跟随浏览器")}</span>
            {follow && <Icon name="check" width="14" height="14" />}
          </button>
          {LANGS.map((item, idx) => {
            const isSelected = !follow && lang === item.id;
            return (
              <button
                key={item.id}
                ref={(el) => {
                  optionRefs.current[idx + 1] = el;
                }}
                type="button"
                role="option"
                aria-selected={isSelected}
                onClick={() => {
                  setLang(item.id);
                  setOpen(false);
                  button.current?.focus();
                }}
              >
                <span>{item.label}</span>
                {isSelected && <Icon name="check" width="14" height="14" />}
              </button>
            );
          })}
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
