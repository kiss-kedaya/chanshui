export const ACTIVE_POLL_MS = 1500;
export const MAX_POLL_MS = 5000;
const MAX_TRANSIENT_RETRIES = 3;

export function nextPollDelay(unchanged: number): number {
  return Math.min(MAX_POLL_MS, ACTIVE_POLL_MS + Math.max(0, unchanged) * 1000);
}

function isAbortError(error: unknown): boolean {
  return Boolean(error && typeof error === "object" && (error as { name?: string }).name === "AbortError");
}

function isTransientPollError(error: unknown): boolean {
  if (!error || typeof error !== "object") return false;
  const err = error as { name?: string; message?: string; transient?: boolean };
  if (err.transient) return true;
  const msg = (err.message || "").toLowerCase();
  return (
    err.name === "TypeError" ||
    msg.includes("networkerror") ||
    msg.includes("failed to fetch") ||
    msg.includes("load failed") ||
    msg.includes("网络连接中断或超时")
  );
}

export function watchResource<T>({
  load,
  onValue,
  isActive,
  onError,
}: {
  load: (signal: AbortSignal) => Promise<T | null>;
  onValue: (value: T) => void;
  isActive: (value: T) => boolean;
  onError: (error: Error) => void;
}): () => void {
  let stopped = false;
  let timer: ReturnType<typeof setTimeout> | undefined;
  let request: AbortController | null = null;
  let lastValue: string | undefined;
  let unchanged = 0;
  let transientFailures = 0;
  let wakePending = false;
  const available = () =>
    document.visibilityState !== "hidden" && navigator.onLine !== false;
  const clearTimer = () => {
    if (timer !== undefined) clearTimeout(timer);
    timer = undefined;
  };
  const onPageHide = () => {
    stop();
  };
  const stop = () => {
    if (stopped) return;
    stopped = true;
    clearTimer();
    request?.abort();
    document.removeEventListener("visibilitychange", wake);
    window.removeEventListener("online", wake);
    window.removeEventListener("offline", wake);
    window.removeEventListener("pagehide", onPageHide);
  };
  async function poll() {
    if (stopped || request || !available()) return;
    const controller = new AbortController();
    request = controller;
    try {
      const value = await load(controller.signal);
      if (stopped || controller.signal.aborted) return;
      transientFailures = 0;
      const serialized = value === null ? lastValue : JSON.stringify(value);
      const changed = value !== null && serialized !== lastValue;
      unchanged = changed ? 0 : unchanged + 1;
      if (changed) {
        lastValue = serialized;
        onValue(value);
      }
      if (value !== null && !isActive(value)) {
        stop();
        return;
      }
      timer = setTimeout(poll, nextPollDelay(unchanged));
    } catch (error) {
      if (stopped || controller.signal.aborted || isAbortError(error)) return;
      if (isTransientPollError(error)) {
        transientFailures++;
        if (lastValue !== undefined || transientFailures <= MAX_TRANSIENT_RETRIES) {
          timer = setTimeout(poll, Math.min(MAX_POLL_MS, 1200 * transientFailures));
          return;
        }
      }
      stop();
      onError(error instanceof Error ? error : new Error(String(error)));
    } finally {
      request = null;
      if (wakePending && !stopped) {
        wakePending = false;
        clearTimer();
        void poll();
      }
    }
  }
  function wake() {
    clearTimer();
    if (!available()) {
      request?.abort();
      return;
    }
    if (request) wakePending = true;
    else void poll();
  }
  document.addEventListener("visibilitychange", wake);
  window.addEventListener("online", wake);
  window.addEventListener("offline", wake);
  window.addEventListener("pagehide", onPageHide);
  void poll();
  return stop;
}
