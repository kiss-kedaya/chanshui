const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const vm = require("node:vm");
const ts = require("typescript");
const source = fs.readFileSync(path.join(__dirname,"../lib/report-view.ts"),"utf8");
const compiled = ts.transpileModule(source,{compilerOptions:{module:ts.ModuleKind.CommonJS}}).outputText;
const scope = {exports:{},URL};
vm.runInNewContext(compiled,scope);
const {reportHost,reportPath,parseReportID,probabilityVisible,fingerprintState,presentedTotal}=scope.exports;
test("hosts exclude prefixes, paths, queries and credentials",()=>{
 assert.equal(reportHost("https://gateway.example/v1"),"gateway.example");
 assert.equal(reportHost("https://name:secret@gateway.example:8443/sub/v1?q=1"),"gateway.example:8443");
 assert.equal(reportHost("http://[::1]:8770/v1"),"[::1]:8770");
 assert.equal(reportHost("invalid"),"未知站点");
});
test("share lookup accepts exact IDs and copied links",()=>{
 const id="a1".repeat(16);
 assert.equal(parseReportID(id.toUpperCase()),id);
 assert.equal(parseReportID("http://localhost:8770"+reportPath(id)),id);
 for(const text of ["", "1", "script:alert(1)",id+"x"])assert.equal(parseReportID(text),"");
});
test("hide displayed 0.0 percent without changing nonzero probabilities",()=>{
 for(const value of [0,0.00001,0.00049,-1,NaN,Infinity])assert.equal(probabilityVisible(value),false);
 for(const value of [0.00051,0.001,0.99999,1])assert.equal(probabilityVisible(value),true);
});

test("fingerprint state separates confirmed swaps from suspicion",()=>{
  assert.equal(fingerprintState("openai/gpt-6.1-sol","gpt","gpt-6.1-sol"), "match");
  assert.equal(fingerprintState("openai/gpt-6.1-sol","gpt","gpt-6-sol"), "suspicious");
  assert.equal(fingerprintState("anthropic/claude-fable-5","claude","claude-fable-5"), "match");
  assert.equal(fingerprintState("deepseek-v4.1-flash-1017","deepseek","deepseek-flash"), "match");
  assert.equal(fingerprintState("deepseek-v4.1-flash-0910","deepseek","deepseek-flash"), "suspicious");
  assert.equal(fingerprintState("deepseek-flash","deepseek","deepseek-v4.1-flash-0910"), "suspicious");
  assert.equal(fingerprintState("deepseek-flash","deepseek","deepseek-v4.1-flash-1017"), "match");
  assert.equal(fingerprintState("deepseek-v4.1-flash","deepseek","deepseek-v4.1-flash-0910"), "match");
  assert.equal(fingerprintState("deepseek-v4-flash","deepseek","deepseek-flash"), "suspicious");
  assert.equal(fingerprintState("deepseek-flash","deepseek","deepseek-v4-pro"), "suspicious");
  assert.equal(fingerprintState("claude-opus-4-6","gpt","gpt-5.4"), "fake");
  assert.equal(fingerprintState("claude-opus-4-6","claude","claude-haiku-4-5"), "fake");
  assert.equal(fingerprintState("claude-opus-4-6","claude","claude-opus-4-5"), "suspicious");
  assert.equal(fingerprintState("claude-fable-5-2","claude","claude-opus-5-5"), "suspicious");
  assert.equal(fingerprintState("gpt-6-astra","gpt","gpt-5.6-luna"), "suspicious");
  assert.equal(fingerprintState("gemini-3.1-pro","unknown",""), "match");
  assert.equal(fingerprintState("gpt-5.4","openai","gpt-5.4"), "match");
  const parts = [
    {key:"fingerprint", contribution:32, coefficient:32},
    {key:"iq", contribution:27},
  ];
  const fake = presentedTotal(85, parts, "fake");
  assert.equal(fake.score, 53);
  assert.equal(fake.parts[0].contribution, 0);
  assert.equal(fake.parts[1].contribution, 27);
  const suspicious = presentedTotal(85, parts, "suspicious");
  assert.equal(suspicious.score, 65);
  assert.equal(suspicious.parts[0].contribution, 12);
  const match = presentedTotal(85, parts, "match");
  assert.equal(match.score, 85);
});

