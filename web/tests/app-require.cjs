const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const ts = require('typescript');
const cache = new Map();

function resolveTs(base) {
  const candidates = [
    base,
    base + '.ts',
    base + '.tsx',
    path.join(base, 'index.ts'),
    path.join(base, 'index.tsx'),
  ];
  return candidates.find((p) => {
    try {
      return fs.statSync(p).isFile();
    } catch {
      return false;
    }
  });
}

function appRequire(name, fromDir = __dirname) {
  let file;
  if (name.startsWith('@/')) {
    const base = path.join(__dirname, '..', name.slice(2));
    file = resolveTs(base);
    if (!file) throw new Error('Missing application module: ' + name);
  } else if (name.startsWith('./') || name.startsWith('../')) {
    const base = path.resolve(fromDir, name);
    file = resolveTs(base);
    if (!file) return require(path.resolve(fromDir, name));
  } else {
    return require(name);
  }
  if (cache.has(file)) return cache.get(file);
  const compiled = ts.transpileModule(fs.readFileSync(file, 'utf8'), {
    compilerOptions: {
      module: ts.ModuleKind.CommonJS,
      jsx: ts.JsxEmit.ReactJSX,
      esModuleInterop: true,
    },
  }).outputText;
  const fileDir = path.dirname(file);
  const mod = { exports: {} };
  const scope = {
    exports: mod.exports,
    module: mod,
    require: (dep) => appRequire(dep, fileDir),
    process,
    URLSearchParams,
    console,
    Buffer,
    __filename: file,
    __dirname: fileDir,
  };
  vm.runInNewContext(compiled, scope, { filename: file });
  cache.set(file, mod.exports);
  return mod.exports;
}

module.exports = appRequire;
