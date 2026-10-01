"use client";
import { useEffect, useRef, useState } from "react";
import Icon from "@/components/Icon";
type Theme = { accent: "orange" | "mint"; mode: "light" | "dark" };
export default function ThemeMenu() {
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
      accent: el.dataset.accent === "mint" ? "mint" : "orange",
      mode: el.dataset.mode === "dark" ? "dark" : "light",
    });
    const close = (e: MouseEvent) => {
      if (!root.current?.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener("mousedown", close);
    return () => document.removeEventListener("mousedown", close);
  }, []);
  function change(next: Theme) {
    setTheme(next);
    document.documentElement.dataset.accent = next.accent;
    document.documentElement.dataset.mode = next.mode;
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
        aria-label="切换主题样式"
        title="切换主题样式"
        onClick={() => setOpen(!open)}
      >
        <span className={"theme-dot theme-dot-" + theme.accent} />
        <span>外观</span>
        <Icon name="chevron" width="12" height="12" />
      </button>
      {open && (
        <div id="theme-options" className="theme-popover">
          <span className="theme-label">配色</span>
          <div className="theme-choices">
            <button
              type="button"
              aria-pressed={theme.accent === "orange"}
              onClick={() => change({ ...theme, accent: "orange" })}
            >
              <i className="theme-dot theme-dot-orange" />
              暖橙
            </button>
            <button
              type="button"
              aria-pressed={theme.accent === "mint"}
              onClick={() => change({ ...theme, accent: "mint" })}
            >
              <i className="theme-dot theme-dot-mint" />
              薄荷绿
            </button>
          </div>
          <span className="theme-label">明暗</span>
          <div className="theme-choices">
            <button
              type="button"
              aria-pressed={theme.mode === "light"}
              onClick={() => change({ ...theme, mode: "light" })}
            >
              <Icon name="sun" width="16" height="16" />
              日间
            </button>
            <button
              type="button"
              aria-pressed={theme.mode === "dark"}
              onClick={() => change({ ...theme, mode: "dark" })}
            >
              <Icon name="moon" width="16" height="16" />
              夜间
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
