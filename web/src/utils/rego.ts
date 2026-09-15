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
 * `/tag-event/tag-rule/model/metadata` endpoint.
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

// ---- custom OPA builtins & sqlite vtable metadata ----

/**
 * Metadata of a custom OPA builtin callable inside rego rules, as returned by
 * the `/tag-event/tag-rule/functions` endpoint. Drives the Rego editor's
 * function keyword highlight, hover signature hints and autocomplete.
 */
export interface RegoFunctionMeta {
  /** full function name, e.g. "sqlite.query" */
  name: string;
  /** localized description shown in the autocomplete popup / hover tooltip */
  description: string;
  /** human-readable call signature, e.g. "sqlite.query(sql, params)" */
  signature: string;
  /** optional usage example text */
  example?: string;
}

/** Metadata of a single column of a sqlite vtable datasource. */
export interface SqliteColumnMeta {
  name: string;
  /** value type name, e.g. "text" | "integer" | "real" | "blob" | "any" */
  type: string;
}

/**
 * Metadata of a third-party datasource registered as an sqlite vtable, as
 * returned by the `/tag-event/tag-rule/sqlite/meta` endpoint. Drives SQL
 * table/column completion inside `sqlite.query(...)` strings.
 */
export interface SqliteDatasourceMeta {
  /** bare table name referenced by sqlite.query, e.g. "ip_geo" */
  name: string;
  columns: SqliteColumnMeta[];
}

/**
 * SQL keywords offered by the editor for keyword highlight and autocomplete
 * inside `sqlite.query("...")` strings. SQLite read-only SELECT subset.
 */
export const SQL_KEYWORDS = [
  "select",
  "from",
  "where",
  "and",
  "or",
  "not",
  "null",
  "as",
  "join",
  "left",
  "right",
  "inner",
  "outer",
  "cross",
  "on",
  "order",
  "by",
  "group",
  "having",
  "limit",
  "offset",
  "distinct",
  "count",
  "sum",
  "avg",
  "min",
  "max",
  "case",
  "when",
  "then",
  "else",
  "end",
  "between",
  "like",
  "in",
  "is",
  "true",
  "false",
  "asc",
  "desc",
];

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

/** 一个开箱场景样例：标题/说明的 i18n 键 + 可插入编辑器的完整 rego 片段。 */
export interface DatasourceSample {
  /** 样例标题的 i18n 键 */
  titleKey: string;
  /** 样例一句话说明的 i18n 键 */
  descKey: string;
  /** 插入编辑器的完整 rego 片段（package zki + tags contains 契约） */
  snippet: string;
}

/**
 * buildDatasourceSamples 基于当前命中的数据源 meta 组装开箱场景样例，演示
 * sqlite.query 的存在性判断 / 取单值 / 聚合统计三种用法（含 `?` 参数绑定与
 * 行对象字段访问）。生成的片段以 `package zki` + `tags contains` 契约包裹，
 * 可直接试运行。table 为数据源裸名（sqlite.query 中 FROM 引用的表名）。
 */
export function buildDatasourceSamples(
  ds: SqliteDatasourceMeta,
  tag = "tag",
  field = "",
): DatasourceSample[] {
  const table = ds.name;
  const c1 = ds.columns[0]?.name ?? "col";
  const c2 = ds.columns[1]?.name ?? c1;
  const target = field ? `input.${field}` : "input.<字段>";
  const task = tag.trim() || "tag";

  const exists = [
    "package zki",
    "",
    `tags contains "${task}" if {`,
    `\t# 存在性判断：命中任一匹配行即成立（COUNT(*) > 0）`,
    `\trows := sqlite.query("SELECT COUNT(*) AS n FROM ${table} WHERE ${c1} = ?", [${target}])`,
    "\trows[0].n > 0",
    "}",
    "",
  ].join("\n");

  const value = [
    "package zki",
    "",
    `tags contains "${task}" if {`,
    `\t# 取单值：SQL 结果每行是一个对象，以 rows[0].<列> 取字段（参数 \`?\` 与数组一一对应）`,
    `\trows := sqlite.query("SELECT ${c2} FROM ${table} WHERE ${c1} = ? LIMIT 1", [${target}])`,
    "\tcount(rows) > 0",
    `\trows[0].${c2} == "期望值"`,
    "}",
    "",
  ].join("\n");

  const aggregate = [
    "package zki",
    "",
    `tags contains "${task}" if {`,
    `\t# 聚合统计：COUNT(DISTINCT ...) / SUM / AVG，结果行列名为别名 n`,
    `\trows := sqlite.query("SELECT COUNT(DISTINCT ${c1}) AS n FROM ${table}", [])`,
    "\trows[0].n >= 1",
    "}",
    "",
  ].join("\n");

  return [
    { titleKey: "guide.sample.exists", descKey: "guide.sample.existsDesc", snippet: exists },
    { titleKey: "guide.sample.value", descKey: "guide.sample.valueDesc", snippet: value },
    {
      titleKey: "guide.sample.aggregate",
      descKey: "guide.sample.aggregateDesc",
      snippet: aggregate,
    },
  ];
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
  // 与后端 TagRuleCreateIn.Rego 的 max=65536（64KiB）保持一致
  if (src.length > 65536) {
    push("rego exceeds the maximum length of 65536 characters", 1, 1);
  }

  errors.sort((a, b) => a.line - b.line || a.column - b.column);
  return errors;
}
