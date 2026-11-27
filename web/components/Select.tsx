"use client";
import { useI18n } from "@/lib/i18n";
import {
  Children,
  CSSProperties,
  isValidElement,
  ReactNode,
  useCallback,
  useEffect,
  useId,
  useRef,
  useState,
} from "react";
import Icon from "@/components/Icon";

type Option = { value: string; label: string };

export type SelectLayout = {
  placement: "top" | "bottom";
  popoverMaxHeight: number;
  optionsMaxHeight: number;
};

export function computeSelectLayout(
  rect: { top: number; bottom: number },
  viewportHeight: number,
  options?: {
    topInset?: number;
    isInDialog?: boolean;
    hasFilter?: boolean;
    preferredHeight?: number;
    minHeight?: number;
    maxHeight?: number;
    gap?: number;
    pad?: number;
  },
): SelectLayout {
  const isInDialog = options?.isInDialog ?? false;
  const topInset = Math.max(0, options?.topInset ?? (isInDialog ? 8 : 58));
  const chromeReserve = 16 + (options?.hasFilter ? 48 : 0);
  const maxCap = options?.maxHeight ?? 260;
  const preferredHeight = options?.preferredHeight ?? maxCap + chromeReserve;
  const gap = options?.gap ?? 6;
  const pad = options?.pad ?? 8;
  const above = Math.max(0, rect.top - topInset - gap - pad);
  const below = Math.max(0, viewportHeight - rect.bottom - gap - pad);
  const placement: "top" | "bottom" =
    below < preferredHeight && above > below ? "top" : "bottom";
  const available = Math.floor(placement === "top" ? above : below);
  const popoverMaxHeight =
    available <= 0 ? 48 : Math.min(maxCap + chromeReserve, available);
  const optionsMaxHeight = Math.min(
    maxCap,
    Math.max(32, popoverMaxHeight - chromeReserve),
  );
  return { placement, popoverMaxHeight, optionsMaxHeight };
}

export function nextSelectOptionIndex(
  currentIndex: number,
  key: string,
  count: number,
): number {
  if (count <= 0) return -1;
  switch (key) {
    case "ArrowDown":
      return currentIndex < 0 ? 0 : (currentIndex + 1) % count;
    case "ArrowUp":
      return currentIndex < 0 ? count - 1 : (currentIndex - 1 + count) % count;
    case "Home":
      return 0;
    case "End":
      return count - 1;
    default:
      return currentIndex;
  }
}

function measureTopInset(isInDialog: boolean): number {
  if (isInDialog) return 8;
  if (typeof document === "undefined") return 58;
  const topbar = document.querySelector<HTMLElement>(".app-topbar");
  if (!topbar) return 58;
  const bottom = topbar.getBoundingClientRect().bottom;
  return Number.isFinite(bottom) && bottom > 0 ? Math.ceil(bottom) : 58;
}

export default function Select({
  id,
  value,
  onChange,
  children,
  disabled = false,
  loading = false,
  placeholder,
  filterLabel,
  filterPlaceholder,
}: {
  id?: string;
  value: string;
  onChange: (event: { target: { value: string } }) => void;
  children: ReactNode;
  disabled?: boolean;
  loading?: boolean;
  placeholder?: string;
  filterLabel?: string;
  filterPlaceholder?: string;
}) {
  const { t: tr } = useI18n();
  const generated = useId(),
    name = id || generated,
    root = useRef<HTMLDivElement>(null),
    trigger = useRef<HTMLButtonElement>(null),
    filterInput = useRef<HTMLInputElement>(null),
    pendingFocus = useRef<"filter" | "first" | "last" | "selected" | null>(null);
  const [open, setOpen] = useState(false),
    [query, setQuery] = useState(""),
    [layout, setLayout] = useState<SelectLayout>({
      placement: "bottom",
      popoverMaxHeight: 276,
      optionsMaxHeight: 260,
    });
  const opts: Option[] = [];
  function collect(nodes: ReactNode) {
    Children.forEach(nodes, (node) => {
      if (!isValidElement<{ value?: string; children?: ReactNode }>(node))
        return;
      if (node.type === "option")
        opts.push({
          value: String(node.props.value ?? ""),
          label: String(node.props.children ?? ""),
        });
      else collect(node.props.children);
    });
  }
  collect(children);
  const hasFilter = !loading && opts.length > 6;
  const visible = opts.filter((o) =>
    o.label.toLowerCase().includes(query.toLowerCase()),
  );
  const current = opts.find((o) => o.value === value);

  const updateLayout = useCallback(() => {
    const rect = root.current?.getBoundingClientRect();
    if (!rect || typeof window === "undefined") return;
    const isInDialog = Boolean(root.current?.closest("dialog"));
    const viewport =
      window.visualViewport?.height ?? window.innerHeight;
    setLayout(
      computeSelectLayout(rect, viewport, {
        isInDialog,
        topInset: measureTopInset(isInDialog),
        hasFilter,
      }),
    );
  }, [hasFilter]);

  useEffect(() => {
    if (!open) return;
    const closeOnOutside = (e: PointerEvent) => {
      if (!root.current?.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener("pointerdown", closeOnOutside);
    return () => document.removeEventListener("pointerdown", closeOnOutside);
  }, [open]);

  useEffect(() => {
    if (!open) return;
    updateLayout();
    window.addEventListener("resize", updateLayout);
    window.addEventListener("scroll", updateLayout, { capture: true, passive: true });
    window.visualViewport?.addEventListener("resize", updateLayout);
    window.visualViewport?.addEventListener("scroll", updateLayout, { passive: true });
    return () => {
      window.removeEventListener("resize", updateLayout);
      window.removeEventListener("scroll", updateLayout, true);
      window.visualViewport?.removeEventListener("resize", updateLayout);
      window.visualViewport?.removeEventListener("scroll", updateLayout);
    };
  }, [open, updateLayout]);

  useEffect(() => {
    if (disabled) setOpen(false);
  }, [disabled]);

  useEffect(() => {
    if (!open) return;
    const mode = pendingFocus.current;
    pendingFocus.current = null;
    if (!mode) return;
    const id = requestAnimationFrame(() => {
      if (mode === "filter" && filterInput.current) {
        filterInput.current.focus({ preventScroll: true });
        return;
      }
      const buttons = Array.from(
        root.current?.querySelectorAll<HTMLButtonElement>('[role="option"]') ||
          [],
      );
      if (!buttons.length) return;
      if (mode === "last") {
        buttons[buttons.length - 1]?.focus({ preventScroll: true });
      } else if (mode === "selected") {
        const selectedBtn = buttons.find(
          (btn) => btn.getAttribute("aria-selected") === "true",
        );
        (selectedBtn || buttons[0])?.focus({ preventScroll: true });
      } else {
        buttons[0]?.focus({ preventScroll: true });
      }
    });
    return () => cancelAnimationFrame(id);
  }, [open]);

  function focusOptionAt(index: number) {
    const buttons = Array.from(
      root.current?.querySelectorAll<HTMLButtonElement>('[role="option"]') ||
        [],
    );
    if (!buttons.length) return;
    const clamped = Math.max(0, Math.min(buttons.length - 1, index));
    buttons[clamped]?.focus({ preventScroll: true });
  }

  function choose(o: Option) {
    onChange({ target: { value: o.value } });
    setOpen(false);
    setQuery("");
    trigger.current?.focus({ preventScroll: true });
  }

  return (
    <div
      className="custom-select"
      ref={root}
      onKeyDown={(e) => {
        if (e.key === "Escape" && open) {
          e.stopPropagation();
          setOpen(false);
          trigger.current?.focus({ preventScroll: true });
        }
        if (e.key === "Tab") setOpen(false);
      }}
    >
      <button
        ref={trigger}
        id={id}
        type="button"
        role="combobox"
        aria-expanded={open}
        aria-busy={loading}
        aria-controls={name + "-list"}
        disabled={disabled}
        onClick={() => {
          if (disabled) return;
          if (open) {
            setOpen(false);
            return;
          }
          pendingFocus.current = hasFilter ? "filter" : "selected";
          updateLayout();
          setQuery("");
          setOpen(true);
        }}
        onKeyDown={(e) => {
          if (e.key === "Escape" && open) {
            e.preventDefault();
            e.stopPropagation();
            setOpen(false);
            return;
          }
          if (e.key === "ArrowDown") {
            e.preventDefault();
            if (!open) {
              pendingFocus.current = "first";
              updateLayout();
              setQuery("");
              setOpen(true);
            } else {
              focusOptionAt(0);
            }
          } else if (e.key === "ArrowUp") {
            e.preventDefault();
            if (!open) {
              pendingFocus.current = "last";
              updateLayout();
              setQuery("");
              setOpen(true);
            } else {
              const buttons =
                root.current?.querySelectorAll<HTMLButtonElement>(
                  '[role="option"]',
                );
              if (buttons?.length) focusOptionAt(buttons.length - 1);
            }
          }
        }}
      >
        <span className={current ? "select-value" : "select-placeholder"}>
          {current ? current.label : placeholder || tr("请选择")}
        </span>
        <span className="select-chevron" aria-hidden="true">
          {loading ? (
            <Icon name="spinner" className="icon-spin" />
          ) : (
            <Icon name="chevron" width="16" height="16" />
          )}
        </span>
      </button>
      {open && !disabled && (
        <div
          className="select-popover"
          data-placement={layout.placement}
          style={
            {
              maxHeight: layout.popoverMaxHeight,
              "--select-max-h": `${layout.optionsMaxHeight}px`,
            } as CSSProperties
          }
        >
          {hasFilter && (
            <input
              ref={filterInput}
              aria-label={filterLabel || tr("筛选选项")}
              placeholder={filterPlaceholder || tr("输入关键字筛选…")}
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "ArrowDown") {
                  e.preventDefault();
                  focusOptionAt(0);
                } else if (e.key === "ArrowUp") {
                  e.preventDefault();
                  const buttons =
                    root.current?.querySelectorAll<HTMLButtonElement>(
                      '[role="option"]',
                    );
                  if (buttons?.length) focusOptionAt(buttons.length - 1);
                } else if (e.key === "Enter") {
                  e.preventDefault();
                  if (visible.length === 1) choose(visible[0]);
                  else if (visible.length > 0) focusOptionAt(0);
                } else if (e.key === "Escape") {
                  e.preventDefault();
                  e.stopPropagation();
                  setOpen(false);
                  trigger.current?.focus({ preventScroll: true });
                }
              }}
            />
          )}
          <div
            id={name + "-list"}
            role="listbox"
            aria-labelledby={id}
            className="select-options"
          >
            {loading ? (
              <div className="models-loading" role="status">
                <Icon
                  name="spinner"
                  className="icon-spin"
                  width="28"
                  height="28"
                />
                <span>{tr("正在获取模型…")}</span>
              </div>
            ) : visible.length ? (
              visible.map((o) => (
                <button
                  type="button"
                  role="option"
                  aria-selected={o.value === value}
                  key={o.value}
                  onClick={() => choose(o)}
                  onKeyDown={(e) => {
                    if (e.key === "Escape") {
                      e.preventDefault();
                      e.stopPropagation();
                      setOpen(false);
                      trigger.current?.focus({ preventScroll: true });
                      return;
                    }
                    if (
                      e.key === "ArrowDown" ||
                      e.key === "ArrowUp" ||
                      e.key === "Home" ||
                      e.key === "End"
                    ) {
                      e.preventDefault();
                      const buttons = Array.from(
                          root.current?.querySelectorAll<HTMLButtonElement>(
                            '[role="option"]',
                          ) || [],
                        ),
                        i = buttons.indexOf(e.currentTarget);
                      if (e.key === "ArrowUp" && i === 0 && filterInput.current) {
                        filterInput.current.focus({ preventScroll: true });
                        return;
                      }
                      const nextIdx = nextSelectOptionIndex(
                        i,
                        e.key,
                        buttons.length,
                      );
                      buttons[nextIdx]?.focus({ preventScroll: true });
                    }
                  }}
                >
                  <span>{o.label}</span>
                  {o.value === value && (
                    <Icon
                      name="check"
                      className="select-check"
                      width="15"
                      height="15"
                    />
                  )}
                </button>
              ))
            ) : (
              <p className="select-empty">{tr("没有匹配的模型")}</p>
            )}
          </div>
        </div>
      )}
    </div>
  );
}
