/**
 * Rego helpers shared by the tag rule admin pages and the RegoEditor
 * component: tag/field literal extraction for heuristic expansion, a field
 * suggestion dictionary for autocomplete, Rego skeleton generation and a
 * lightweight local preflight checker.
 */

export interface RegoValidationError {
  line: number;
  column: number;
  message: string;
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

export interface InputFieldSuggestion {
  /** input field name, e.g. "method" */
  field: string;
  /** example snippet inserted into the editor */
  snippet: string;
  /** brief description shown in the autocomplete popup */
  description: string;
}

/**
 * Dictionary of commonly used fields of the HTTP transport input document
 * (`zhongkui.com/zhongkui/internal/models/network.HttpTransport` JSON).
 */
export const INPUT_FIELD_SUGGESTIONS: InputFieldSuggestion[] = [
  {
    field: "method",
    snippet: `input.method == "GET"`,
    description: "HTTP request method",
  },
  {
    field: "scheme",
    snippet: `input.scheme == "http"`,
    description: "URL scheme",
  },
  {
    field: "host",
    snippet: `input.host == "example.com"`,
    description: "request host",
  },
  {
    field: "http_version",
    snippet: `input.http_version == "HTTP/1.1"`,
    description: "HTTP protocol version",
  },
  {
    field: "client_ip",
    snippet: `input.client_ip == "10.0.0.1"`,
    description: "client IP address",
  },
  {
    field: "server_ip",
    snippet: `input.server_ip == "93.184.216.34"`,
    description: "server IP address",
  },
  {
    field: "url",
    snippet: `input.url == "http://example.com/path"`,
    description: "full request URL",
  },
  {
    field: "path",
    snippet: `input.path == "/api/login"`,
    description: "request path",
  },
  {
    field: "file_ext",
    snippet: `input.file_ext == "html"`,
    description: "path file extension",
  },
  {
    field: "req_hdr_user_agent",
    snippet: `contains(input.req_hdr_user_agent, "curl")`,
    description: "user-agent request header",
  },
];

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
