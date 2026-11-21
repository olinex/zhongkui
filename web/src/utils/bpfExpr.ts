/**
 * BPF 过滤表达式（tcpdump / libpcap 常用子集）的前端文法工具。
 *
 * 本文件只服务于前端体验：语法高亮、即时 lint、上下文补全、悬停说明与语法速查。
 * 后端提交时的权威校验使用 libpcap（`pcap.CompileBPFFilter`），因此这里的文法
 * 以「零假阳性」为优先：只拦截结构性错误，未识别的关键字 / 转义 / raw 字节访问
 * 语法按宽松模式放行，交由后端兜底。
 *
 * 前端文法只覆盖体验所需的常用子集（后端提交时由 libpcap 兜底，见
 * `internal/pkg/validator/bpf.go`）：
 *
 *   expr      := orExpr EOF
 *   orExpr    := andExpr { OR andExpr }
 *   andExpr   := unary { [AND] unary }        // 并列即隐式 AND
 *   unary     := NOT unary | '(' expr ')' | primitive
 *   primitive := { protoKw } { qualifier } [ value ]
 *   qualifier := [direction] typeKw | direction | 'broadcast' | 'multicast'
 *   direction := 'src' | 'dst' | ('src' ('or'|'and') 'dst')
 *   typeKw    := host | net | port | portrange | proto | protochain | less | greater
 *   protoKw   := ip | ip6 | arp | rarp | tcp | udp | icmp | icmp6 | igmp | sctp | vlan | mpls
 */

import i18n from "@/utils/i18n";

// ---- 关键字元信息 ----

/** BPF 关键字类别：逻辑词 / 协议 / 方向 / 类型限定词。 */
export type BpfKeywordKind = "logical" | "protocol" | "direction" | "qualifier";

/** 单个 BPF 关键字的补全、悬停与速查元信息。 */
export interface BpfKeywordMeta {
  /** 展示与匹配使用的规范写法。 */
  label: string;
  /** 选中补全后插入的文本（通常带尾随空格）。 */
  insertText: string;
  /** 关键字类别，决定高亮语义与补全分组。 */
  kind: BpfKeywordKind;
  /** 说明文案的 i18n 键。 */
  descriptionKey: string;
  /** 可选的用法示例。 */
  example?: string;
}

const keyword = (
  label: string,
  kind: BpfKeywordKind,
  example?: string,
): BpfKeywordMeta => ({
  label,
  insertText: `${label} `,
  kind,
  descriptionKey: `bpf.kw.${label}`,
  example,
});

/** 全部可用于补全 / 悬停 / 速查的 BPF 关键字。 */
export const BPF_KEYWORDS: BpfKeywordMeta[] = [
  // 逻辑词
  keyword("and", "logical", "tcp and udp"),
  keyword("or", "logical", "tcp or udp"),
  keyword("not", "logical", "not port 22"),
  // 协议
  keyword("ip", "protocol", "ip proto 6"),
  keyword("ip6", "protocol", "ip6 protochain 6"),
  keyword("arp", "protocol", "arp"),
  keyword("rarp", "protocol", "rarp"),
  keyword("tcp", "protocol", "tcp port 80"),
  keyword("udp", "protocol", "udp port 53"),
  keyword("icmp", "protocol", "icmp"),
  keyword("icmp6", "protocol", "icmp6"),
  keyword("igmp", "protocol", "igmp"),
  keyword("sctp", "protocol", "sctp"),
  keyword("vlan", "protocol", "vlan 100"),
  keyword("mpls", "protocol", "mpls"),
  // 方向
  keyword("src", "direction", "src host 10.0.0.1"),
  keyword("dst", "direction", "dst port 443"),
  // 类型限定词
  keyword("host", "qualifier", "host 192.168.1.1"),
  keyword("net", "qualifier", "net 10.0.0.0/8"),
  keyword("port", "qualifier", "port 80"),
  keyword("portrange", "qualifier", "portrange 8000-9000"),
  keyword("proto", "qualifier", "ip proto 6"),
  keyword("protochain", "qualifier", "ip protochain 6"),
  keyword("ether", "qualifier", "ether host aa:bb:cc:dd:ee:ff"),
  keyword("broadcast", "qualifier", "ip broadcast"),
  keyword("multicast", "qualifier", "ip multicast"),
  keyword("less", "qualifier", "less 128"),
  keyword("greater", "qualifier", "greater 512"),
];

/** 常用表达式示例，供速查面板一键插入。 */
export const BPF_EXAMPLES: { expr: string; descriptionKey: string }[] = [
  { expr: "tcp port 80", descriptionKey: "bpf.examples.http" },
  { expr: "udp port 53", descriptionKey: "bpf.examples.dns" },
  { expr: "host 192.168.1.1", descriptionKey: "bpf.examples.host" },
  { expr: "src host 10.0.0.1 and dst port 443", descriptionKey: "bpf.examples.directional" },
  { expr: "(tcp or udp) and not port 22", descriptionKey: "bpf.examples.excludeSsh" },
  { expr: "tcp portrange 8000-9000", descriptionKey: "bpf.examples.portrange" },
  { expr: "net 10.0.0.0/8", descriptionKey: "bpf.examples.cidr" },
];

const KEYWORD_BY_LABEL = new Map(BPF_KEYWORDS.map((item) => [item.label, item]));

const PROTOCOL_LABELS = [
  "ip",
  "ip6",
  "arp",
  "rarp",
  "tcp",
  "udp",
  "icmp",
  "icmp6",
  "igmp",
  "sctp",
  "vlan",
  "mpls",
];
const DIRECTION_LABELS = ["src", "dst"];
const QUALIFIER_LABELS = [
  "host",
  "net",
  "port",
  "portrange",
  "proto",
  "protochain",
  "ether",
  "broadcast",
  "multicast",
  "less",
  "greater",
];

const PROTOCOL_SET = new Set(PROTOCOL_LABELS);
const DIRECTION_SET = new Set(DIRECTION_LABELS);
const VALUE_KEYWORDS = new Set([
  "host",
  "net",
  "port",
  "portrange",
  "proto",
  "protochain",
  "less",
  "greater",
]);
const DIRECTION_TYPE_SET = new Set(["host", "net", "port", "portrange", "proto", "protochain"]);
const ETHER_TYPE_SET = new Set(["host", "src", "dst", "proto"]);

/** 按关键字类别对单词分类；未命中的单词视为普通值（可作主机名 / 端口名）。 */
export function classifyBpfWord(word: string): BpfKeywordKind | "value" {
  return KEYWORD_BY_LABEL.get(word.toLowerCase())?.kind ?? "value";
}

// ---- 词法 ----

/** 词法单元类型（运算符与括号单独成类，便于递归下降解析）。 */
export type BpfTokenType = "word" | "and" | "or" | "not" | "lparen" | "rparen" | "invalid";

/** 带绝对字符偏移的词法单元。 */
export interface BpfToken {
  type: BpfTokenType;
  text: string;
  /** 起始偏移（含）。 */
  from: number;
  /** 结束偏移（不含）。 */
  to: number;
  /** 命中关键字表时的规范小写写法。 */
  keyword?: string;
}

/** 一条前端语法诊断，偏移用于 CodeMirror lint 下划线。 */
export interface BpfDiagnostic {
  from: number;
  to: number;
  message: string;
  severity: "error" | "warning";
}

const WORD_START_RE = /[A-Za-z0-9_./:\\-]/;
const WORD_RE = /^[A-Za-z0-9_./:\\-]+/;

const tokenMessage = (key: string, params?: Record<string, unknown>): string =>
  i18n.global.t(`bpf.error.${key}`, params ?? {});

/** 把源码切成词法单元；非法字符同时产出一条诊断（解析阶段只取第一条）。 */
export function tokenizeBpf(src: string): {
  tokens: BpfToken[];
  diagnostics: BpfDiagnostic[];
} {
  const tokens: BpfToken[] = [];
  const diagnostics: BpfDiagnostic[] = [];
  let i = 0;
  while (i < src.length) {
    const ch = src[i];
    if (/\s/.test(ch)) {
      i += 1;
      continue;
    }
    if (ch === "(" || ch === ")") {
      tokens.push({
        type: ch === "(" ? "lparen" : "rparen",
        text: ch,
        from: i,
        to: i + 1,
      });
      i += 1;
      continue;
    }
    if (src.startsWith("&&", i)) {
      tokens.push({ type: "and", text: "&&", from: i, to: i + 2 });
      i += 2;
      continue;
    }
    if (src.startsWith("||", i)) {
      tokens.push({ type: "or", text: "||", from: i, to: i + 2 });
      i += 2;
      continue;
    }
    if (ch === "!") {
      tokens.push({ type: "not", text: "!", from: i, to: i + 1 });
      i += 1;
      continue;
    }
    if (WORD_START_RE.test(ch)) {
      const matched = WORD_RE.exec(src.slice(i));
      const text = matched ? matched[0] : ch;
      const lower = text.toLowerCase();
      if (lower === "and" || lower === "or" || lower === "not") {
        tokens.push({ type: lower, text, from: i, to: i + text.length });
      } else {
        tokens.push({
          type: "word",
          text,
          from: i,
          to: i + text.length,
          keyword: KEYWORD_BY_LABEL.has(lower) ? lower : undefined,
        });
      }
      i += text.length;
      continue;
    }
    diagnostics.push({
      from: i,
      to: i + 1,
      severity: "error",
      message: tokenMessage("unexpectedChar", { char: ch }),
    });
    tokens.push({ type: "invalid", text: ch, from: i, to: i + 1 });
    i += 1;
  }
  return { tokens, diagnostics };
}

// ---- 递归下降解析 ----

class BpfParseError extends Error {
  from: number;
  to: number;

  constructor(message: string, from: number, to: number) {
    super(message);
    this.name = "BpfParseError";
    this.from = from;
    this.to = to;
  }
}

class BpfParser {
  private pos = 0;

  constructor(
    private readonly tokens: BpfToken[],
    private readonly srcLength: number,
  ) {}

  /** 解析整段表达式；首个语法错误以 BpfParseError 抛出。 */
  parse(): void {
    if (this.tokens.length === 0) {
      return;
    }
    this.parseOr();
    const rest = this.peek();
    if (rest) {
      this.failToken("unexpectedToken", rest, { token: rest.text });
    }
  }

  private peek(): BpfToken | undefined {
    return this.tokens[this.pos];
  }

  private peekAt(offset: number): BpfToken | undefined {
    return this.tokens[this.pos + offset];
  }

  private next(): void {
    this.pos += 1;
  }

  private parseOr(): void {
    this.parseAnd();
    while (this.peek()?.type === "or") {
      this.next();
      this.parseAnd();
    }
  }

  private parseAnd(): void {
    this.parseUnary();
    for (;;) {
      const token = this.peek();
      if (!token || token.type === "or" || token.type === "rparen") {
        return;
      }
      if (token.type === "and") {
        this.next();
        this.parseUnary();
        continue;
      }
      // 并列即隐式 AND：`tcp port 80` 等价于 `tcp and port 80`
      this.parseUnary();
    }
  }

  private parseUnary(): void {
    const token = this.peek();
    if (!token) {
      this.failAtEnd("unexpectedEnd");
    }
    if (token.type === "not") {
      this.next();
      this.parseUnary();
      return;
    }
    if (token.type === "lparen") {
      this.next();
      const next = this.peek();
      if (next?.type === "rparen") {
        this.next();
        this.failToken("emptyGroup", next, {});
      }
      this.parseOr();
      const close = this.peek();
      if (!close || close.type !== "rparen") {
        this.failAtEnd("missingClose");
      }
      this.next();
      return;
    }
    if (token.type === "and" || token.type === "or" || token.type === "rparen") {
      this.next();
      this.failToken("unexpectedToken", token, { token: token.text });
    }
    if (token.type === "invalid") {
      this.next();
      this.failToken("unexpectedChar", token, { char: token.text });
    }
    this.parsePrimitive();
  }

  private parsePrimitive(): void {
    let consumed = false;
    let pendingValue: string | null = null;
    let lastWasProtocol = false;
    for (;;) {
      const token = this.peek();
      if (!token) {
        if (pendingValue) {
          this.failAtEnd("missingValue", { keyword: pendingValue });
        }
        if (!consumed) {
          this.failAtEnd("unexpectedEnd");
        }
        return;
      }
      if (token.type === "and" || token.type === "or" || token.type === "rparen") {
        if (pendingValue) {
          this.failToken("missingValue", token, { keyword: pendingValue });
        }
        if (!consumed) {
          this.failToken("unexpectedToken", token, { token: token.text });
        }
        return;
      }
      if (token.type === "lparen") {
        if (pendingValue) {
          this.failToken("missingValue", token, { keyword: pendingValue });
        }
        if (!consumed) {
          this.failToken("unexpectedToken", token, { token: token.text });
        }
        return;
      }
      if (token.type === "invalid") {
        this.failToken("unexpectedChar", token, { char: token.text });
      }
      if (pendingValue) {
        this.validateValue(pendingValue, token);
        this.next();
        return;
      }
      if (token.keyword === "src" || token.keyword === "dst") {
        this.next();
        consumed = true;
        lastWasProtocol = false;
        if (token.keyword === "src") {
          const op = this.peek();
          if (op && (op.type === "and" || op.type === "or")) {
            const after = this.peekAt(1);
            if (after && after.type === "word" && after.text.toLowerCase() === "dst") {
              this.next();
              this.next();
            }
          }
        }
        const next = this.peek();
        if (!next || next.type !== "word" || !DIRECTION_TYPE_SET.has(next.text.toLowerCase())) {
          if (next) {
            this.failToken("missingValue", next, { keyword: token.text });
          }
          this.failAtEnd("missingValue", { keyword: token.text });
        }
        continue;
      }
      if (token.keyword === "ether") {
        this.next();
        consumed = true;
        lastWasProtocol = false;
        const next = this.peek();
        if (!next || next.type !== "word" || !ETHER_TYPE_SET.has(next.text.toLowerCase())) {
          if (next) {
            this.failToken("missingValue", next, { keyword: token.text });
          }
          this.failAtEnd("missingValue", { keyword: token.text });
        }
        continue;
      }
      if (token.keyword === "vlan" || token.keyword === "mpls") {
        this.next();
        consumed = true;
        lastWasProtocol = true;
        const next = this.peek();
        if (next && next.type === "word" && /^\d+$/.test(next.text)) {
          this.validateValue(token.keyword, next);
          this.next();
          return;
        }
        continue;
      }
      if (token.keyword === "broadcast" || token.keyword === "multicast") {
        // 独立限定词：`ip broadcast` / `ip multicast` 等，不要求后续取值。
        this.next();
        consumed = true;
        lastWasProtocol = false;
        continue;
      }
      if (token.keyword && VALUE_KEYWORDS.has(token.keyword)) {
        this.next();
        consumed = true;
        lastWasProtocol = false;
        pendingValue = token.keyword;
        continue;
      }
      if (token.keyword && PROTOCOL_SET.has(token.keyword)) {
        this.next();
        consumed = true;
        lastWasProtocol = true;
        continue;
      }
      // 协议后直接跟裸值在 libpcap 中是非法的（如 `tcp 80`），提前给出提示；
      // 裸值作为表达式起点仍是合法的主机名简写（`tcpdump 10.0.0.1`）。
      if (lastWasProtocol) {
        this.failToken("expectedQualifier", token, { token: token.text });
      }
      this.next();
      consumed = true;
      return;
    }
  }

  private validateValue(kind: string, token: BpfToken): void {
    const text = token.text;
    if (kind === "port") {
      if (/^\d+$/.test(text) && Number(text) > 65535) {
        this.failToken("invalidPort", token, { value: text });
      }
      return;
    }
    if (kind === "portrange") {
      // libpcap 接受单端口与逆序范围（如 9000-8000），这里只校验数值上限，
      // 非数值按服务名放行，避免对 libpcap 可接受的写法误报。
      const matched = /^(\d+)(?:-(\d+))?$/.exec(text);
      if (!matched) {
        return;
      }
      const low = Number(matched[1]);
      const high = matched[2] ? Number(matched[2]) : low;
      if (low > 65535 || high > 65535) {
        this.failToken("invalidPortRange", token, { value: text });
      }
      return;
    }
    if (kind === "net") {
      const slash = text.lastIndexOf("/");
      if (slash >= 0) {
        const address = text.slice(0, slash);
        const prefix = text.slice(slash + 1);
        if (!/^\d+$/.test(prefix)) {
          this.failToken("invalidPrefix", token, { value: text });
        }
        const maxPrefix = address.includes(":") ? 128 : 32;
        if (Number(prefix) > maxPrefix) {
          this.failToken("invalidPrefix", token, { value: text });
        }
        if (LOOKS_LIKE_IPV4.test(address)) {
          this.validateIPv4(address, token);
        } else if (address.includes(":") && !/^[0-9a-fA-F:.]+$/.test(address)) {
          this.failToken("invalidIp", token, { value: address });
        }
        return;
      }
      if (LOOKS_LIKE_IPV4.test(text)) {
        this.validateIPv4(text, token);
      }
      return;
    }
    if (kind === "host") {
      if (LOOKS_LIKE_IPV4.test(text)) {
        this.validateIPv4(text, token);
      }
      return;
    }
    if (kind === "less" || kind === "greater") {
      if (!/^\d+$/.test(text)) {
        this.failToken("invalidNumber", token, { value: text });
      }
      return;
    }
    if (kind === "vlan") {
      if (!/^\d+$/.test(text) || Number(text) > 4095) {
        this.failToken("invalidNumber", token, { value: text });
      }
    }
  }

  private validateIPv4(address: string, token: BpfToken): void {
    const parts = address.split(".");
    if (parts.length !== 4 || parts.some((part) => Number(part) > 255)) {
      this.failToken("invalidIp", token, { value: address });
    }
  }

  private failToken(key: string, token: BpfToken, params: Record<string, unknown>): never {
    throw new BpfParseError(
      tokenMessage(key, params),
      token.from,
      Math.max(token.to, token.from + 1),
    );
  }

  private failAtEnd(key: string, params?: Record<string, unknown>): never {
    const from = Math.max(0, this.srcLength - 1);
    throw new BpfParseError(tokenMessage(key, params), from, this.srcLength);
  }
}

const LOOKS_LIKE_IPV4 = /^\d+\.\d+\.\d+\.\d+$/;

/**
 * raw BPF 字节访问语法（如 `tcp[13] & 2 != 0`）的宽松校验：
 * 只检查括号配对与末尾悬挂运算符，避免对合法 raw 表达式误报。
 * raw 语法允许任意算术 / 比较运算符，因此这里不参考普通词法诊断。
 */
function parseRawLenient(src: string): BpfDiagnostic[] {
  const stack: { ch: string; at: number }[] = [];
  for (let i = 0; i < src.length; i += 1) {
    const ch = src[i];
    if (ch === "(" || ch === "[") {
      stack.push({ ch, at: i });
      continue;
    }
    if (ch === ")" || ch === "]") {
      const open = stack.pop();
      if (!open || (open.ch === "(" ? ch !== ")" : ch !== "]")) {
        return [
          {
            from: i,
            to: i + 1,
            severity: "error",
            message: tokenMessage("unexpectedClose", { char: ch }),
          },
        ];
      }
    }
  }
  if (stack.length > 0) {
    const open = stack[stack.length - 1];
    return [
      {
        from: open.at,
        to: Math.min(src.length, open.at + 1),
        severity: "error",
        message: tokenMessage("missingClose"),
      },
    ];
  }
  if (
    /(?:&&|\|\||\band\b|\bor\b|\bnot\b|!|\+|-|\*|\/|%|&|\||\^|<<|>>|<=|>=|==|!=|=|<|>)$/i.test(
      src.trimEnd(),
    )
  ) {
    const from = Math.max(0, src.length - 1);
    return [
      {
        from,
        to: src.length,
        severity: "error",
        message: tokenMessage("unexpectedEnd"),
      },
    ];
  }
  return [];
}

/**
 * 解析 BPF 表达式并返回诊断（最多一条，首个错误即停止）。
 * 空串 / 纯空白返回空数组（必填与长度由表单规则负责）。
 */
export function parseBpf(src: string): BpfDiagnostic[] {
  if (!src.trim()) {
    return [];
  }
  const { tokens, diagnostics } = tokenizeBpf(src);
  if (src.includes("[")) {
    return parseRawLenient(src);
  }
  if (diagnostics.length > 0) {
    return [diagnostics[0]];
  }
  try {
    new BpfParser(tokens, src.length).parse();
    return [];
  } catch (error) {
    if (error instanceof BpfParseError) {
      return [
        {
          from: error.from,
          to: error.to,
          severity: "error",
          message: error.message,
        },
      ];
    }
    throw error;
  }
}

/** 定位覆盖给定偏移的词法单元（用于悬停说明）。 */
export function bpfTokenAt(src: string, pos: number): BpfToken | null {
  const { tokens } = tokenizeBpf(src);
  for (const token of tokens) {
    if (pos >= token.from && pos <= token.to) {
      return token;
    }
  }
  return null;
}

/** 按单词（含 `&&` / `||` / `!` 别名）查询关键字元信息。 */
export function bpfKeywordDoc(word: string): BpfKeywordMeta | null {
  const lower = word.toLowerCase();
  if (lower === "&&") {
    return KEYWORD_BY_LABEL.get("and") ?? null;
  }
  if (lower === "||") {
    return KEYWORD_BY_LABEL.get("or") ?? null;
  }
  if (lower === "!") {
    return KEYWORD_BY_LABEL.get("not") ?? null;
  }
  return KEYWORD_BY_LABEL.get(lower) ?? null;
}

// ---- 补全 ----

/** 一条补全候选，由组件转换为 CodeMirror Completion。 */
export interface BpfCompletionItem {
  label: string;
  insertText: string;
  type: "keyword" | "value" | "operator" | "bracket";
  detail?: string;
}

/** 补全上下文：位置、前缀与候选列表。 */
export interface BpfCompletionContext {
  kind:
    | "start"
    | "afterDirection"
    | "afterEther"
    | "value"
    | "afterProtocol"
    | "operator"
    | "none";
  from: number;
  prefix: string;
  items: BpfCompletionItem[];
}

const COMMON_PORTS: [string, string][] = [
  ["http", "80"],
  ["https", "443"],
  ["ssh", "22"],
  ["dns", "53"],
  ["ftp", "21"],
  ["smtp", "25"],
  ["ntp", "123"],
  ["mysql", "3306"],
  ["postgresql", "5432"],
  ["redis", "6379"],
];

const COMMON_NETS = ["0.0.0.0/0", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"];

const toKeywordItem = (meta: BpfKeywordMeta): BpfCompletionItem => ({
  label: meta.label,
  insertText: meta.insertText,
  type: meta.kind === "logical" ? "operator" : "keyword",
  detail: i18n.global.t(meta.descriptionKey),
});

const pickKeywords = (labels: string[]): BpfCompletionItem[] =>
  labels
    .map((label) => KEYWORD_BY_LABEL.get(label))
    .filter((meta): meta is BpfKeywordMeta => Boolean(meta))
    .map(toKeywordItem);

const bracketItem: BpfCompletionItem = { label: "(", insertText: "(", type: "bracket" };
const rightParenItem: BpfCompletionItem = { label: ")", insertText: ")", type: "bracket" };
const aliasItems: BpfCompletionItem[] = [
  { label: "&&", insertText: "&& ", type: "operator", detail: i18n.global.t("bpf.kw.and") },
  { label: "||", insertText: "|| ", type: "operator", detail: i18n.global.t("bpf.kw.or") },
  { label: "!", insertText: "! ", type: "operator", detail: i18n.global.t("bpf.kw.not") },
];

/**
 * 计算光标处的补全上下文。`from` 指向当前前缀起点；`items` 已按前缀过滤。
 * 调用方可在 `kind === "operator"` 且前缀为空时抑制自动弹出。
 */
export function bpfCompletionContext(src: string, pos: number): BpfCompletionContext {
  const before = src.slice(0, pos);
  const prefixMatch = /[A-Za-z0-9_./:\\-]*$/.exec(before);
  const prefix = prefixMatch ? prefixMatch[0] : "";
  const from = pos - prefix.length;
  const { tokens } = tokenizeBpf(before.slice(0, from));
  const last = tokens[tokens.length - 1];
  let kind: BpfCompletionContext["kind"] = "start";
  let valueKeyword = "";

  if (last) {
    if (
      last.type === "lparen" ||
      last.type === "and" ||
      last.type === "or" ||
      last.type === "not"
    ) {
      kind = "start";
    } else if (last.type === "rparen") {
      kind = "operator";
    } else if (last.type === "invalid") {
      kind = "none";
    } else {
      const word = last.text.toLowerCase();
      if (DIRECTION_SET.has(word)) {
        kind = "afterDirection";
      } else if (word === "ether") {
        kind = "afterEther";
      } else if (VALUE_KEYWORDS.has(word)) {
        kind = "value";
        valueKeyword = word;
      } else if (PROTOCOL_SET.has(word)) {
        kind = "afterProtocol";
      } else {
        kind = "operator";
      }
    }
  }

  let items: BpfCompletionItem[] = [];
  switch (kind) {
    case "start":
      items = [
        ...pickKeywords(PROTOCOL_LABELS),
        ...pickKeywords(DIRECTION_LABELS),
        ...pickKeywords(QUALIFIER_LABELS),
        ...pickKeywords(["not"]),
        bracketItem,
      ];
      break;
    case "afterDirection":
      items = pickKeywords(["host", "net", "port", "portrange", "proto", "protochain"]);
      break;
    case "afterEther":
      items = pickKeywords(["host", "src", "dst", "proto"]);
      break;
    case "afterProtocol":
      items = [...pickKeywords(QUALIFIER_LABELS), ...pickKeywords(["and", "or"])];
      break;
    case "value":
      items = valueCompletions(valueKeyword);
      break;
    case "operator":
      items = [...pickKeywords(["and", "or"]), ...aliasItems, rightParenItem];
      break;
    default:
      items = [];
  }

  const lowerPrefix = prefix.toLowerCase();
  const seen = new Set<string>();
  const filtered = items.filter((item) => {
    if (prefix && !item.label.toLowerCase().startsWith(lowerPrefix)) {
      return false;
    }
    const key = `${item.label}\u0000${item.insertText}`;
    if (seen.has(key)) {
      return false;
    }
    seen.add(key);
    return true;
  });
  return { kind, from, prefix, items: filtered };
}

function valueCompletions(keyword: string): BpfCompletionItem[] {
  if (keyword === "port") {
    return COMMON_PORTS.map(([label, port]) => ({
      label,
      insertText: label,
      type: "value" as const,
      detail: port,
    }));
  }
  if (keyword === "net") {
    return COMMON_NETS.map((cidr) => ({
      label: cidr,
      insertText: cidr,
      type: "value" as const,
    }));
  }
  if (keyword === "proto" || keyword === "protochain") {
    // libpcap 的 proto 取值需要转义关键字（如 `ip proto \tcp`）：
    // 展示仍用协议名便于前缀匹配，插入时补上反斜杠。
    return PROTOCOL_LABELS.map((label) => ({
      label,
      insertText: `\\${label}`,
      type: "value" as const,
      detail: i18n.global.t("bpf.kw.proto"),
    }));
  }
  return [];
}
