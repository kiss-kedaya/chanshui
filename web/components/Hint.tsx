"use client";
import { ReactNode, useId, useRef, useState } from "react";
import { createPortal } from "react-dom";
import Icon from "@/components/Icon";
import { useI18n } from "@/lib/i18n";
export default function Hint({
  text,
  label = "功能说明",
  children,
}: {
  text: string;
  label?: string;
  children?: ReactNode;
}) {
  const { t: tr } = useI18n();
  const id = useId(),
    ref = useRef<HTMLButtonElement>(null);
  const [open, setOpen] = useState(false),
    [position, setPosition] = useState({ left: 8, top: 8 });
  function show() {
    const rect = ref.current?.getBoundingClientRect();
    if (rect)
      setPosition({
        left: Math.max(8, Math.min(window.innerWidth - 296, rect.left - 120)),
        top: Math.min(window.innerHeight - 140, rect.bottom + 8),
      });
    setOpen(true);
  }
  return (
    <span
      className="hint"
      onMouseEnter={show}
      onMouseLeave={() => setOpen(false)}
    >
      <button
        ref={ref}
        type="button"
        className="hint-button"
        aria-label={tr(label)}
        aria-describedby={open ? id : undefined}
        onFocus={show}
        onBlur={() => setOpen(false)}
        onClick={() => (open ? setOpen(false) : show())}
        onKeyDown={(e) => {
          if (e.key === "Escape") setOpen(false);
        }}
      >
        {children || <Icon name="info" width="16" height="16" />}
      </button>
      {open &&
        createPortal(
          <span
            id={id}
            role="tooltip"
            className="hint-popover"
            style={position}
          >
            {tr(text)}
          </span>,
          document.body,
        )}
    </span>
  );
}
