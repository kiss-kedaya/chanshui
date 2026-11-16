const test = require("node:test");
const assert = require("node:assert/strict");
const { staleChunks } = require("./app-require.cjs")("@/lib/build-guard");

test("a page built from the same chunks stays put", () => {
  assert.equal(staleChunks(["/_next/static/chunks/a.js", "/_next/static/chunks/b.js"], ["/_next/static/chunks/a.js"]), false);
});

test("a chunk the page never loaded means the page is old", () => {
  assert.equal(staleChunks(["/_next/static/chunks/a.js"], ["/_next/static/chunks/a.js", "/_next/static/chunks/b.js"]), true);
});