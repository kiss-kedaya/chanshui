"use client";
import { useEffect, useRef, useState } from "react";
export default function AnimatedNumber({
  value,
  decimals = 0,
}: {
  value: number;
  decimals?: number;
}) {
  const safe = Number.isFinite(value) ? value : 0;
  const [shown, setShown] = useState(safe);
  const previous = useRef(0);
  useEffect(() => {
    const reduce = window.matchMedia("(prefers-reduced-motion: reduce)");
    let frame = 0;
    const from = previous.current;
    previous.current = safe;
    const stop = () => {
      cancelAnimationFrame(frame);
      setShown(safe);
    };
    if (reduce.matches) {
      stop();
      return;
    }
    const start = performance.now();
    setShown(from);
    const draw = (now: number) => {
      const t = Math.min((now - start) / 550, 1);
      setShown(from + (safe - from) * (1 - Math.pow(1 - t, 3)));
      if (t < 1) frame = requestAnimationFrame(draw);
    };
    frame = requestAnimationFrame(draw);
    reduce.addEventListener("change", stop);
    return () => {
      cancelAnimationFrame(frame);
      reduce.removeEventListener("change", stop);
    };
  }, [safe]);
  const format = (n: number) =>
    n.toLocaleString("zh-CN", {
      minimumFractionDigits: decimals,
      maximumFractionDigits: decimals,
    });
  return (
    <span className="animated-number">
      <span aria-hidden="true">{format(shown)}</span>
      <span className="sr-only">{format(safe)}</span>
    </span>
  );
}
