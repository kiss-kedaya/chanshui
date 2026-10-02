const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const ts = require('typescript');
const cache = new Map();
module.exports = function appRequire(name) {
  if (!name.startsWith('@/')) return require(name);
  if (cache.has(name)) return cache.get(name);
  const base = path.join(__dirname, '..', name.slice(2));
  const file = [base + '.ts', base + '.tsx'].find(fs.existsSync);
  if (!file) throw new Error('Missing application module: ' + name);
  const compiled = ts.transpileModule(fs.readFileSync(file, 'utf8'), {
    compilerOptions: { module: ts.ModuleKind.CommonJS, jsx: ts.JsxEmit.ReactJSX },
  }).outputText;
  const scope = { exports: {}, require: module.exports };
  vm.runInNewContext(compiled, scope, { filename: file });
  cache.set(name, scope.exports);
  return scope.exports;
};
