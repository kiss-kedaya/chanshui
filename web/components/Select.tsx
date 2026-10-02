"use client";
import {
  Children,
  isValidElement,
  ReactNode,
  useEffect,
  useId,
  useRef,
  useState,
} from "react";
import Icon from "@/components/Icon";
type Option = { value: string; label: string };
export default function Select({
  id,
  value,
  onChange,
  children,
  disabled = false,
  loading = false,
}: {
  id?: string;
  value: string;
  onChange: (event: { target: { value: string } }) => void;
  children: ReactNode;
  disabled?: boolean;
  loading?: boolean;
}) {
  const generated = useId(),
    name = id || generated,
    root = useRef<HTMLDivElement>(null),
    trigger = useRef<HTMLButtonElement>(null);
  const [open, setOpen] = useState(false),
    [query, setQuery] = useState("");
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
  const visible = opts.filter((o) =>
    o.label.toLowerCase().includes(query.toLowerCase()),
  );
  const current = opts.find((o) => o.value === value);
  useEffect(() => {
    const close = (e: MouseEvent) => {
      if (!root.current?.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener("mousedown", close);
    return () => document.removeEventListener("mousedown", close);
  }, []);
  function choose(o: Option) {
    onChange({ target: { value: o.value } });
    setOpen(false);
    setQuery("");
    trigger.current?.focus();
  }
  return (
    <div
      className="custom-select"
      ref={root}
      onKeyDown={(e) => {
        if (e.key === "Escape") {
          e.stopPropagation();
          setOpen(false);
          trigger.current?.focus();
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
          setOpen(!open);
          setQuery("");
        }}
        onKeyDown={(e) => {
          if (e.key === "Escape") setOpen(false);
          if (e.key === "ArrowDown") {
            e.preventDefault();
            setOpen(true);
            requestAnimationFrame(() =>
              root.current
                ?.querySelector<HTMLButtonElement>('[role="option"]')
                ?.focus(),
            );
          }
        }}
      >
        <span className={value ? "" : "select-placeholder"}>
          {current?.label || "请选择"}
        </span>
        <span className="select-chevron" aria-hidden="true">
          {loading ? (
            <Icon name="spinner" className="icon-spin" />
          ) : (
            <Icon name="chevron" width="16" height="16" />
          )}
        </span>
      </button>
      {open && (
        <div className="select-popover">
          {!loading && opts.length > 6 && (
            <input
              autoFocus
              aria-label="搜索模型"
              placeholder="输入名称筛选模型…"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
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
                <span>正在获取模型…</span>
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
                      setOpen(false);
                      trigger.current?.focus();
                    }
                    if (e.key === "ArrowDown" || e.key === "ArrowUp") {
                      e.preventDefault();
                      const buttons = Array.from(
                          root.current?.querySelectorAll<HTMLButtonElement>(
                            '[role="option"]',
                          ) || [],
                        ),
                        i = buttons.indexOf(e.currentTarget);
                      buttons[
                        (i +
                          (e.key === "ArrowDown" ? 1 : -1) +
                          buttons.length) %
                          buttons.length
                      ]?.focus();
                    }
                  }}
                >
                  <span>{o.label}</span>
                  {o.value === value && <b aria-hidden="true">✓</b>}
                </button>
              ))
            ) : (
              <p className="select-empty">没有匹配的模型</p>
            )}
          </div>
        </div>
      )}
    </div>
  );
}
