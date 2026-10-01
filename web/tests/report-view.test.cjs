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
const {reportHost,reportPath,parseReportID,probabilityVisible}=scope.exports;
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
