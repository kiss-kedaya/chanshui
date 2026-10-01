"use client";
import { useRef, useState } from "react";
import Icon from "@/components/Icon";
import Select from "@/components/Select";
import { submitFeedback } from "@/lib/api";
export default function Feedback() {
  const dialog = useRef<HTMLDialogElement>(null),
    trigger = useRef<HTMLButtonElement>(null),
    pending = useRef(false);
  const [kind, setKind] = useState("bug"),
    [contact, setContact] = useState(""),
    [message, setMessage] = useState(""),
    [busy, setBusy] = useState(false),
    [error, setError] = useState(""),
    [success, setSuccess] = useState("");
  function open() {
    setError("");
    setSuccess("");
    dialog.current?.showModal();
  }
  function close() {
    if (!pending.current) dialog.current?.close();
  }
  async function send() {
    if (pending.current) return;
    pending.current = true;
    setBusy(true);
    setError("");
    try {
      const result = await submitFeedback({
        kind,
        contact: contact.trim(),
        message: message.trim(),
      });
      setSuccess(result.id);
      setContact("");
      setMessage("");
    } catch (e) {
      setError((e as Error).message);
    } finally {
      pending.current = false;
      setBusy(false);
    }
  }
  return (
    <>
      <button
        type="button"
        ref={trigger}
        className="feedback-trigger"
        aria-label="提交反馈"
        onClick={open}
      >
        <Icon name="feedback" />
        <span>提交反馈</span>
        <Icon name="arrow" />
      </button>
      <dialog
        ref={dialog}
        className="feedback-dialog"
        onClose={() => trigger.current?.focus()}
        onCancel={(e) => {
          if (pending.current) e.preventDefault();
        }}
        onClick={(e) => {
          if (e.target === e.currentTarget) {
            const r = e.currentTarget.getBoundingClientRect();
            if (
              e.clientX < r.left ||
              e.clientX > r.right ||
              e.clientY < r.top ||
              e.clientY > r.bottom
            )
              close();
          }
        }}
        aria-labelledby="feedback-title"
      >
        <div className="feedback-heading">
          <span className="feedback-symbol">
            <Icon name={success ? "check" : "feedback"} />
          </span>
          <button
            type="button"
            className="icon-button"
            aria-label="关闭反馈"
            onClick={close}
            disabled={busy}
          >
            <Icon name="close" />
          </button>
        </div>
        <h2 id="feedback-title">{success ? "反馈已收到" : "让检测更好一点"}</h2>
        {success ? (
          <div className="feedback-success">
            <p>感谢反馈，我们会根据你留下的方式联系。</p>
            <span className="muted">反馈编号</span>
            <code>{success}</code>
            <button className="btn-primary" onClick={close}>
              完成
            </button>
          </div>
        ) : (
          <form
            onSubmit={(e) => {
              e.preventDefault();
              void send();
            }}
          >
            <p className="feedback-description">
              分享更好的检测方法，或告诉我们哪里出了问题。
            </p>
            <div className="field">
              <label htmlFor="feedback-kind">反馈类型</label>
              <Select
                id="feedback-kind"
                value={kind}
                onChange={(e) => setKind(e.target.value)}
                disabled={busy}
              >
                <option value="bug">问题反馈</option>
                <option value="method">检测方法</option>
                <option value="suggestion">改进建议</option>
              </Select>
            </div>
            <div className="field">
              <label htmlFor="feedback-contact">联系方式</label>
              <input
                id="feedback-contact"
                value={contact}
                onChange={(e) => setContact(e.target.value)}
                required
                minLength={2}
                maxLength={200}
                disabled={busy}
                placeholder="邮箱 / Telegram / QQ"
                autoComplete="off"
              />
            </div>
            <div className="field">
              <label htmlFor="feedback-message">问题或建议</label>
              <textarea
                id="feedback-message"
                value={message}
                onChange={(e) => setMessage(e.target.value)}
                required
                minLength={5}
                maxLength={4000}
                disabled={busy}
                placeholder="描述现象或检测方法，请勿填写 API 密钥。"
              />
            </div>
            {error && (
              <p className="error-box" role="alert">
                {error}
              </p>
            )}
            <div className="feedback-footer">
              <span>联系方式仅用于反馈处理</span>
              <button
                className="btn-primary"
                type="submit"
                disabled={busy}
                aria-busy={busy}
              >
                {busy && <Icon name="spinner" className="icon-spin" />}
                {busy ? "提交中…" : "提交反馈"}
              </button>
            </div>
          </form>
        )}
      </dialog>
    </>
  );
}
