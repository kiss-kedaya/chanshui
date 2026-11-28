type Translator = (source: string, vars?: Record<string, string | number>) => string;

/**
 * Translates backend-generated Chinese error messages, nested wrappers,
 * FriendlyError summaries, and Claude strictness/deep check details into
 * the active locale while leaving raw upstream English payloads untouched.
 */
export function translateError(text: string, tr: Translator): string {
  if (!text) return "";
  const trimmed = text.trim();
  if (!trimmed) return text;

  const direct = tr(trimmed);
  if (direct !== trimmed) return direct;

  // Recursive wrappers
  const abort = trimmed.match(/^整单失败：([\s\S]+)$/);
  if (abort) return tr("整单失败：{detail}", { detail: translateError(abort[1], tr) });

  const behav = trimmed.match(/^行为探针失败：([\s\S]+)$/);
  if (behav) return tr("行为探针失败：") + translateError(behav[1], tr);

  const fpInc = trimmed.match(/^指纹采样未完成（有效样本 (\d+)\/(\d+)）：([\s\S]+)$/);
  if (fpInc) {
    return tr("指纹采样未完成（有效样本 {a}/{b}）：{detail}", {
      a: fpInc[1],
      b: fpInc[2],
      detail: translateError(fpInc[3], tr),
    });
  }

  const injInc = trimmed.match(/^注入采样不足：(\d+)\/2 有效；([\s\S]+)$/);
  if (injInc) {
    return tr("注入采样不足：{n}/2 有效；{detail}", {
      n: injInc[1],
      detail: translateError(injInc[2], tr),
    });
  }

  const cacheStep = trimmed.match(/^第(\d+)次缓存请求：([\s\S]+)$/);
  if (cacheStep) {
    return tr("第 {n} 次缓存请求：{detail}", {
      n: cacheStep[1],
      detail: translateError(cacheStep[2], tr),
    });
  }

  // Claude cache first-write threshold error (reported in English UI screenshot)
  const claudePrefix = trimmed.match(/^Claude 缓存首写未建立：前缀已到 (\d+) 字符$/);
  if (claudePrefix) {
    return tr("Claude 缓存首写未建立：前缀已到 {n} 字符", { n: claudePrefix[1] });
  }

  // FriendlyError prefix followed by raw upstream detail on the same line
  const friendlyTail = trimmed.match(/^([^\n]+?；已尝试 \d+ 次)\s+([\s\S]+)$/);
  if (friendlyTail) {
    const head = translateError(friendlyTail[1], tr);
    if (head !== friendlyTail[1]) {
      return `${head} ${translateRedactedTokens(friendlyTail[2], tr)}`;
    }
  }

  // gateway.FriendlyError summary: "<msg>（HTTP <status>）；已尝试 <n> 次"
  const friendly = trimmed.match(/^(.+?)(?:（HTTP (\d+)）)?(?:；已尝试 (\d+) 次)?$/);
  if (friendly && (friendly[2] || friendly[3])) {
    const base = tr(friendly[1]);
    if (base !== friendly[1] || !/[\u4e00-\u9fff]/.test(friendly[1])) {
      if (friendly[2] && friendly[3]) {
        return tr("{msg}（HTTP {status}）；已尝试 {n} 次", {
          msg: base,
          status: friendly[2],
          n: friendly[3],
        });
      }
      if (friendly[2]) {
        return tr("{msg}（HTTP {status}）", { msg: base, status: friendly[2] });
      }
      if (friendly[3]) {
        return tr("{msg}；已尝试 {n} 次", { msg: base, n: friendly[3] });
      }
    }
  }

  // Strictness & Claude deep check details
  const unavail = trimmed.match(/^上游不可用\/限流（HTTP (\d+)），按不定处理$/);
  if (unavail) return tr("上游不可用/限流（HTTP {status}），按不定处理", { status: unavail[1] });

  const httpUncertain = trimmed.match(/^HTTP (\d+)，按不定处理$/);
  if (httpUncertain) return tr("HTTP {status}，按不定处理", { status: httpUncertain[1] });

  const httpSkip = trimmed.match(/^HTTP (\d+)，跳过$/);
  if (httpSkip) return tr("HTTP {status}，跳过", { status: httpSkip[1] });

  const httpRole = trimmed.match(/^HTTP (\d+)，未校验 role$/);
  if (httpRole) return tr("HTTP {status}，未校验 role", { status: httpRole[1] });

  const beta400 = trimmed.match(/^非法 beta 未返回 400（HTTP (\d+)）$/);
  if (beta400) return tr("非法 beta 未返回 400（HTTP {status}）", { status: beta400[1] });

  const compact = trimmed.match(/^stop_reason=compaction（输入 (\d+) tokens）$/);
  if (compact) return tr("stop_reason=compaction（输入 {n} tokens）", { n: compact[1] });

  const noRef = trimmed.match(/^没有该型号的官方参考值（in=(\d+)）$/);
  if (noRef) return tr("没有该型号的官方参考值（in={n}）", { n: noRef[1] });

  const refEq = trimmed.match(/^input_tokens=(\d+)，参考 (\d+)$/);
  if (refEq) return tr("input_tokens={a}，参考 {b}", { a: refEq[1], b: refEq[2] });

  const refHi = trimmed.match(/^input_tokens=(\d+) 高于参考 (\d+)，疑似注入$/);
  if (refHi) return tr("input_tokens={a} 高于参考 {b}，疑似注入", { a: refHi[1], b: refHi[2] });

  const refLo = trimmed.match(/^input_tokens=(\d+) 低于参考 (\d+)，不能单独证明 0 注入$/);
  if (refLo) return tr("input_tokens={a} 低于参考 {b}，不能单独证明 0 注入", { a: refLo[1], b: refLo[2] });

  const sigDiff = trimmed.match(/^两次签名不同（(\d+) \/ (\d+) 字节）$/);
  if (sigDiff) return tr("两次签名不同（{a} / {b} 字节）", { a: sigDiff[1], b: sigDiff[2] });

  const thinkEn = trimmed.match(/^thinking\.enabled 未被拒绝（HTTP (\d+)），无法检查报错文本$/);
  if (thinkEn) return tr("thinking.enabled 未被拒绝（HTTP {status}），无法检查报错文本", { status: thinkEn[1] });

  const maskParam = trimmed.match(/^报错里的参数名被打码：([\s\S]+)$/);
  if (maskParam) return tr("报错里的参数名被打码：{detail}", { detail: maskParam[1] });

  const noParam = trimmed.match(/^报错里没有参数名可供判断：([\s\S]+)$/);
  if (noParam) return tr("报错里没有参数名可供判断：{detail}", { detail: noParam[1] });

  const noMarker = trimmed.match(/^没有回显 marker，system 可能被剥掉或改写：([\s\S]+)$/);
  if (noMarker) return tr("没有回显 marker，system 可能被剥掉或改写：{detail}", { detail: noMarker[1] });

  const extCtx = trimmed.match(/^疑似注入请求外的上下文：([\s\S]+)$/);
  if (extCtx) return tr("疑似注入请求外的上下文：{detail}", { detail: extCtx[1] });

  const noDrop = trimmed.match(/^响应有 input_transformations 但没有 thinking_dropped：([\s\S]+)$/);
  if (noDrop) return tr("响应有 input_transformations 但没有 thinking_dropped：{detail}", { detail: noDrop[1] });

  const unkSec = trimmed.match(/^未知检测板块:\s*(.+)$/);
  if (unkSec) return tr("未知检测板块: {name}", { name: unkSec[1] });

  return translateRedactedTokens(trimmed, tr);
}

function translateRedactedTokens(text: string, tr: Translator): string {
  return text
    .replace(/\[密钥已隐藏\]/g, tr("[密钥已隐藏]"))
    .replace(/\[已隐藏\]/g, tr("[已隐藏]"));
}
