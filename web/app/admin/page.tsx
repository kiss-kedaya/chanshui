"use client";

import Icon from "@/components/Icon";
import Select from "@/components/Select";
import {
  FormEvent,
  KeyboardEvent as ReactKeyboardEvent,
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
} from "react";
import { formatLocaleDate, useI18n } from "@/lib/i18n";
import { loadKey, saveKey } from "@/lib/keyvault";
import { getModels } from "@/lib/api";
import { filterModels, modelFamilyLabel } from "@/lib/model-picker";

type AuthState =
  | "checking"
  | "disabled"
  | "unauthenticated"
  | "load_error"
  | "authenticated";

type Item = {
  id: string;
  created_at: number;
  kind: string;
  contact: string;
  message: string;
};

type Reference = {
  id: string;
  display_name: string;
  family: string;
  family_name: string;
  source: string;
  captured_at: string;
  samples: number;
  aliases?: string[];
};

type CaptureGroup = {
  group: number;
  top_model: string;
  top_display: string;
  probability: number;
  pass: boolean;
};

type Capture = {
  id: string;
  status: string;
  started_at: string;
  updated_at: string;
  train_requested: number;
  holdout_requested: number;
  total: number;
  completed: number;
  train_usable: number;
  holdout_usable: number;
  error?: string;
  groups?: CaptureGroup[];
  reference?: Reference;
};

type CaptureForm = {
  base_url: string;
  key: string;
  model: string;
  id: string;
  display_name: string;
  aliases: string;
  protocol: "auto" | "chat" | "anthropic" | "openai";
  train: number;
  holdout: number;
  concurrency: number;
  timeout: number;
};

const emptyCapture: CaptureForm = {
  base_url: "",
  key: "",
  model: "",
  id: "",
  display_name: "",
  aliases: "",
  protocol: "auto",
  train: 36,
  holdout: 12,
  concurrency: 3,
  timeout: 180,
};

const captureLabels: Record<string, string> = {
  running: "采集中",
  validating: "校验归因",
  done: "已完成",
  failed: "失败",
  cancelled: "已取消",
};

const RING_RADIUS = 17;
const RING_CIRCUMFERENCE = 2 * Math.PI * RING_RADIUS;

type CaptureModelComboboxProps = {
  id: string;
  value: string;
  options: string[];
  loading: boolean;
  placeholder?: string;
  onChange: (value: string) => void;
};

function CaptureModelCombobox({
  id,
  value,
  options,
  loading,
  placeholder,
  onChange,
}: CaptureModelComboboxProps) {
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

  function placementFor(): "top" | "bottom" {
    const rect = root.current?.getBoundingClientRect();
    if (!rect) return "bottom";
    const viewport = window.visualViewport?.height ?? window.innerHeight;
    const below = viewport - rect.bottom;
    return below < 320 && rect.top - 58 > below ? "top" : "bottom";
  }

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
    if (!open || activeIndex < 0) return;
    const option = list.current?.querySelector<HTMLElement>(
      `[data-index="${activeIndex}"]`,
    );
    option?.scrollIntoView({ block: "nearest" });
  }, [activeIndex, open]);

  const countLabel = loading && options.length === 0
    ? tr("正在获取模型…")
    : options.length === 0
      ? tr("尚未拉取上游模型")
      : query.trim()
        ? `${visible.length} / ${options.length}`
        : tr("{n} 个模型", { n: options.length });

  return (
    <div
      className="admin-model-combobox"
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
        required
        role="combobox"
        aria-expanded={open}
        aria-controls={listId}
        aria-autocomplete="list"
        aria-activedescendant={
          open && activeIndex >= 0 ? `${listId}-${activeIndex}` : undefined
        }
        onClick={() => {
          if (!open) show("");
        }}
        onChange={(event) => {
          const next = event.target.value;
          onChange(next);
          const nextVisible = filterModels(options, next);
          setOpen(true);
          setQuery(next);
          setPlacement(placementFor());
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
      <span className="admin-model-actions">
        {value.length > 0 && (
          <>
            <button
              type="button"
              className="model-action"
              title={tr("清空模型名")}
              aria-label={tr("清空模型名")}
              onMouseDown={(event) => event.preventDefault()}
              onClick={() => {
                onChange("");
                setQuery("");
                setActive(-1);
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
          className="model-action model-toggle"
          title={tr("模型列表")}
          aria-label={tr("模型列表")}
          aria-expanded={open}
          aria-controls={listId}
          onClick={toggleFromButton}
        >
          <Icon name="chevron" />
        </button>
      </span>
      {open && (
        <div className="model-popover" data-placement={placement}>
          <div className="model-popover-head">
            <strong>{tr("上游模型")}</strong>
            <span className="model-popover-count">{countLabel}</span>
          </div>
          <div
            id={listId}
            ref={list}
            className="model-options"
            role="listbox"
            aria-label={tr("上游模型")}
          >
            {loading && options.length === 0 ? (
              <div className="model-loading" role="status">
                <Icon name="spinner" className="icon-spin" />
                <span>{tr("正在获取模型…")}</span>
              </div>
            ) : visible.length > 0 ? (
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
            ) : options.length === 0 ? (
              <div className="model-empty">
                <Icon name="search" />
                <strong>{tr("尚未拉取上游模型")}</strong>
                <span>{tr("点击「拉取模型」获取上游列表，或直接输入请求名")}</span>
              </div>
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

export default function AdminPage() {
  const { t: tr, lang } = useI18n();
  const kinds: Record<string, string> = {
    bug: tr("问题反馈"),
    method: tr("检测方法"),
    suggestion: tr("改进建议"),
  };
  const [authState, setAuthState] = useState<AuthState>("checking");
  const authStateRef = useRef<AuthState>("checking");
  const [token, setToken] = useState("");
  const [show, setShow] = useState(false);
  const [items, setItems] = useState<Item[] | null>(null);
  const [error, setError] = useState("");
  const [refError, setRefError] = useState("");
  const [busy, setBusy] = useState(false);
  const [refreshing, setRefreshing] = useState(false);
  const [logoutBusy, setLogoutBusy] = useState(false);
  const [refs, setRefs] = useState<Reference[]>([]);
  const [capture, setCapture] = useState<Capture | null>(null);
  const [form, setForm] = useState<CaptureForm>(emptyCapture);
  const [idEdited, setIdEdited] = useState(false);
  const [captureBusy, setCaptureBusy] = useState(false);
  const [cancelBusy, setCancelBusy] = useState(false);
  const [captureError, setCaptureError] = useState("");
  const [fillBusy, setFillBusy] = useState(false);
  const [modelBusy, setModelBusy] = useState(false);
  const [captureModels, setCaptureModels] = useState<string[]>([]);
  const [showCaptureKey, setShowCaptureKey] = useState(false);
  const [confirmDeleteId, setConfirmDeleteId] = useState<string | null>(null);
  const [deletingId, setDeletingId] = useState<string | null>(null);
  const [tab, setTab] = useState<"feedback" | "fingerprint">("feedback");

  function setAuth(next: AuthState) {
    authStateRef.current = next;
    setAuthState(next);
  }

  function clearSessionData() {
    setItems(null);
    setRefs([]);
    setCapture(null);
    setConfirmDeleteId(null);
    setDeletingId(null);
    setIdEdited(false);
    setCaptureModels([]);
    setCaptureError("");
    setRefError("");
  }

  function applyModelChange(nextModel: string) {
    setForm((f) => ({
      ...f,
      model: nextModel,
      id: idEdited ? f.id : nextModel.trim().replace(/\s+/g, "-"),
    }));
  }

  async function load(signal?: AbortSignal) {
    const [feedback, fingerprints] = await Promise.all([
      fetch("/api/admin/feedback", {
        cache: "no-store",
        credentials: "same-origin",
        signal,
      }),
      fetch("/api/admin/fingerprints", {
        cache: "no-store",
        credentials: "same-origin",
        signal,
      }),
    ]);

    if (feedback.status === 404 || fingerprints.status === 404) {
      clearSessionData();
      setError("");
      setAuth("disabled");
      return;
    }

    if (feedback.status === 401) {
      clearSessionData();
      setAuth("unauthenticated");
      return;
    }

    const data = await feedback.json().catch(() => ({}));
    if (!feedback.ok) {
      const msg = (typeof data?.error === "string" && data.error) || tr("读取失败");
      if (authStateRef.current === "authenticated") {
        setError(msg);
      } else {
        clearSessionData();
        setError(msg);
        setAuth("load_error");
      }
      return;
    }

    setItems(Array.isArray(data.items) ? data.items : []);
    setError("");

    if (fingerprints.status === 401) {
      clearSessionData();
      setError(tr("管理员会话已失效，请重新登录。"));
      setAuth("unauthenticated");
      return;
    }

    if (!fingerprints.ok) {
      const fpErr = await fingerprints.json().catch(() => ({}));
      setRefError(
        (typeof fpErr?.error === "string" && fpErr.error) ||
          tr("自定义指纹读取失败"),
      );
      setAuth("authenticated");
      return;
    }

    const fp = await fingerprints.json().catch(() => ({}));
    setRefError("");
    setRefs(Array.isArray(fp.items) ? fp.items : []);
    setCapture(fp.capture ?? null);
    setAuth("authenticated");
  }

  useEffect(() => {
    const controller = new AbortController();
    setAuth("checking");
    load(controller.signal).catch((err: unknown) => {
      if ((err as { name?: string })?.name === "AbortError") return;
      clearSessionData();
      setError(tr("网络连接中断或超时，请稍后重试"));
      setAuth("load_error");
    });
    return () => controller.abort();
  }, []);

  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      const res = await fetch("/api/admin/login", {
        method: "POST",
        credentials: "same-origin",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ token }),
      });
      if (res.status === 404) {
        clearSessionData();
        setToken("");
        setAuth("disabled");
        return;
      }
      const data = await res.json().catch(() => ({}));
      if (res.status === 429) {
        setError(
          (typeof data?.error === "string" && data.error) ||
            tr("请求过于频繁，请十分钟后再试。"),
        );
        return;
      }
      if (!res.ok) {
        setError(
          (typeof data?.error === "string" && data.error) || tr("登录失败"),
        );
        return;
      }
      setToken("");
      await load();
    } catch {
      setError(tr("网络连接中断或超时，请稍后重试"));
    } finally {
      setBusy(false);
    }
  }

  async function logout() {
    if (logoutBusy) return;
    setLogoutBusy(true);
    setError("");
    try {
      const res = await fetch("/api/admin/logout", {
        method: "POST",
        credentials: "same-origin",
      });
      if (res.status === 404) {
        clearSessionData();
        setTab("feedback");
        setAuth("disabled");
        return;
      }
      if (!res.ok && res.status !== 401) {
        const data = await res.json().catch(() => ({}));
        setError(
          (typeof data?.error === "string" && data.error) || tr("退出失败"),
        );
        return;
      }
      clearSessionData();
      setError("");
      setTab("feedback");
      setAuth("unauthenticated");
    } catch {
      setError(tr("网络连接中断或超时，请稍后重试"));
    } finally {
      setLogoutBusy(false);
    }
  }

  async function refresh() {
    setRefreshing(true);
    setError("");
    try {
      await load();
    } catch {
      const msg = tr("网络连接中断或超时，请稍后重试");
      if (authStateRef.current === "authenticated") {
        setError(msg);
      } else {
        setError(msg);
        setAuth("load_error");
      }
    } finally {
      setRefreshing(false);
    }
  }

  async function fillSaved() {
    setFillBusy(true);
    setCaptureError("");
    try {
      let saved: { base_url?: string; model?: string; protocol?: string } = {};
      try {
        saved = JSON.parse(localStorage.getItem("chanshui-settings") || "{}");
      } catch {}
      const key = await loadKey();
      if (!saved.base_url && !key) {
        setCaptureError(tr("本机没有保存过的网关或 Key，请手动填写。"));
        return;
      }
      setForm((f) => {
        const nextModel = f.model || saved.model || "";
        return {
          ...f,
          base_url: f.base_url || saved.base_url || "",
          key: f.key || key || "",
          model: nextModel,
          id: idEdited
            ? f.id
            : f.id || nextModel.trim().replace(/\s+/g, "-"),
          protocol:
            f.protocol === "auto" &&
            ["auto", "chat", "anthropic", "openai"].includes(
              saved.protocol || "",
            )
              ? (saved.protocol as CaptureForm["protocol"])
              : f.protocol,
        };
      });
    } catch (err) {
      setCaptureError((err as Error).message);
    } finally {
      setFillBusy(false);
    }
  }

  async function listCaptureModels() {
    if (!form.base_url.trim() || !form.key.trim()) {
      setCaptureError(tr("先填写接口地址和 API Key。"));
      return;
    }
    setModelBusy(true);
    setCaptureError("");
    try {
      const data = await getModels({
        base_url: form.base_url.trim(),
        key: form.key.trim(),
        protocol: form.protocol,
      });
      setCaptureModels(data.models);
      if (!form.model.trim() && data.models.length) {
        applyModelChange(data.models[0]);
      }
      if (!data.models.length) {
        setCaptureError(tr("上游没有返回模型，仍可手动填写请求名。"));
      }
    } catch (err) {
      setCaptureError((err as Error).message);
    } finally {
      setModelBusy(false);
    }
  }

  async function startCapture(e: FormEvent) {
    e.preventDefault();
    if (!form.model.trim()) {
      setCaptureError(tr("请填写请求模型名。"));
      return;
    }
    setCaptureBusy(true);
    setCaptureError("");
    try {
      await saveKey(form.key);
      try {
        localStorage.setItem(
          "chanshui-settings",
          JSON.stringify({
            base_url: form.base_url.trim(),
            model: form.model,
            protocol: form.protocol,
          }),
        );
      } catch {}
      const res = await fetch("/api/admin/fingerprints/capture", {
        method: "POST",
        credentials: "same-origin",
        headers: { "content-type": "application/json" },
        body: JSON.stringify(form),
      });
      if (res.status === 404) {
        clearSessionData();
        setAuth("disabled");
        return;
      }
      if (res.status === 401) {
        clearSessionData();
        setError(tr("管理员会话已失效，请重新登录。"));
        setAuth("unauthenticated");
        return;
      }
      const data = await res.json().catch(() => ({}));
      if (res.status === 409 && data?.capture) {
        setCapture(data.capture);
        setCaptureError(
          (typeof data?.error === "string" && data.error) || tr("采集启动失败"),
        );
        return;
      }
      if (!res.ok) {
        setCaptureError(
          (typeof data?.error === "string" && data.error) || tr("采集启动失败"),
        );
        return;
      }
      setCapture(data);
      setForm((f) => ({ ...f, key: "" }));
      setIdEdited(false);
      setTab("fingerprint");
    } catch {
      setCaptureError(tr("网络连接中断或超时，请稍后重试"));
    } finally {
      setCaptureBusy(false);
    }
  }

  async function cancelCapture() {
    if (!capture || cancelBusy) return;
    setCancelBusy(true);
    setCaptureError("");
    try {
      const res = await fetch(
        `/api/admin/fingerprints/capture/${encodeURIComponent(capture.id)}/cancel`,
        {
          method: "POST",
          credentials: "same-origin",
        },
      );
      if (res.status === 404) {
        const msg = tr("采集任务不存在或已过期");
        setCaptureError(msg);
        setCapture((prev) =>
          prev && (prev.status === "running" || prev.status === "validating")
            ? { ...prev, status: "failed", error: msg }
            : prev,
        );
        return;
      }
      if (res.status === 401) {
        clearSessionData();
        setError(tr("管理员会话已失效，请重新登录。"));
        setAuth("unauthenticated");
        return;
      }
      const data = await res.json().catch(() => ({}));
      if (!res.ok) {
        setCaptureError(
          (typeof data?.error === "string" && data.error) ||
            tr("取消采集失败"),
        );
        return;
      }
      setCapture(data);
    } catch {
      setCaptureError(tr("网络连接中断或超时，请稍后重试"));
    } finally {
      setCancelBusy(false);
    }
  }

  async function removeReference(id: string) {
    if (deletingId) return;
    setDeletingId(id);
    setRefError("");
    try {
      const res = await fetch(
        `/api/admin/fingerprints/${encodeURIComponent(id)}`,
        {
          method: "DELETE",
          credentials: "same-origin",
        },
      );
      if (res.status === 401) {
        clearSessionData();
        setError(tr("管理员会话已失效，请重新登录。"));
        setAuth("unauthenticated");
        return;
      }
      const data = await res.json().catch(() => ({}));
      if (!res.ok) {
        setRefError(
          (typeof data?.error === "string" && data.error) || tr("删除失败"),
        );
        return;
      }
      setConfirmDeleteId(null);
      await refresh();
    } catch {
      setRefError(tr("网络连接中断或超时，请稍后重试"));
    } finally {
      setDeletingId(null);
    }
  }

  useEffect(() => {
    if (!capture || !["running", "validating"].includes(capture.status)) return;
    let alive = true;
    let timer: number | null = null;
    let controller: AbortController | null = null;
    let transientErrors = 0;

    const poll = async () => {
      if (!alive) return;
      controller = new AbortController();
      try {
        const res = await fetch(
          `/api/admin/fingerprints/capture/${encodeURIComponent(capture.id)}`,
          {
            cache: "no-store",
            credentials: "same-origin",
            signal: controller.signal,
          },
        );
        if (!alive) return;
        if (res.status === 401) {
          clearSessionData();
          setError(tr("管理员会话已失效，请重新登录。"));
          setAuth("unauthenticated");
          return;
        }
        const data = await res.json().catch(() => null);
        if (!alive) return;
        if (res.status === 404) {
          const msg =
            (typeof data?.error === "string" && data.error) ||
            tr("采集任务不存在或已过期");
          setCaptureError(msg);
          setCapture((prev) =>
            prev && (prev.status === "running" || prev.status === "validating")
              ? { ...prev, status: "failed", error: msg }
              : prev,
          );
          return;
        }
        if (!res.ok || !data) {
          transientErrors += 1;
          if (transientErrors >= 3) {
            const msg =
              (typeof data?.error === "string" && data.error) || tr("读取失败");
            setCaptureError(msg);
            setCapture((prev) =>
              prev && (prev.status === "running" || prev.status === "validating")
                ? { ...prev, status: "failed", error: msg }
                : prev,
            );
            return;
          }
          timer = window.setTimeout(poll, 1800);
          return;
        }
        transientErrors = 0;
        setCapture(data);
        if (["running", "validating"].includes(data.status)) {
          timer = window.setTimeout(poll, 1200);
        } else {
          await load().catch(() => {});
        }
      } catch (err: unknown) {
        if (!alive || (err as { name?: string })?.name === "AbortError") return;
        transientErrors += 1;
        if (transientErrors >= 3) {
          const msg = tr("网络连接中断或超时，请稍后重试");
          setCaptureError(msg);
          setCapture((prev) =>
            prev && (prev.status === "running" || prev.status === "validating")
              ? { ...prev, status: "failed", error: msg }
              : prev,
          );
          return;
        }
        timer = window.setTimeout(poll, 1800);
      }
    };

    timer = window.setTimeout(poll, 1200);
    return () => {
      alive = false;
      if (timer !== null) window.clearTimeout(timer);
      controller?.abort();
    };
  }, [capture?.id, capture?.status]);

  function download() {
    if (!items || items.length === 0) return;
    const blob = new Blob([JSON.stringify(items, null, 2)], {
      type: "application/json",
    });
    const url = URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.href = url;
    link.download = `chanshui-feedback-${new Date().toISOString().slice(0, 10)}.json`;
    link.click();
    URL.revokeObjectURL(url);
  }

  function handleTabKeyDown(event: ReactKeyboardEvent<HTMLDivElement>) {
    const order: Array<"feedback" | "fingerprint"> = ["feedback", "fingerprint"];
    const idx = order.indexOf(tab);
    let nextTab: "feedback" | "fingerprint" | null = null;
    if (event.key === "ArrowRight") nextTab = order[(idx + 1) % order.length];
    else if (event.key === "ArrowLeft")
      nextTab = order[(idx - 1 + order.length) % order.length];
    else if (event.key === "Home") nextTab = order[0];
    else if (event.key === "End") nextTab = order[order.length - 1];
    if (!nextTab) return;
    event.preventDefault();
    setTab(nextTab);
    const target = event.currentTarget.querySelector<HTMLButtonElement>(
      `#admin-tab-${nextTab}`,
    );
    target?.focus();
  }

  const percent = capture?.total
    ? Math.max(
        0,
        Math.min(100, Math.round((capture.completed / capture.total) * 100)),
      )
    : 0;
  const ringOffset = RING_CIRCUMFERENCE * (1 - percent / 100);
  const captureActive =
    !!capture && ["running", "validating"].includes(capture.status);

  return (
    <div className="admin-shell">
      <header className="admin-head">
        <div>
          <span className="admin-kicker">{tr("管理员控制台")}</span>
          <h1>
            {authState === "authenticated"
              ? tr("反馈与指纹")
              : authState === "disabled"
                ? tr("管理员入口未开启")
                : tr("管理员登录")}
          </h1>
          <p>{tr("查看用户反馈，维护自定义模型指纹。")}</p>
        </div>
        {authState === "authenticated" && (
          <button
            className="admin-button ghost"
            type="button"
            disabled={logoutBusy}
            onClick={() => void logout()}
          >
            {tr("退出")}
          </button>
        )}
      </header>

      {authState === "checking" ? (
        <section
          className="admin-card admin-skeleton"
          role="status"
          aria-busy="true"
          aria-label={tr("正在检查管理员会话…")}
        >
          <div className="admin-skeleton-head">
            <span className="skeleton-line admin-sk-title" />
            <span className="skeleton-line admin-sk-actions" />
          </div>
          <span className="skeleton-line admin-sk-sub" />
          <div className="admin-skeleton-rows">
            <span className="skeleton-line admin-sk-row" />
            <span className="skeleton-line admin-sk-row short" />
            <span className="skeleton-line admin-sk-row" />
          </div>
          <p className="muted admin-sk-caption">{tr("正在检查管理员会话…")}</p>
        </section>
      ) : authState === "disabled" ? (
        <section className="admin-card admin-state-card" role="status">
          <div className="admin-login-icon" aria-hidden="true">
            <Icon name="ban" width="22" height="22" />
          </div>
          <h2>{tr("管理员入口未开启")}</h2>
          <p className="muted">
            {tr("服务端未配置 CHANSHUI_ADMIN_TOKEN（至少 24 位），管理控制台已关闭。")}
          </p>
          <button
            className="admin-button ghost"
            type="button"
            disabled={refreshing}
            onClick={() => void refresh()}
          >
            <Icon
              name={refreshing ? "spinner" : "refresh"}
              width="15"
              height="15"
              className={refreshing ? "icon-spin" : undefined}
            />
            {refreshing ? tr("刷新中…") : tr("重新加载")}
          </button>
        </section>
      ) : authState === "load_error" ? (
        <section className="admin-card admin-state-card">
          <div className="admin-login-icon is-bad" aria-hidden="true">
            <Icon name="shield" width="22" height="22" />
          </div>
          <h2>{tr("读取失败")}</h2>
          {error && (
            <div className="error-box" role="alert">
              {error}
            </div>
          )}
          <button
            className="admin-button ghost"
            type="button"
            disabled={refreshing}
            onClick={() => void refresh()}
          >
            <Icon
              name={refreshing ? "spinner" : "refresh"}
              width="15"
              height="15"
              className={refreshing ? "icon-spin" : undefined}
            />
            {refreshing ? tr("刷新中…") : tr("重新加载")}
          </button>
        </section>
      ) : authState !== "authenticated" ? (
        <section className="admin-card admin-login">
          <div className="admin-login-icon" aria-hidden="true">
            <Icon name="shield" width="22" height="22" />
          </div>
          <h2>{tr("进入反馈后台")}</h2>
          <p className="muted">{tr("输入服务端配置的 CHANSHUI_ADMIN_TOKEN。")}</p>
          <form onSubmit={submit}>
            <div className="field">
              <label htmlFor="admin-token">{tr("管理员口令")}</label>
              <div className="input-action">
                <input
                  id="admin-token"
                  type={show ? "text" : "password"}
                  value={token}
                  autoComplete="current-password"
                  onChange={(e) => setToken(e.target.value)}
                  autoFocus
                  spellCheck={false}
                  autoCapitalize="off"
                  required
                />
                <button
                  type="button"
                  onClick={() => setShow((v) => !v)}
                  aria-label={show ? tr("隐藏口令") : tr("显示口令")}
                >
                  <Icon name={show ? "eyeOff" : "eye"} />
                </button>
              </div>
              <small className="admin-hint">{tr("通常是 48 位随机字符串。")}</small>
            </div>
            {error && (
              <div className="error-box" role="alert">
                {error}
              </div>
            )}
            <button className="admin-button primary wide" disabled={busy}>
              {busy ? tr("正在核对…") : tr("进入")}
            </button>
          </form>
        </section>
      ) : (
        <>
          <div
            className="admin-tabs"
            role="tablist"
            aria-label={tr("后台模块")}
            onKeyDown={handleTabKeyDown}
          >
            <button
              id="admin-tab-feedback"
              type="button"
              role="tab"
              aria-selected={tab === "feedback"}
              aria-controls="admin-panel-feedback"
              tabIndex={tab === "feedback" ? 0 : -1}
              className={tab === "feedback" ? "active" : ""}
              onClick={() => setTab("feedback")}
            >
              {tr("反馈记录")}
              <span className="is-pill">{items ? items.length : 0}</span>
            </button>
            <button
              id="admin-tab-fingerprint"
              type="button"
              role="tab"
              aria-selected={tab === "fingerprint"}
              aria-controls="admin-panel-fingerprint"
              tabIndex={tab === "fingerprint" ? 0 : -1}
              className={tab === "fingerprint" ? "active" : ""}
              onClick={() => setTab("fingerprint")}
            >
              {tr("指纹采集")}
              <span className="is-pill">{refs.length}</span>
            </button>
          </div>

          {tab === "feedback" && (
            <section
              className="admin-card"
              role="tabpanel"
              id="admin-panel-feedback"
              aria-labelledby="admin-tab-feedback"
            >
              <div className="admin-card-head">
                <div>
                  <h2>{tr("反馈记录")}</h2>
                  <p>{tr("联系方式仅用于反馈处理，不会公开。")}</p>
                </div>
                <div className="admin-actions">
                  <button
                    className="admin-button ghost"
                    type="button"
                    onClick={() => void refresh()}
                    disabled={refreshing}
                  >
                    {refreshing ? tr("刷新中…") : tr("刷新")}
                  </button>
                  <button
                    className="admin-button ghost"
                    type="button"
                    disabled={!items || items.length === 0}
                    onClick={download}
                  >
                    {tr("导出 JSON")}
                  </button>
                </div>
              </div>
              {error && (
                <div className="error-box" role="alert">
                  {error}
                </div>
              )}
              {!items || items.length === 0 ? (
                <div className="admin-empty">
                  <Icon name="feedback" width="22" height="22" />
                  <b>{tr("还没有反馈")}</b>
                  <span>{tr("用户提交后会出现在这里。")}</span>
                </div>
              ) : (
                <div className="admin-list">
                  {items.map((item) => (
                    <article key={item.id}>
                      <header>
                        <b>{kinds[item.kind] || item.kind}</b>
                        <time>
                          {formatLocaleDate(item.created_at * 1000, lang, {
                            year: "numeric",
                            month: "2-digit",
                            day: "2-digit",
                            hour: "2-digit",
                            minute: "2-digit",
                            second: "2-digit",
                            hour12: false,
                          })}
                        </time>
                      </header>
                      <p className="admin-contact">{item.contact}</p>
                      <p>{item.message}</p>
                    </article>
                  ))}
                </div>
              )}
            </section>
          )}

          {tab === "fingerprint" && (
            <section
              className="admin-card"
              role="tabpanel"
              id="admin-panel-fingerprint"
              aria-labelledby="admin-tab-fingerprint"
            >
              <div className="admin-card-head">
                <div>
                  <h2>{tr("指纹采集与归因")}</h2>
                  <p>{tr("采集训练与保留样本，全部通过后写入归因库。")}</p>
                </div>
                <button
                  className="admin-button ghost"
                  type="button"
                  onClick={() => void fillSaved()}
                  disabled={fillBusy}
                >
                  {fillBusy ? tr("读取中…") : tr("填入上次保存")}
                </button>
              </div>
              {error && (
                <div className="error-box" role="alert">
                  {error}
                </div>
              )}

              <form className="admin-capture-form" onSubmit={startCapture}>
                <div className="admin-form-section">
                  <span className="admin-section-label">
                    01 · {tr("连接上游")}
                  </span>
                  <div className="admin-capture-grid">
                    <div className="field">
                      <label htmlFor="capture-base">{tr("接口地址")}</label>
                      <input
                        id="capture-base"
                        value={form.base_url}
                        onChange={(e) =>
                          setForm({ ...form, base_url: e.target.value })
                        }
                        placeholder="https://gateway.example"
                        required
                      />
                    </div>
                    <div className="field">
                      <label htmlFor="capture-key">API Key</label>
                      <div className="input-action">
                        <input
                          id="capture-key"
                          type={showCaptureKey ? "text" : "password"}
                          value={form.key}
                          onChange={(e) =>
                            setForm({ ...form, key: e.target.value })
                          }
                          placeholder={tr("只用于本次采集")}
                          autoComplete="off"
                          required
                        />
                        <button
                          type="button"
                          onClick={() => setShowCaptureKey((v) => !v)}
                          aria-label={
                            showCaptureKey ? tr("隐藏 Key") : tr("显示 Key")
                          }
                        >
                          <Icon name={showCaptureKey ? "eyeOff" : "eye"} />
                        </button>
                      </div>
                    </div>
                    <div className="field">
                      <label htmlFor="capture-model">{tr("请求模型名")}</label>
                      <div className="admin-model-field-row">
                        <CaptureModelCombobox
                          id="capture-model"
                          value={form.model}
                          options={captureModels}
                          loading={modelBusy}
                          placeholder="deepseek-flash"
                          onChange={applyModelChange}
                        />
                        <button
                          className="admin-button ghost admin-fetch-models"
                          type="button"
                          onClick={() => void listCaptureModels()}
                          disabled={modelBusy}
                        >
                          {modelBusy ? tr("拉取中…") : tr("拉取模型")}
                        </button>
                      </div>
                    </div>
                    <div className="field">
                      <label htmlFor="capture-protocol">{tr("协议")}</label>
                      <Select
                        id="capture-protocol"
                        value={form.protocol}
                        onChange={(e) =>
                          setForm({
                            ...form,
                            protocol: e.target.value as CaptureForm["protocol"],
                          })
                        }
                      >
                        <option value="auto">{tr("自适应")}</option>
                        <option value="chat">Chat</option>
                        <option value="anthropic">Anthropic</option>
                        <option value="openai">OpenAI</option>
                      </Select>
                    </div>
                  </div>
                </div>

                <div className="admin-form-section">
                  <span className="admin-section-label">
                    02 · {tr("归因信息")}
                  </span>
                  <div className="admin-capture-grid">
                    <div className="field">
                      <label htmlFor="capture-id">{tr("归因 ID")}</label>
                      <input
                        id="capture-id"
                        value={form.id}
                        onChange={(e) => {
                          const nextId = e.target.value;
                          setIdEdited(nextId.trim().length > 0);
                          setForm((f) => ({ ...f, id: nextId }));
                        }}
                        placeholder="deepseek-v4.1-flash-1017"
                      />
                    </div>
                    <div className="field">
                      <label htmlFor="capture-display">{tr("显示名称")}</label>
                      <input
                        id="capture-display"
                        value={form.display_name}
                        onChange={(e) =>
                          setForm({ ...form, display_name: e.target.value })
                        }
                        placeholder={tr("留空则使用归因 ID")}
                      />
                    </div>
                    <div className="field global-field">
                      <label htmlFor="capture-aliases">
                        {tr("别名（逗号分隔）")}
                      </label>
                      <input
                        id="capture-aliases"
                        value={form.aliases}
                        onChange={(e) =>
                          setForm({ ...form, aliases: e.target.value })
                        }
                        placeholder="deepseek-flash"
                      />
                    </div>
                  </div>
                </div>

                <div className="admin-form-section">
                  <span className="admin-section-label">
                    03 · {tr("采样参数")}
                  </span>
                  <div className="admin-capture-grid four">
                    <div className="field">
                      <label htmlFor="capture-train">{tr("训练样本")}</label>
                      <input
                        id="capture-train"
                        type="number"
                        min={30}
                        max={40}
                        value={form.train}
                        onChange={(e) =>
                          setForm({ ...form, train: Number(e.target.value) })
                        }
                      />
                    </div>
                    <div className="field">
                      <label htmlFor="capture-holdout">{tr("保留样本")}</label>
                      <input
                        id="capture-holdout"
                        type="number"
                        min={9}
                        max={12}
                        step={3}
                        value={form.holdout}
                        onChange={(e) =>
                          setForm({ ...form, holdout: Number(e.target.value) })
                        }
                      />
                    </div>
                    <div className="field">
                      <label htmlFor="capture-concurrency">{tr("并发")}</label>
                      <input
                        id="capture-concurrency"
                        type="number"
                        min={1}
                        max={4}
                        value={form.concurrency}
                        onChange={(e) =>
                          setForm({
                            ...form,
                            concurrency: Number(e.target.value),
                          })
                        }
                      />
                    </div>
                    <div className="field">
                      <label htmlFor="capture-timeout">{tr("超时 / 秒")}</label>
                      <input
                        id="capture-timeout"
                        type="number"
                        min={10}
                        max={600}
                        value={form.timeout}
                        onChange={(e) =>
                          setForm({ ...form, timeout: Number(e.target.value) })
                        }
                      />
                    </div>
                  </div>
                </div>

                {captureError && (
                  <div className="error-box" role="alert">
                    {captureError}
                  </div>
                )}
                <div className="admin-form-actions">
                  <button
                    className="admin-button primary"
                    disabled={captureBusy || captureActive}
                  >
                    {captureBusy ? tr("正在启动…") : tr("开始采集")}
                  </button>
                  {captureActive && (
                    <button
                      className="admin-button ghost"
                      type="button"
                      disabled={cancelBusy}
                      onClick={() => void cancelCapture()}
                    >
                      {cancelBusy ? tr("取消中…") : tr("取消")}
                    </button>
                  )}
                  <span className="admin-form-note">
                    {tr("保留样本全部命中新 ID 才会写入归因库。")}
                  </span>
                </div>
              </form>

              {capture && (
                <div className={`admin-progress ${capture.status}`}>
                  <div className="admin-progress-head">
                    <div className="admin-progress-title">
                      <svg
                        className="admin-progress-ring"
                        width="44"
                        height="44"
                        viewBox="0 0 44 44"
                        aria-hidden="true"
                      >
                        <circle
                          className="admin-progress-ring-track"
                          cx="22"
                          cy="22"
                          r={RING_RADIUS}
                        />
                        <circle
                          className="admin-progress-ring-arc"
                          cx="22"
                          cy="22"
                          r={RING_RADIUS}
                          strokeDasharray={RING_CIRCUMFERENCE}
                          strokeDashoffset={ringOffset}
                        />
                        <text
                          x="22"
                          y="22"
                          textAnchor="middle"
                          dominantBaseline="central"
                        >
                          {percent}%
                        </text>
                      </svg>
                      <span
                        className="admin-status-pill is-pill"
                        data-status={capture.status}
                      >
                        {captureActive && (
                          <i className="live-dot" aria-hidden="true" />
                        )}
                        <b>{tr(captureLabels[capture.status] || capture.status)}</b>
                      </span>
                    </div>
                    <span className="admin-progress-count">
                      {capture.completed}/{capture.total}
                    </span>
                  </div>
                  <div className="admin-progress-bar is-pill">
                    <i style={{ width: `${percent}%` }} />
                  </div>
                  <div className="admin-progress-meta">
                    <span>
                      {tr("训练可用")} {capture.train_usable}/
                      {capture.train_requested}
                    </span>
                    <span>
                      {tr("保留可用")} {capture.holdout_usable}/
                      {capture.holdout_requested}
                    </span>
                  </div>
                  {capture.error && (
                    <p className="admin-error-text">{capture.error}</p>
                  )}
                  {capture.status === "done" && capture.reference && (
                    <p className="admin-success-text">
                      {tr("已加入归因库")}：{capture.reference.id}
                    </p>
                  )}
                  {!!capture.groups?.length && (
                    <ol className="admin-capture-groups">
                      {capture.groups.map((g) => (
                        <li key={g.group}>
                          <span className="admin-group-index is-pill">
                            #{g.group}
                          </span>
                          <b className={`${g.pass ? "good" : "bad"} is-pill`}>
                            {g.pass ? tr("通过") : tr("未通过")}
                          </b>
                          <span className="admin-group-model">
                            {g.top_display || g.top_model}
                          </span>
                          <span className="admin-group-prob">
                            {(g.probability * 100).toFixed(2)}%
                          </span>
                        </li>
                      ))}
                    </ol>
                  )}
                </div>
              )}

              <div className="admin-ref-list">
                <div className="admin-subhead">
                  <b>{tr("自定义指纹")}</b>
                  <span>{refs.length}</span>
                </div>
                {refError && (
                  <div className="error-box" role="alert">
                    {refError}
                  </div>
                )}
                {refs.length === 0 ? (
                  <p className="muted admin-empty-line">
                    {tr("还没有自定义指纹。")}
                  </p>
                ) : (
                  refs.map((ref) => (
                    <article key={ref.id}>
                      <div>
                        <b>{ref.display_name || ref.id}</b>
                        <small>
                          {ref.family_name} ·{" "}
                          {tr("{n} 个训练样本", { n: ref.samples })}
                          {ref.captured_at
                            ? ` · ${formatLocaleDate(ref.captured_at, lang, {
                                year: "numeric",
                                month: "2-digit",
                                day: "2-digit",
                              })}`
                            : ""}
                        </small>
                      </div>
                      {confirmDeleteId === ref.id ? (
                        <div
                          className="admin-ref-confirm"
                          role="group"
                          aria-label={tr("确认删除")}
                        >
                          <span className="admin-ref-confirm-note">
                            {tr("删除后已有报告不会重算")}
                          </span>
                          <button
                            className="admin-button danger mini"
                            type="button"
                            disabled={deletingId === ref.id}
                            onClick={() => void removeReference(ref.id)}
                          >
                            {deletingId === ref.id
                              ? tr("删除中…")
                              : tr("确认删除")}
                          </button>
                          <button
                            className="admin-button ghost mini"
                            type="button"
                            disabled={deletingId === ref.id}
                            onClick={() => setConfirmDeleteId(null)}
                          >
                            {tr("取消")}
                          </button>
                        </div>
                      ) : (
                        <button
                          className="admin-button ghost mini"
                          type="button"
                          onClick={() => {
                            setRefError("");
                            setConfirmDeleteId(ref.id);
                          }}
                        >
                          {tr("删除")}
                        </button>
                      )}
                    </article>
                  ))
                )}
              </div>
            </section>
          )}
        </>
      )}
    </div>
  );
}
