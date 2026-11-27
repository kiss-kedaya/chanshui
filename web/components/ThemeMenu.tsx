"use client";
import { useEffect, useRef, useState } from "react";
import Icon from "@/components/Icon";
import { useI18n } from "@/lib/i18n";

type Accent = "orange" | "mint" | "mono";
type Mode = "light" | "dark";
type Theme = { accent: Accent; mode: Mode };

function parseAccent(raw?: string): Accent {
  if (raw === "mint" || raw === "mono") return raw;
  return "orange";
}

export default function ThemeMenu() {
  const { t: tr } = useI18n();
  const [theme, setTheme] = useState<Theme>({
      accent: "orange",
      mode: "light",
    }),
    [open, setOpen] = useState(false);
  const root = useRef<HTMLDivElement>(null),
    button = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    const el = document.documentElement;
    setTheme({
      accent: parseAccent(el.dataset.accent),
      mode: el.dataset.mode === "dark" ? "dark" : "light",
    });
    const close = (e: PointerEvent) => {
      if (!root.current?.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener("pointerdown", close);
    return () => document.removeEventListener("pointerdown", close);
  }, []);
  function change(next: Theme) {
    setTheme(next);
    document.documentElement.dataset.accent = next.accent;
    document.documentElement.dataset.mode = next.mode;
    document.documentElement.style.colorScheme = next.mode;
    try {
      localStorage.setItem("chanshui-theme", JSON.stringify(next));
    } catch {}
  }
  return (
    <div
      className="theme-menu"
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
        aria-controls="theme-options"
        aria-label={tr("切换主题样式")}
        title={tr("切换主题样式")}
        onClick={() => setOpen(!open)}
      >
        <span className={"theme-dot theme-dot-" + theme.accent} />
        <span className="tool-label">{tr("外观")}</span>
        <Icon name="chevron" width="12" height="12" />
      </button>
      {open && (
        <div id="theme-options" className="theme-popover">
          <span className="theme-label">{tr("配色")}</span>
          <div className="theme-choices theme-choices-3">
            <button
              type="button"
              aria-pressed={theme.accent === "orange"}
              onClick={() => change({ ...theme, accent: "orange" })}
            >
              <i className="theme-dot theme-dot-orange" />
              <span>{tr("暖橙")}</span>
            </button>
            <button
              type="button"
              aria-pressed={theme.accent === "mint"}
              onClick={() => change({ ...theme, accent: "mint" })}
            >
              <i className="theme-dot theme-dot-mint" />
              <span>{tr("薄荷绿")}</span>
            </button>
            <button
              type="button"
              aria-pressed={theme.accent === "mono"}
              onClick={() => change({ ...theme, accent: "mono" })}
            >
              <i className="theme-dot theme-dot-mono" />
              <span>{tr("黑白灰")}</span>
            </button>
          </div>
          <span className="theme-label">{tr("明暗")}</span>
          <div className="theme-choices">
            <button
              type="button"
              aria-pressed={theme.mode === "light"}
              onClick={() => change({ ...theme, mode: "light" })}
            >
              <Icon name="sun" width="15" height="15" />
              <span>{tr("日间")}</span>
            </button>
            <button
              type="button"
              aria-pressed={theme.mode === "dark"}
              onClick={() => change({ ...theme, mode: "dark" })}
            >
              <Icon name="moon" width="15" height="15" />
              <span>{tr("夜间")}</span>
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
