const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const vm = require("node:vm");
const ts = require("typescript");

const source = fs.readFileSync(path.join(__dirname, "../lib/polling.ts"), "utf8");
const compiled = ts.transpileModule(source, {
  compilerOptions: { module: ts.ModuleKind.CommonJS },
}).outputText;

function fixture() {
  const timers = new Map();
  const documentListeners = new Map();
  const windowListeners = new Map();
  let nextTimer = 0;
  const eventTarget = (listeners) => ({
    addEventListener: (name, listener) => listeners.set(name, listener),
    removeEventListener: (name) => listeners.delete(name),
  });
  const document = { visibilityState: "visible", ...eventTarget(documentListeners) };
  const navigator = { onLine: true };
  const scope = {
    exports: {},
    AbortController,
    document,
    navigator,
    window: eventTarget(windowListeners),
    setTimeout: (callback, delay) => {
      const id = ++nextTimer;
      timers.set(id, { callback, delay });
      return id;
    },
    clearTimeout: (id) => timers.delete(id),
  };
  vm.runInNewContext(compiled, scope);
  return {
    ...scope.exports,
    document,
    navigator,
    timers,
    documentListeners,
    windowListeners,
    tick: async () => {
      const [id, timer] = timers.entries().next().value;
      timers.delete(id);
      await timer.callback();
      return timer.delay;
    },
  };
}

const settle = () => new Promise((resolve) => setImmediate(resolve));

test("unchanged reports back off, do not rerender, and terminal reports stop", async () => {
  const f = fixture();
  const values = [];
  const errors = [];
  let requests = 0;
  let done = false;
  f.watchResource({
    load: async () => { requests++; return { status: done ? "done" : "running" }; },
    onValue: (value) => values.push(value),
    isActive: (value) => value.status === "running",
    onError: (error) => errors.push(error),
  });
  await settle();
  assert.equal(requests, 1);
  assert.equal(values.length, 1);
  assert.equal(await f.tick(), 1500);
  assert.equal(await f.tick(), 2500);
  assert.equal(await f.tick(), 3500);
  assert.equal(await f.tick(), 4500);
  assert.equal(values.length, 1);
  done = true;
  assert.equal(await f.tick(), 5000);
  assert.equal(values.length, 2);
  assert.equal(values[1].status, "done");
  assert.equal(errors.length, 0);
  assert.equal(f.timers.size, 0);
  assert.equal(f.documentListeners.size, 0);
  assert.equal(f.windowListeners.size, 0);
});

test("hidden pages abort their request and resume without overlap or stale updates", async () => {
  const f = fixture();
  const pending = [];
  const values = [];
  const errors = [];
  const stop = f.watchResource({
    load: (signal) => new Promise((resolve, reject) => {
      signal.addEventListener("abort", () => reject(new Error("cancelled")));
      pending.push({ signal, resolve });
    }),
    onValue: (value) => values.push(value),
    isActive: () => true,
    onError: (error) => errors.push(error),
  });
  assert.equal(pending.length, 1);
  f.document.visibilityState = "hidden";
  f.documentListeners.get("visibilitychange")();
  assert.equal(pending[0].signal.aborted, true);
  f.document.visibilityState = "visible";
  f.documentListeners.get("visibilitychange")();
  await settle();
  assert.equal(pending.length, 2);
  pending[0].resolve({ status: "old" });
  pending[1].resolve({ status: "running" });
  await settle();
  assert.equal(values.length, 1);
  assert.equal(values[0].status, "running");
  assert.equal(errors.length, 0);
  stop();
  assert.equal(f.timers.size, 0);
});

test("offline or initially hidden pages send no requests until visible and online", async () => {
  const f = fixture();
  let requests = 0;
  f.document.visibilityState = "hidden";
  const stop = f.watchResource({
    load: async () => { requests++; return null; },
    onValue: () => assert.fail("304 must not trigger render"),
    isActive: () => true,
    onError: () => assert.fail("unexpected error"),
  });
  await settle();
  assert.equal(requests, 0);
  f.navigator.onLine = false;
  f.document.visibilityState = "visible";
  f.documentListeners.get("visibilitychange")();
  await settle();
  assert.equal(requests, 0);
  f.navigator.onLine = true;
  f.windowListeners.get("online")();
  await settle();
  assert.equal(requests, 1);
  assert.equal(await f.tick(), 2500);
  assert.equal(requests, 2);
  stop();
});
