const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const vm = require("node:vm");
const ts = require("typescript");
const source = fs.readFileSync(path.join(__dirname, "../lib/job-controls.ts"), "utf8");
const compiled = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.CommonJS } }).outputText;
function localStore() {
  const values = new Map();
  return {
    values,
    get length() { return values.size; },
    getItem(key) { return values.get(key) ?? null; },
    setItem(key, value) { values.set(key, value); },
    removeItem(key) { values.delete(key); },
    key(index) { return [...values.keys()][index] ?? null; },
  };
}
function boot(localStorage) {
  const scope = { exports: {}, window: { localStorage }, Date, Map };
  vm.runInNewContext(compiled, scope);
  return scope.exports;
}
const job = (letter = "a") => ({ jobId: letter.repeat(32), token: "f".repeat(32), expiresAt: Math.floor(Date.now() / 1000) + 86400 });

test("retry keys survive reload and remain tied to their job", () => {
  const store = localStore();
  const first = boot(store);
  const a = job();
  const b = { ...job("b"), token: "c".repeat(32) };
  assert.equal(first.saveJobControl({ ...a, api_key: "must-not-persist" }), true);
  assert.equal(first.saveJobControl(b), true);
  const reloaded = boot(store);
  assert.equal(reloaded.getJobControl(a.jobId).token, a.token);
  assert.equal(reloaded.getJobControl(b.jobId).token, b.token);
  assert.equal(reloaded.getLastControlledJob().jobId, b.jobId);
  assert.equal(reloaded.getJobControl("d".repeat(32)), null);
  assert.equal([...store.values.values()].join("").includes("must-not-persist"), false);
});

test("only all-completed or expired jobs lose their retry key", () => {
  const store = localStore();
  const api = boot(store);
  const a = job();
  const b = job("b");
  api.saveJobControl(a);
  api.saveJobControl(b);
  for (const status of ["queued", "running", "partial", "cancelled", "interrupted"]) {
    api.reconcileJobControl({ id: a.jobId, status, expires_at: a.expiresAt });
    assert.equal(api.getJobControl(a.jobId).token, a.token);
  }
  api.reconcileJobControl({ id: a.jobId, status: "done", expires_at: a.expiresAt });
  assert.equal(boot(store).getJobControl(a.jobId), null);
  api.saveJobControl(a);
  api.reconcileJobControl({ id: a.jobId, status: "done", expires_at: a.expiresAt, probes: [{ status: "error" }] });
  assert.equal(boot(store).getJobControl(a.jobId).token, a.token);
  api.reconcileJobControl({ id: a.jobId, status: "done", expires_at: a.expiresAt, probes: [{ status: "done" }] });
  assert.equal(boot(store).getJobControl(a.jobId), null);
  assert.equal(boot(store).getJobControl(b.jobId).token, b.token);
  api.reconcileJobControl({ id: b.jobId, status: "partial", expires_at: 1 });
  assert.equal(boot(store).getJobControl(b.jobId), null);
});

test("corrupt, mismatched and expired stored records are discarded", () => {
  for (const value of ["broken", "null", JSON.stringify({ ...job(), jobId: "b".repeat(32) }), JSON.stringify({ ...job(), expiresAt: 1 })]) {
    const store = localStore();
    store.setItem("chanshui-job-retry:" + job().jobId, value);
    const api = boot(store);
    assert.equal(api.getJobControl(job().jobId), null);
    assert.equal(store.getItem("chanshui-job-retry:" + job().jobId), null);
  }
});

test("blocked browser storage does not break an in-progress job", () => {
  const denied = { getItem() { throw Error("denied"); }, setItem() { throw Error("denied"); }, removeItem() { throw Error("denied"); } };
  const api = boot(denied);
  assert.equal(api.saveJobControl(job()), false);
  assert.equal(api.getJobControl(job().jobId).token, job().token);
});

test("completion in another tab invalidates an already loaded key", () => {
  const store = localStore();
  const tabA = boot(store);
  const tabB = boot(store);
  tabA.saveJobControl(job());
  assert.equal(tabB.getJobControl(job().jobId).token, job().token);
  tabA.reconcileJobControl({ id: job().jobId, status: "done", expires_at: job().expiresAt });
  assert.equal(tabB.getJobControl(job().jobId), null);
});
