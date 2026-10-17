"use client";
import { useEffect, useId, useMemo, useRef, useState } from "react";
import { useI18n } from "@/lib/i18n";
import { filterModels, modelFamilyLabel } from "@/lib/model-picker";
import Icon from "@/components/Icon";

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
}: ModelPickerProps) {
  const { t: tr } = useI18n();
  const listId = useId();
  const root = useRef<HTMLDivElement>(null);
  const input = useRef<HTMLInputElement>(null);
  const list = useRef<HTMLDivElement>(null);
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [active, setActive] = useState(-1);
  const [placement, setPlacement] = useState<"top" | "bottom">("bottom");
  const visible = useMemo(
    () => filterModels(options, query),
    [options, query],
  );
  const activeIndex = active >= 0 && active < visible.length ? active : -1;
  const current = value.trim();

  function close() {
    setOpen(false);
    setQuery("");
    setActive(-1);
  }

  function show(nextQuery = "") {
    const nextVisible = filterModels(options, nextQuery);
    setOpen(true);
    setQuery(nextQuery);
    setPlacement(placementFor());
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

  function placementFor(): "top" | "bottom" {
    const rect = root.current?.getBoundingClientRect();
    if (!rect) return "bottom";
    const viewport =
      window.visualViewport?.height ?? window.innerHeight;
    const below = viewport - rect.bottom;
    return below < 360 && rect.top > below ? "top" : "bottom";
  }

  useEffect(() => {
    if (!open) return;
    const onDown = (event: MouseEvent) => {
      if (!root.current?.contains(event.target as Node)) close();
    };
    document.addEventListener("mousedown", onDown);
    return () => document.removeEventListener("mousedown", onDown);
  }, [open]);

  useEffect(() => {
    if (!open) return;
    const place = () => setPlacement(placementFor());
    window.addEventListener("resize", place);
    window.visualViewport?.addEventListener("resize", place);
    return () => {
      window.removeEventListener("resize", place);
      window.visualViewport?.removeEventListener("resize", place);
    };
  }, [open]);

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

  return (
    <div
      className="model-picker"
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
        <div className="model-popover" data-placement={placement}>
          <div className="model-popover-head">
            <strong>{tr("模型库")}</strong>
            <span className="model-popover-count">
              {query.trim()
                ? `${visible.length} / ${options.length}`
                : tr("{n} 个可测模型", { n: options.length })}
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
  );
}
