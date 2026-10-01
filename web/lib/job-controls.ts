export interface JobControl {
  jobId: string;
  token: string;
  expiresAt: number;
}

const prefix = "chanshui-job-retry:";
const lastJobKey = "chanshui-last-job";
const memory = new Map<string, JobControl>();
const transient = new Set<string>();
let lastJob = "";
const validID = (value: unknown): value is string =>
  typeof value === "string" && /^[a-f0-9]{32}$/.test(value);

function storage(): Storage | undefined {
  try {
    return typeof window === "undefined" ? undefined : window.localStorage;
  } catch {
    return undefined;
  }
}

export function forgetJobControl(jobId: string): void {
  if (!validID(jobId)) return;
  memory.delete(jobId);
  transient.delete(jobId);
  if (lastJob === jobId) lastJob = "";
  try {
    const local = storage();
    local?.removeItem(prefix + jobId);
    if (local?.getItem(lastJobKey) === jobId) local.removeItem(lastJobKey);
  } catch {}
}

export function getJobControl(jobId: string): JobControl | null {
  if (!validID(jobId)) return null;
  let value: JobControl | undefined = memory.get(jobId);
  try {
    const local = storage();
    if (local) {
      const raw = local.getItem(prefix + jobId);
      if (raw) {
        try { value = JSON.parse(raw); } catch { value = undefined; }
      } else if (!transient.has(jobId)) value = undefined;
    }
  } catch {}
  if (
    !value || value.jobId !== jobId || !validID(value.token) ||
    !Number.isFinite(value.expiresAt) || value.expiresAt <= Date.now() / 1000
  ) {
    forgetJobControl(jobId);
    return null;
  }
  memory.set(jobId, value);
  return value;
}

export function saveJobControl(value: JobControl): boolean {
  if (!validID(value.jobId) || !validID(value.token) || value.expiresAt <= Date.now() / 1000)
    return false;
  // Only this job's retry capability is durable; the upstream API key never is.
  const safe = { jobId: value.jobId, token: value.token, expiresAt: value.expiresAt };
  memory.set(value.jobId, safe);
  transient.add(value.jobId);
  lastJob = value.jobId;
  try {
    const local = storage();
    if (!local) return false;
    local.setItem(prefix + value.jobId, JSON.stringify(safe));
    transient.delete(value.jobId);
    local.setItem(lastJobKey, value.jobId);
    return true;
  } catch {
    return false;
  }
}

export function getLastControlledJob(): JobControl | null {
  try {
    const local = storage();
    const latest = local?.getItem(lastJobKey) || lastJob;
    const saved = getJobControl(latest);
    if (saved) return saved;
    if (local) {
      const ids: string[] = [];
      for (let i = 0; i < local.length; i++) {
        const key = local.key(i);
        if (key?.startsWith(prefix)) ids.push(key.slice(prefix.length));
      }
      for (const id of ids.reverse()) {
        const found = getJobControl(id);
        if (found) return found;
      }
    }
  } catch {}
  return getJobControl(lastJob);
}

export function reconcileJobControl(task: {
  id: string;
  status: string;
  expires_at: number;
  probes?: { status: string }[];
}): void {
  const unfinished = (task.probes || []).some((probe) =>
    ["queued", "running", "error", "cancelled"].includes(probe.status),
  );
  if ((task.status === "done" && !unfinished) || task.expires_at <= Date.now() / 1000)
    forgetJobControl(task.id);
}
