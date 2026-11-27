"use client";
import { useCallback, useEffect, useId, useMemo, useRef, useState } from "react";
import { useI18n } from "@/lib/i18n";
import { filterModels, modelFamilyLabel } from "@/lib/model-picker";
import ErrorFold from "@/components/ErrorFold";
import Icon from "@/components/Icon";

export type ModelPickerLayout = {
  placement: "top" | "bottom";
  maxHeight: number;
};

export function computeModelPickerLayout(
  rect: { top: number; bottom: number },
  viewportHeight: number,
  options?: {
    topInset?: number;
    preferredHeight?: number;
    minHeight?: number;
    maxHeight?: number;
    gap?: number;
    pad?: number;
  },
): ModelPickerLayout {
  const topInset = Math.max(0, options?.topInset ?? 58);
  const preferredHeight = options?.preferredHeight ?? 360;
  const maxCap = options?.maxHeight ?? 420;
  const gap = options?.gap ?? 8;
  const pad = options?.pad ?? 8;
  const above = Math.max(0, rect.top - topInset - gap - pad);
  const below = Math.max(0, viewportHeight - rect.bottom - gap - pad);
  const placement: "top" | "bottom" =
    below < preferredHeight && above > below ? "top" : "bottom";
  const available = Math.floor(placement === "top" ? above : below);
  const maxHeight = available <= 0 ? 48 : Math.min(maxCap, available);
  return { placement, maxHeight };
}

function measureTopInset(): number {
  if (typeof document === "undefined") return 58;
  const topbar = document.querySelector<HTMLElement>(".app-topbar");
  if (!topbar) return 58;
  const bottom = topbar.getBoundingClientRect().bottom;
  return Number.isFinite(bottom) && bottom > 0 ? Math.ceil(bottom) : 58;
}

type ModelPickerProps = {
  id: string;
  value: string;
  onChange: (value: string) => void;
  options: string[];
  disabled?: boolean;
  loading?: boolean;
  refreshDisabled?: boolean;
  onRefresh?: () => void;
  placeholder?: string;
  describedBy?: string;
  statusTone?: "loading" | "ok" | "warn" | "error";
  statusMessage?: string;
};

export default function ModelPicker({
  id,
  value,
  onChange,
  options,
  disabled = false,
  loading = false,
  refreshDisabled = false,
  onRefresh,
  placeholder,
  describedBy,
  statusTone,
  statusMessage,
}: ModelPickerProps) {
  const { t: tr } = useI18n();
  const listId = useId();
  const root = useRef<HTMLDivElement>(null);
  const input = useRef<HTMLInputElement>(null);
  const list = useRef<HTMLDivElement>(null);
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [active, setActive] = useState(-1);
  const [layout, setLayout] = useState<ModelPickerLayout>({
    placement: "bottom",
    maxHeight: 420,
  });
  const visible = useMemo(
    () => filterModels(options, query),
    [options, query],
  );
  const activeIndex = active >= 0 && active < visible.length ? active : -1;
  const current = value.trim();
  const hasClear = Boolean(value) && !disabled;

  const updateLayout = useCallback(() => {
    const rect = root.current?.getBoundingClientRect();
    if (!rect || typeof window === "undefined") return;
    const viewport =
      window.visualViewport?.height ?? window.innerHeight;
    setLayout(
      computeModelPickerLayout(rect, viewport, {
        topInset: measureTopInset(),
      }),
    );
  }, []);

  function close() {
    setOpen(false);
    setQuery("");
    setActive(-1);
  }

  function show(nextQuery = "") {
    const nextVisible = filterModels(options, nextQuery);
    updateLayout();
    setOpen(true);
    setQuery(nextQuery);
    setActive(
      nextQuery
        ? nextVisible.length
          ? 0
          : -1
        : options.findIndex((item) => item === current),
    );
  }

  function choose(next: string) {
    onChange(next);
    if (document.activeElement !== input.current) {
      input.current?.focus({ preventScroll: true });
    }
    close();
  }

  function toggleFromButton() {
    if (disabled) return;
    if (open) {
      close();
      return;
    }
    if (document.activeElement !== input.current) {
      input.current?.focus({ preventScroll: true });
    }
    show("");
  }

  useEffect(() => {
    if (!open) return;
    const onPointerDown = (event: PointerEvent) => {
      if (!root.current?.contains(event.target as Node)) close();
    };
    document.addEventListener("pointerdown", onPointerDown);
    return () => document.removeEventListener("pointerdown", onPointerDown);
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
    if (disabled) close();
  }, [disabled]);

  useEffect(() => {
    if (!open || activeIndex < 0) return;
    const option = list.current?.querySelector<HTMLElement>(
      `[data-index="${activeIndex}"]`,
    );
    option?.scrollIntoView({ block: "nearest" });
  }, [activeIndex, open]);

  const statusNode = !statusMessage ? null : statusTone === "error" && statusMessage.includes("\n") ? (
    <ErrorFold text={statusMessage} compact />
  ) : (
    <p
      className={"model-status" + (statusTone ? ` is-${statusTone}` : "")}
      role={statusTone === "error" ? "alert" : "status"}
      aria-live="polite"
    >
      {statusTone === "loading" ? (
        <Icon name="spinner" className="icon-spin" width="13" height="13" />
      ) : statusTone === "ok" ? (
        <Icon name="check" width="13" height="13" />
      ) : (
        <Icon name="info" width="13" height="13" />
      )}
      <span>{statusMessage}</span>
    </p>
  );

  return (
    <>
      <div
        className={"model-picker" + (hasClear ? " has-clear" : "")}
        ref={root}
        data-open={open ? "true" : "false"}
        onKeyDown={(event) => {
          if (event.defaultPrevented) return;
          if (event.key === "Escape" && open) {
            event.stopPropagation();
            close();
            input.current?.focus({ preventScroll: true });
          }
        }}
      >
        <input
          ref={input}
          id={id}
          value={value}
          placeholder={placeholder}
          maxLength={256}
          autoComplete="off"
          spellCheck={false}
          disabled={disabled}
          aria-describedby={describedBy}
          role="combobox"
          aria-expanded={open}
          aria-controls={listId}
          aria-autocomplete="list"
          aria-activedescendant={
            open && activeIndex >= 0 ? `${listId}-${activeIndex}` : undefined
          }
          onClick={() => {
            if (!disabled && !open) show("");
          }}
          onChange={(event) => {
            const next = event.target.value;
            onChange(next);
            const nextVisible = filterModels(options, next);
            if (!open) updateLayout();
            setOpen(true);
            setQuery(next);
            setActive(next ? (nextVisible.length ? 0 : -1) : -1);
          }}
          onKeyDown={(event) => {
            if (event.key === "ArrowDown") {
              event.preventDefault();
              if (!open) {
                show("");
              } else if (visible.length) {
                setActive(
                  activeIndex < 0 ? 0 : (activeIndex + 1) % visible.length,
                );
              }
            } else if (event.key === "ArrowUp") {
              event.preventDefault();
              if (!open) {
                show("");
              } else if (visible.length) {
                setActive(
                  activeIndex < 0
                    ? visible.length - 1
                    : (activeIndex - 1 + visible.length) % visible.length,
                );
              }
            } else if (event.key === "Enter") {
              if (!open) return;
              event.preventDefault();
              if (activeIndex >= 0) choose(visible[activeIndex]);
              else close();
            } else if (event.key === "Escape") {
              if (!open) return;
              event.preventDefault();
              close();
            } else if (event.key === "Tab") {
              if (open) close();
            }
          }}
        />
        <span className="model-picker-actions">
          {hasClear && (
            <>
              <button
                type="button"
                className="model-action model-clear is-round"
                title={tr("清空模型名称")}
                aria-label={tr("清空模型名称")}
                onMouseDown={(e) => e.preventDefault()}
                onClick={() => {
                  onChange("");
                  setQuery("");
                  setActive(-1);
                  if (open) updateLayout();
                  input.current?.focus({ preventScroll: true });
                }}
              >
                <Icon name="close" />
              </button>
              <span className="model-action-sep" aria-hidden="true" />
            </>
          )}
          <button
            type="button"
            className="model-action"
            title={tr("获取模型列表")}
            aria-label={tr("获取模型列表")}
            aria-busy={loading}
            disabled={disabled || refreshDisabled}
            onClick={() => onRefresh?.()}
          >
            <Icon
              name={loading ? "spinner" : "refresh"}
              className={loading ? "icon-spin" : ""}
            />
          </button>
          <span className="model-action-sep" aria-hidden="true" />
          <button
            type="button"
            className="model-action model-toggle"
            title={tr("模型列表")}
            aria-label={tr("模型列表")}
            aria-expanded={open}
            aria-controls={listId}
            disabled={disabled}
            onClick={toggleFromButton}
          >
            <Icon name="chevron" />
          </button>
        </span>
        {open && !disabled && (
          <div
            className="model-popover"
            data-placement={layout.placement}
            style={{ maxHeight: layout.maxHeight }}
          >
            <div className="model-popover-head">
              <strong>{tr("模型库")}</strong>
              <span className="model-popover-meta">
                {loading && options.length > 0 && (
                  <span className="model-popover-sync" role="status">
                    <Icon name="spinner" className="icon-spin" width="12" height="12" />
                    <span>{tr("正在刷新…")}</span>
                  </span>
                )}
                <span className="model-popover-count">
                  {query.trim()
                    ? `${visible.length} / ${options.length}`
                    : tr("{n} 个可测模型", { n: options.length })}
                </span>
              </span>
            </div>
            <div
              id={listId}
              ref={list}
              className="model-options"
              role="listbox"
              aria-label={tr("模型库")}
            >
              {loading && options.length === 0 ? (
                <div className="model-loading" role="status">
                  <Icon name="spinner" className="icon-spin" />
                  <span>{tr("正在获取模型…")}</span>
                </div>
              ) : visible.length ? (
                visible.map((item, index) => {
                  const selected = item === current;
                  return (
                    <button
                      key={item}
                      id={`${listId}-${index}`}
                      type="button"
                      role="option"
                      aria-selected={selected}
                      data-index={index}
                      data-active={index === activeIndex ? "true" : undefined}
                      className="model-option"
                      title={item}
                      onMouseDown={(event) => event.preventDefault()}
                      onMouseMove={() => setActive(index)}
                      onClick={() => choose(item)}
                    >
                      <span className="model-family">
                        {modelFamilyLabel(item)}
                      </span>
                      <span className="model-option-name">{item}</span>
                      {selected && <Icon name="check" />}
                    </button>
                  );
                })
              ) : (
                <div className="model-empty">
                  <Icon name="search" />
                  <strong>{tr("没有匹配的模型")}</strong>
                  <span>{tr("可直接输入自定义名称")}</span>
                </div>
              )}
            </div>
            <div className="model-popover-foot">
              {tr("↑↓ 选择 · Enter 确认 · Esc 关闭")}
            </div>
          </div>
        )}
      </div>
      {statusNode}
    </>
  );
}
