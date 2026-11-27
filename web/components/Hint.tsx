"use client";
import { ReactNode, useCallback, useEffect, useId, useLayoutEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import Icon from "@/components/Icon";
import { useI18n } from "@/lib/i18n";

export type HintRect = { left: number; right: number; top: number; bottom: number; width: number };
export type HintViewport = { width: number; height: number };
export type HintSize = { width: number; height: number };
export type HintPosition = { left: number; top: number; placement: "top" | "bottom" };
export type HintVisibility = { open: boolean; pinned: boolean };
export type HintEvent = "hover-enter" | "hover-leave" | "focus" | "blur" | "click" | "dismiss";

const useIsomorphicLayoutEffect = typeof window !== "undefined" ? useLayoutEffect : useEffect;

export function nextHintVisibility(
  state: HintVisibility,
  event: HintEvent,
  isTouch = false,
): HintVisibility {
  switch (event) {
    case "hover-enter":
    case "focus":
      if (isTouch) return state;
      return { open: true, pinned: state.open && state.pinned };
    case "hover-leave":
      if (isTouch || state.pinned) return state;
      return { open: false, pinned: false };
    case "click":
      if (isTouch) {
        return state.open ? { open: false, pinned: false } : { open: true, pinned: true };
      }
      if (state.open && state.pinned) return { open: false, pinned: false };
      return { open: true, pinned: true };
    case "blur":
      return state.pinned ? state : { open: false, pinned: false };
    case "dismiss":
      return { open: false, pinned: false };
  }
}

export function computeHintPosition(
  rect: HintRect,
  viewport: HintViewport,
  tipSize: HintSize = { width: 280, height: 112 },
): HintPosition {
  const pad = 8;
  const gap = 8;
  const tipWidth = Math.min(tipSize.width || 280, Math.max(120, viewport.width - pad * 2));
  const tipHeight = Math.max(40, tipSize.height || 112);
  const centerLeft = rect.left + rect.width / 2 - tipWidth / 2;
  const left = Math.round(Math.max(pad, Math.min(viewport.width - tipWidth - pad, centerLeft)));
  const spaceBelow = viewport.height - rect.bottom - pad;
  const spaceAbove = rect.top - pad;
  const placement: "top" | "bottom" =
    spaceBelow < tipHeight + gap && spaceAbove > spaceBelow ? "top" : "bottom";
  const rawTop = placement === "top" ? rect.top - tipHeight - gap : rect.bottom + gap;
  const top = Math.round(Math.max(pad, Math.min(viewport.height - tipHeight - pad, rawTop)));
  return { left, top, placement };
}

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
    rootRef = useRef<HTMLSpanElement>(null),
    ref = useRef<HTMLButtonElement>(null),
    popoverRef = useRef<HTMLSpanElement>(null),
    touchPointer = useRef(false);
  const [visibility, setVisibility] = useState<HintVisibility>({ open: false, pinned: false }),
    [position, setPosition] = useState<HintPosition>({ left: 8, top: 8, placement: "bottom" });
  const open = visibility.open;

  const updatePosition = useCallback(() => {
    const rect = ref.current?.getBoundingClientRect();
    if (!rect || typeof window === "undefined") return;
    const tipEl = popoverRef.current;
    const tipSize: HintSize = tipEl
      ? { width: tipEl.offsetWidth || 280, height: tipEl.offsetHeight || 112 }
      : { width: 280, height: 112 };
    setPosition(
      computeHintPosition(
        rect,
        { width: window.innerWidth, height: window.innerHeight },
        tipSize,
      ),
    );
  }, []);

  const trigger = useCallback((event: HintEvent, isTouch = false) => {
    setVisibility((prev) => nextHintVisibility(prev, event, isTouch));
  }, []);

  useIsomorphicLayoutEffect(() => {
    if (!open) return;
    updatePosition();
  }, [open, text, updatePosition]);

  useEffect(() => {
    if (!open) return;
    const onPointerDown = (e: PointerEvent) => {
      const target = e.target as Node | null;
      if (
        target &&
        !rootRef.current?.contains(target) &&
        !popoverRef.current?.contains(target)
      ) {
        trigger("dismiss");
      }
    };
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") trigger("dismiss");
    };
    const dismiss = () => trigger("dismiss");
    document.addEventListener("pointerdown", onPointerDown);
    document.addEventListener("keydown", onKeyDown);
    window.addEventListener("scroll", dismiss, true);
    window.addEventListener("resize", dismiss);
    return () => {
      document.removeEventListener("pointerdown", onPointerDown);
      document.removeEventListener("keydown", onKeyDown);
      window.removeEventListener("scroll", dismiss, true);
      window.removeEventListener("resize", dismiss);
    };
  }, [open, trigger]);

  return (
    <span
      ref={rootRef}
      className="hint"
      onPointerDown={(e) => {
        touchPointer.current = e.pointerType === "touch";
      }}
      onPointerMove={(e) => {
        if (e.pointerType === "mouse") touchPointer.current = false;
      }}
      onMouseEnter={() => {
        trigger("hover-enter", touchPointer.current);
      }}
      onMouseLeave={() => {
        trigger("hover-leave", touchPointer.current);
      }}
    >
      <button
        ref={ref}
        type="button"
        className="hint-button"
        aria-label={tr(label)}
        aria-describedby={open ? id : undefined}
        onFocus={() => {
          trigger("focus", touchPointer.current);
        }}
        onBlur={(e) => {
          const nextTarget = e.relatedTarget;
          if (
            typeof Node !== "undefined" &&
            nextTarget instanceof Node &&
            !rootRef.current?.contains(nextTarget) &&
            !popoverRef.current?.contains(nextTarget)
          ) {
            trigger("dismiss");
          } else {
            trigger("blur");
          }
        }}
        onClick={(e) => {
          e.preventDefault();
          e.stopPropagation();
          trigger("click", touchPointer.current);
        }}
        onKeyDown={(e) => {
          if (e.key === "Escape") trigger("dismiss");
        }}
      >
        {children || <Icon name="info" width="16" height="16" />}
      </button>
      {open &&
        typeof document !== "undefined" &&
        createPortal(
          <span
            ref={popoverRef}
            id={id}
            role="tooltip"
            className="hint-popover"
            data-placement={position.placement}
            style={{ left: position.left, top: position.top }}
          >
            {tr(text)}
          </span>,
          document.body,
        )}
    </span>
  );
}
