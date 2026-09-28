/**
 * 标签事件规则条件的布尔表达式解析器。
 *
 * 镜像后端 `internal/models/tag/tag_event_rule.go` 的语法
 * （基于 `github.com/alecthomas/participle/v2` 构建）：
 *
 *   BoolExpr  := AndTerm ( 'OR'  AndTerm )*
 *   AndTerm   := NotFactor ( 'AND' NotFactor )*
 *   NotFactor := 'NOT'? Atom
 *   Atom      := Ident | '(' BoolExpr ')'
 *
 * Ident 为 `[\p{L}]([\p{L}\p{N}_-]*[\p{L}\p{N}])?`（Unicode 字母，故「登录失败」这类
 * 中文标签名可被接受），空白被忽略，AND / OR / NOT 作为运算符保留（按值匹配）。
 *
 * `parseBooleanExpr` 执行完整语法解析，返回与后端 `BoolExpr.String()` 相同的
 * 「规范化」结果，使前端无需服务端往返即可即时校验。
 */

import i18n from "@/utils/i18n";

export interface BooleanExprParseResult {
  ok: boolean;
  /** 规范化后的表达式，ok 为 true 时存在 */
  refined?: string;
  /** 可读的错误信息（已本地化），ok 为 false 时存在 */
  error?: string;
  /** 解析失败处的字符偏移量，可定位时存在 */
  offset?: number;
}

// exprMessage 将解析错误文案本地化：以 notice.tagExpr.<key> 命名的翻译键渲染，
// 避免把英文原文（如 "expression is empty"）直接暴露给用户。
const exprMessage = (key: string, params?: Record<string, unknown>): string =>
  i18n.global.t(`notice.tagExpr.${key}`, params ?? {});

type BoolExprTokenType = "ident" | "keyword" | "lparen" | "rparen";

interface BoolExprToken {
  type: BoolExprTokenType;
  value: string;
  offset: number;
}

const IDENT_RE = /^[\p{L}]([\p{L}\p{N}_-]*[\p{L}\p{N}])?/u;
const KEYWORD_SET = new Set(["AND", "OR", "NOT"]);

function tokenize(src: string): {
  tokens: BoolExprToken[];
  error?: { message: string; offset: number };
} {
  const tokens: BoolExprToken[] = [];
  let i = 0;
  const n = src.length;
  while (i < n) {
    const ch = src[i];
    if (/\s/.test(ch)) {
      i += 1;
      continue;
    }
    if (ch === "(") {
      tokens.push({ type: "lparen", value: ch, offset: i });
      i += 1;
      continue;
    }
    if (ch === ")") {
      tokens.push({ type: "rparen", value: ch, offset: i });
      i += 1;
      continue;
    }
    if (/[\p{L}]/u.test(ch)) {
      const m = IDENT_RE.exec(src.slice(i));
      if (m) {
        const value = m[0];
        tokens.push({
          type: KEYWORD_SET.has(value) ? "keyword" : "ident",
          value,
          offset: i,
        });
        i += value.length;
        continue;
      }
    }
    return {
      tokens,
      error: { message: exprMessage("unexpectedChar", { char: ch }), offset: i },
    };
  }
  return { tokens };
}

class BoolExprParseError extends Error {
  offset?: number;

  constructor(message: string, offset?: number) {
    super(message);
    this.offset = offset;
  }
}

class BoolExprParser {
  private pos = 0;

  constructor(private tokens: BoolExprToken[]) {}

  /** 解析整个 token 流；返回规范化后的表达式。 */
  parse(): string {
    const refined = this.parseOr();
    if (this.pos < this.tokens.length) {
      const tok = this.tokens[this.pos];
      throw new BoolExprParseError(
        exprMessage("unexpectedToken", { token: tok.value }),
        tok.offset,
      );
    }
    return refined;
  }

  private parseOr(): string {
    const parts = [this.parseAnd()];
    while (this.isKeyword("OR")) {
      this.pos += 1;
      parts.push(this.parseAnd());
    }
    return parts.join(" OR ");
  }

  private parseAnd(): string {
    const parts = [this.parseNotFactor()];
    while (this.isKeyword("AND")) {
      this.pos += 1;
      parts.push(this.parseNotFactor());
    }
    return parts.join(" AND ");
  }

  private parseNotFactor(): string {
    if (this.isKeyword("NOT")) {
      this.pos += 1;
      return `NOT ${this.parseAtom()}`;
    }
    return this.parseAtom();
  }

  private parseAtom(): string {
    const tok = this.peek();
    if (!tok) {
      throw new BoolExprParseError(exprMessage("unexpectedEnd"));
    }
    if (tok.type === "ident") {
      this.pos += 1;
      return tok.value;
    }
    if (tok.type === "lparen") {
      this.pos += 1;
      const inner = this.parseOr();
      const close = this.peek();
      if (!close || close.type !== "rparen") {
        throw new BoolExprParseError(exprMessage("missingClose"), close ? close.offset : undefined);
      }
      this.pos += 1;
      return `(${inner})`;
    }
    throw new BoolExprParseError(exprMessage("expectedOperand", { token: tok.value }), tok.offset);
  }

  private peek(): BoolExprToken | undefined {
    return this.tokens[this.pos];
  }

  private isKeyword(word: string): boolean {
    const tok = this.peek();
    return tok !== undefined && tok.type === "keyword" && tok.value === word;
  }
}

/**
 * 解析标签事件条件表达式并规范化。
 * 与后端 `ParseBoolExpr` + `BoolExpr.String()` 行为一致。
 */
export function parseBooleanExpr(src: string): BooleanExprParseResult {
  if (!src.trim()) {
    return { ok: false, error: exprMessage("empty"), offset: 0 };
  }
  const { tokens, error } = tokenize(src);
  if (error) {
    return { ok: false, error: error.message, offset: error.offset };
  }
  try {
    const refined = new BoolExprParser(tokens).parse();
    return { ok: true, refined };
  } catch (err) {
    if (err instanceof BoolExprParseError) {
      return { ok: false, error: err.message, offset: err.offset };
    }
    throw err;
  }
}

/**
 * 返回合法表达式的规范化形式，表达式非法时返回空字符串。
 */
export function refineBooleanExpr(src: string): string {
  return parseBooleanExpr(src).refined ?? "";
}
