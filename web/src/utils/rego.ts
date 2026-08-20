/**
 * Rego helpers shared by the tag rule admin pages and the RegoEditor
 * component: tag/field literal extraction for heuristic expansion, metadata
 * driven input field snippets for autocomplete, Rego skeleton generation and
 * a lightweight local preflight checker.
 */

export interface RegoValidationError {
  line: number;
  column: number;
  message: string;
  /** lint severity key, e.g. "warning" / "error" */
  type?: string;
  /** optional hint rendered alongside the message */
  hint?: string;
}

/** Registered model type key for the HTTP transport input document. */
export const HTTP_TRANSPORT_MODEL_KEY =
  "zhongkui.com/zhongkui/internal/models/network.HttpTransport";

/** Common Rego keywords offered by the editor autocomplete. */
export const REGO_KEYWORDS = [
  "package",
  "import",
  "as",
  "default",
  "contains",
  "if",
  "else",
  "not",
  "some",
  "every",
  "with",
  "input",
  "data",
  "true",
  "false",
  "null",
];

/**
 * Completion position classification for the Rego editor autocomplete.
 * The editor suppresses or filters options that would be invalid at the
 * cursor location, so that e.g. `and`/`abs`/`any` are not suggested right
 * after a completed expression like `input.file_ext == ""`.
 */
export type RegoCompletionKind = "block" | "term" | "allowAll";

/** Keywords that can only start a statement/expression, never a term. */
const EXPRESSION_START_KEYWORDS = new Set([
  "package",
  "import",
  "default",
  "some",
  "every",
  "if",
  "else",
  "contains",
  "with",
  "in",
  "not",
  "as",
]);

/** Matches a fully closed string literal at the line end: `""`, `"..."` or `` `...` ``. */
const CLOSED_LITERAL_TAIL_RE =
  /(?:""|"(?:[^"\\]|\\.)*"|`(?:[^`]|\\.)*`|\d+(?:\.\d+)?|true|false|null)$/;

/** Comparison / assignment operators, looked up anywhere on the line. */
const COMPARISON_OP_RE = /(?:==|!=|<=|>=|<|>|:=|=)/;

/** Line endings that expect a term value: operators, commas or open brackets. */
const TERM_POSITION_RE = /(?:==|!=|<=|>=|<|>|:=|=|,|\[|\{|\()\s*$/;

/** Keywords that expect a following term on the same line. */
const TERM_KEYWORD_RE = /(?:\bif\b|\bin\b|\bnot\b|\bsome\b|\bevery\b|\bcontains\b)\s*$/;

/**
 * Classifies the completion context at the cursor given the line text before
 * the cursor (including any partial word being typed):
 * - "block": a completed expression/literal precedes the cursor, so any word
 *   typed here is a syntax error and no completion should be offered.
 * - "term": only a term (value) is expected here; statement-starting keywords
 *   must be filtered out of the completion list.
 * - "allowAll": no positional constraint, keep the default completions.
 */
export function classifyRegoCompletionContext(lineTextBefore: string): RegoCompletionKind {
  // Drop the partial word being typed so we classify the position itself.
  const before = lineTextBefore.replace(/[a-zA-Z_][a-zA-Z0-9_]*$/, "");

  // A closed literal right after a comparison/assignment completes the
  // expression: any further word on this line is a syntax error.
  if (CLOSED_LITERAL_TAIL_RE.test(before.trimEnd()) && COMPARISON_OP_RE.test(before)) {
    return "block";
  }
  if (TERM_POSITION_RE.test(before) || TERM_KEYWORD_RE.test(before)) {
    return "term";
  }
  return "allowAll";
}

/** Returns true when the completion label is a statement-starting keyword. */
export function isRegoExpressionKeyword(label: string): boolean {
  return EXPRESSION_START_KEYWORDS.has(label);
}

/** Item metadata for array-typed fields, e.g. the element type of `items`. */
export interface InputItemMeta {
  type: string;
  goType: string;
}

/**
 * Field metadata of an input model document as returned by the
 * `/admin/tag-rule/model/metadata` endpoint.
 */
export interface InputFieldMeta {
  /** input field name, e.g. "method" */
  field: string;
  /** semantic type key, e.g. "string" | "number" | "boolean" | "object" | "array" */
  type: string;
  /** Go type name of the backend model field */
  goType: string;
  /** brief description shown in the autocomplete popup */
  description?: string;
  /** whether the field carries sensitive data */
  sensitive?: boolean;
  /** element metadata for array-typed fields */
  items?: InputItemMeta;
}

/** A sample input document offered for dry-run previews. */
export interface InputSample {
  name: string;
  description?: string;
  input: Record<string, unknown>;
}

/** A selectable model option, e.g. rendered as a `<v-select>` item. */
export interface ModelOption {
  value: string;
  label: string;
}

/**
 * Builds an insertable Rego snippet for an input field based on its semantic
 * type. Fields comparable with a literal become `input.<field> == <literal>`
 * (string/number/boolean); object, array and sensitive fields cannot be
 * compared for equality, so only the bare `input.<field>` reference is
 * produced. Unknown types fall back to the bare reference as well.
 */
export function fieldSnippet(field: InputFieldMeta): string {
  const ref = `input.${field.field}`;
  switch (field.type) {
    case "string":
      return `${ref} == ""`;
    case "number":
      return `${ref} == 0`;
    case "boolean":
      return `${ref} == true`;
    case "object":
    case "array":
    case "sensitive":
      return ref;
    default:
      return ref;
  }
}

const STRING_LITERAL_RE = /("(?:[^"\\]|\\.)*"|`(?:[^`]|\\.)*`)/g;

function unquoteLiteral(raw: string): string {
  const inner = raw.slice(1, -1);
  return raw.startsWith("`") ? inner : inner.replace(/\\(.)/g, "$1");
}

/**
 * Extracts tag string literals declared by a Rego rule:
 * `tags contains "x"` and `tags := { "a", "b" }` forms.
 */
export function extractTagLiterals(rego: string): string[] {
  const tags = new Set<string>();
  const containsRe = /tags\s+contains\s+("(?:[^"\\]|\\.)*"|`(?:[^`]|\\.)*`)/g;
  let m: RegExpExecArray | null;
  while ((m = containsRe.exec(rego)) !== null) {
    const value = unquoteLiteral(m[1]);
    if (value) {
      tags.add(value);
    }
  }
  const setRe = /tags\s*:=\s*\{([\s\S]*?)\}/g;
  while ((m = setRe.exec(rego)) !== null) {
    const body = m[1];
    let lit: RegExpExecArray | null;
    while ((lit = STRING_LITERAL_RE.exec(body)) !== null) {
      const value = unquoteLiteral(lit[1]);
      if (value) {
        tags.add(value);
      }
    }
  }
  return [...tags];
}

/**
 * Extracts the first-segment field names referenced as `input.<field>` in a
 * Rego rule, deduplicated in first-seen order.
 */
export function extractInputFields(rego: string): string[] {
  const fields: string[] = [];
  const seen = new Set<string>();
  const re = /input\.([a-zA-Z_][a-zA-Z0-9_]*)/g;
  let m: RegExpExecArray | null;
  while ((m = re.exec(rego)) !== null) {
    if (!seen.has(m[1])) {
      seen.add(m[1]);
      fields.push(m[1]);
    }
  }
  return fields;
}

/**
 * Builds a Rego skeleton following the `package zki` + `tags contains`
 * contract, using the given rule name as the produced tag.
 */
export function buildRegoSkeleton(name: string): string {
  const tag = name.trim() || "tag";
  return [
    "package zki",
    "",
    `tags contains "${tag}" if {`,
    "\t# TODO: add tag conditions",
    "}",
    "",
  ].join("\n");
}

/**
 * Lightweight local Rego preflight checks for instant feedback while typing.
 * Checks bracket/quote pairing and the required `package zki` / `tags`
 * declarations. Not a substitute for the authoritative backend validation.
 */
export function preflightRego(src: string): RegoValidationError[] {
  const errors: RegoValidationError[] = [];
  const stack: { ch: string; line: number; column: number }[] = [];
  let line = 1;
  let column = 1;
  let inComment = false;
  let inString: string | null = null;
  let escaped = false;

  const push = (message: string, l: number, c: number) =>
    errors.push({ line: l, column: c, message });

  for (let i = 0; i < src.length; i++) {
    const ch = src[i];
    if (inComment) {
      if (ch === "\n") {
        inComment = false;
        line += 1;
        column = 1;
      } else {
        column += 1;
      }
      continue;
    }
    if (inString) {
      if (escaped) {
        escaped = false;
      } else if (ch === "\\") {
        escaped = true;
      } else if (ch === inString) {
        inString = null;
      }
      column += 1;
      continue;
    }
    if (ch === "#") {
      inComment = true;
      continue;
    }
    if (ch === '"' || ch === "`") {
      inString = ch;
      column += 1;
      continue;
    }
    if (ch === "{" || ch === "(") {
      stack.push({ ch, line, column });
      column += 1;
      continue;
    }
    if (ch === "}" || ch === ")") {
      const open = stack.pop();
      const expected = ch === "}" ? "{" : "(";
      if (!open || open.ch !== expected) {
        push(`unmatched '${ch}'`, line, column);
      }
      column += 1;
      continue;
    }
    if (ch === "\n") {
      line += 1;
      column = 1;
      continue;
    }
    column += 1;
  }

  if (inString) {
    push("unterminated string", line, column);
  }
  for (const open of stack) {
    push(`unmatched '${open.ch}'`, open.line, open.column);
  }
  if (!/\bpackage\s+zki\b/.test(src)) {
    push("missing `package zki`", 1, 1);
  }
  if (!/tags\s+(contains|:=)/.test(src)) {
    push("missing `tags` output declaration", 1, 1);
  }

  errors.sort((a, b) => a.line - b.line || a.column - b.column);
  return errors;
}
