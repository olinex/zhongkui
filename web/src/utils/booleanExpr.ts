/**
 * Boolean expression parser for tag event rule conditions.
 *
 * Mirrors the backend grammar in `internal/models/tag/tag_event_rule.go`
 * (built with `github.com/alecthomas/participle/v2`):
 *
 *   BoolExpr  := AndTerm ( 'OR'  AndTerm )*
 *   AndTerm   := NotFactor ( 'AND' NotFactor )*
 *   NotFactor := 'NOT'? Atom
 *   Atom      := Ident | '(' BoolExpr ')'
 *
 * Ident is `[\p{L}]([\p{L}\p{N}_-]*[\p{L}\p{N}])?` (Unicode letters, so Chinese
 * tag names like "登录失败" are accepted), whitespace is elided, and the tokens
 * AND / OR / NOT are reserved as operators (matched by value).
 *
 * `parseBooleanExpr` performs a full grammar parse and returns the same
 * "refined" normalization as the backend `BoolExpr.String()` so the frontend
 * can provide instant validation feedback without a server round-trip.
 */

import i18n from "@/utils/i18n";

export interface BooleanExprParseResult {
  ok: boolean;
  /** normalized expression, present when ok */
  refined?: string;
  /** human-readable error message (已本地化), present when not ok */
  error?: string;
  /** character offset where parsing failed, when locatable */
  offset?: number;
}

// exprMessage 将解析错误文案本地化：以 notice.tagExpr.<key> 命名的翻译键渲染，
// 避免把英文原文（如 "expression is empty"）直接暴露给用户。
// exprMessage localizes parse-error messages via the notice.tagExpr.<key> keys,
// so raw English strings (e.g. "expression is empty") are not shown to users.
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

  /** Parses the whole token stream; returns the refined expression. */
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
 * Parses a tag event condition expression and normalizes it.
 * Mirrors the backend `ParseBoolExpr` + `BoolExpr.String()` behavior.
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
 * Returns the normalized form of a valid expression, or an empty string
 * when the expression is invalid.
 */
export function refineBooleanExpr(src: string): string {
  return parseBooleanExpr(src).refined ?? "";
}
