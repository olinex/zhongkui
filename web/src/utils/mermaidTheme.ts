/**
 * src/utils/mermaidTheme.ts
 *
 * 助手 Markdown 内 mermaid 图表的主题合成与强调节点配色，参照 Trae 导出图示的设计语言
 * （long_term_memory_write_and_recall.html）：
 *   - 画布与节点同底、1px 细描边、8px 圆角，节点靠描边而非填色区分；
 *   - 标签 500 字重、13px；连线 1.5px 平滑曲线（basis），箭头小实心；
 *   - 强调节点由模型用 classDef（zkKey / zkWarning / zkError）标注，统一渲染成主题强调色
 *     （primary / warning / error），其出边与箭头自动跟随该节点描边色。
 * 连线走向保持 mermaid 原生渲染，不做端点吸附或路径改写。
 * 颜色取 Vuetify 主题的 surface / border / on-surface / on-surface-variant 与强调色，
 * 保证与抽屉、代码块同处一个色系。
 *
 * 本文件是 mermaid 图表配色的唯一来源：组件内只调用 mermaidConfigFor / mermaidCanvasBackground /
 * decorateMermaidSvg。
 */

// 第三方
import type { MermaidConfig } from "mermaid";

// 与 tokens.scss 的 $zk-font-family 保持一致（mermaid 需字符串形式的字体栈）
const FONT_FAMILY =
  'Roboto, "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", "Noto Sans SC", system-ui, -apple-system, "Segoe UI", sans-serif';

// AccentName 是强调色的档位：与主题语义色一一对应（重点 / 风险 / 异常）。
type AccentName = "primary" | "warning" | "error";

interface MermaidAccent {
  /** 强调描边（取主题 primary / warning / error） */
  stroke: string;
  /** 强调节点底色：强调色与画布底色混合后的浅色块，静态色值（见 MermaidPalette 的说明） */
  fill: string;
}

interface MermaidPalette {
  /** 画布与节点底色（取主题 surface） */
  surface: string;
  /** 节点描边（取主题 border） */
  border: string;
  /** 节点文字（取主题 on-surface） */
  text: string;
  /** 连线与箭头（取主题 on-surface-variant） */
  line: string;
  /** 子图底色（取主题 surface-variant） */
  cluster: string;
  /** 强调色档位（取主题 primary / warning / error 及各自 12%/18% 透明度的浅底） */
  accents: Record<AccentName, MermaidAccent>;
  darkMode: boolean;
}

// 色值刻意写成静态 hex 而非 `--v-theme-*` 变量：mermaid 的配色在渲染时读入 themeVariables，
// 而「复制为图片」会把 SVG 序列化后经 <img> 栅格化，CSS 变量与外部样式在那一步都不再生效。
// surface / border / text / line / cluster 与 plugins/vuetify.ts 的主题色取值一致；
// accents.stroke 取同文件的 primary / warning / error，浅底色是强调色按 12%（亮）/
// 18%（暗）叠加在 surface 上的结果。
const LIGHT_PALETTE: MermaidPalette = {
  surface: "#ffffff",
  border: "#d5dce7",
  text: "#1f2937",
  line: "#4b5563",
  cluster: "#e2e7ef",
  accents: {
    primary: { stroke: "#2563eb", fill: "#e5ecfd" },
    warning: { stroke: "#d97706", fill: "#faefe1" },
    error: { stroke: "#dc2626", fill: "#fbe5e5" },
  },
  darkMode: false,
};

const DARK_PALETTE: MermaidPalette = {
  surface: "#151a21",
  border: "#262e3a",
  text: "#e6e8ec",
  line: "#9ca3af",
  cluster: "#1d242e",
  accents: {
    primary: { stroke: "#3b82f6", fill: "#1c2d47" },
    warning: { stroke: "#fbbf24", fill: "#3e3822" },
    error: { stroke: "#ef4444", fill: "#3c2227" },
  },
  darkMode: true,
};

// EMPHASIS_CLASS 把 mermaid 的 classDef 类名映射到强调色档位：约定类名（zkKey / zkWarning /
// zkError）由 mermaid_check 的工具说明下发给模型，其余常见同义词一并识别，未登记的自定义类按
// 「重点环节」处理——classDef 在助手的图里只用于挑重心，不该停留在模型随手写的色值上。
const EMPHASIS_CLASS: Record<string, AccentName> = {
  zkkey: "primary",
  key: "primary",
  primary: "primary",
  highlight: "primary",
  important: "primary",
  focus: "primary",
  accent: "primary",
  zkwarning: "warning",
  warning: "warning",
  warn: "warning",
  risk: "warning",
  zkerror: "error",
  error: "error",
  danger: "error",
  critical: "error",
  fail: "error",
  failure: "error",
};

// FLOWCHART_LAYOUT 让节点更接近导出图示的“宽卡片”：圆角由样式层补齐（mermaid 无圆角
// 变量），此处只放大内边距与层间距，并统一使用平滑曲线连线。
const FLOWCHART_LAYOUT = {
  curve: "basis",
  nodePadding: 15,
  nodeSpacing: 30,
  rankSpacing: 48,
  diagramPadding: 12,
} as const;

/**
 * mermaidCanvasBackground 返回图表画布底色：与节点填充同源（palette.surface），供「复制为
 * 图片」时填充 PNG 背景，避免透明底在浅色/深色客户端里不可读。
 */
export function mermaidCanvasBackground(dark: boolean): string {
  return (dark ? DARK_PALETTE : LIGHT_PALETTE).surface;
}

/** mermaidConfigFor 把 md-editor 传入的基础配置改写为 Trae 图示风格；明暗由基础主题判定。 */
export function mermaidConfigFor(base: MermaidConfig): MermaidConfig {
  const palette = base.theme === "dark" ? DARK_PALETTE : LIGHT_PALETTE;
  return {
    ...base,
    theme: "base",
    darkMode: palette.darkMode,
    // 标签一律用 SVG <text> 而非 foreignObject：SVG 经 <img> 栅格化到 canvas 时浏览器不会渲染
    // foreignObject 内容，保持 HTML 标签会让「复制为图片」丢失全部文字。该键只在顶层生效
    // （放到 flowchart 子配置里不会改变渲染路径）。
    htmlLabels: false,
    themeVariables: {
      fontFamily: FONT_FAMILY,
      fontSize: "13px",
      // 连线线宽走主题变量：mermaid 给 `.edge-thickness-normal` 生成的是 ID 作用域规则，
      // 外部 CSS 压不过，只能从 themeVariables 注入（导出图示为 1.5px）
      strokeWidth: 1.5,
      // 导出图示没有投影：mermaid 浅色主题默认给节点加 drop-shadow，此处显式关掉
      dropShadow: "none",
      background: palette.surface,
      primaryColor: palette.surface,
      primaryTextColor: palette.text,
      primaryBorderColor: palette.border,
      lineColor: palette.line,
      secondaryColor: palette.cluster,
      tertiaryColor: palette.cluster,
      mainBkg: palette.surface,
      nodeBorder: palette.border,
      clusterBkg: palette.cluster,
      clusterBorder: palette.border,
      edgeLabelBackground: palette.surface,
      textColor: palette.text,
      titleColor: palette.text,
      nodeTextColor: palette.text,
    },
    flowchart: { ...base.flowchart, ...FLOWCHART_LAYOUT },
  };
}

// NODE_ID 捕获节点组 id 中的节点名，形如 `<diagramId>-flowchart-A-0`
const NODE_ID = /-flowchart-(.+?)-\d+$/;
// EDGE_SOURCE 从边 id 中取出 `源_目标_序号` 段，形如 `<diagramId>-L_A_C_0`
const EDGE_SOURCE = /-L_(.+)$/;
// DECORATED 标记已处理过的 SVG，避免观察器重复触发时重复克隆箭头 marker
const DECORATED = "zkMermaidDecorated";
// BASE_CLASS_NAMES 是 mermaid 给每个节点都会加的类，不算强调标记
const BASE_CLASS_NAMES = new Set(["node", "default"]);

/**
 * decorateMermaidSvg 把带 classDef 的强调节点渲染成主题强调色，并让其出边与箭头跟随节点描边色。
 * 连线走向沿用 mermaid 原生渲染结果，不做端点吸附或路径改写。
 * 明暗由调用方传入（与 mermaidConfigFor 的判定同源），图切主题时会重挂载并重新渲染，故每个
 * SVG 只处理一次。
 */
export function decorateMermaidSvg(root: ParentNode, dark: boolean): void {
  const palette = dark ? DARK_PALETTE : LIGHT_PALETTE;
  root.querySelectorAll<SVGSVGElement>("svg").forEach((svg) => {
    if (svg.dataset[DECORATED] === "true") {
      return;
    }
    svg.dataset[DECORATED] = "true";

    const accents = emphasizeNodes(svg, palette);
    if (accents.size > 0) {
      colorLinksBySource(svg, accents);
    }
  });
}

/** emphasisAccentOf 取节点的强调档位：没有额外类即普通节点；未登记的自定义类按重点处理。 */
const emphasisAccentOf = (node: Element): AccentName | undefined => {
  const emphasis = Array.from(node.classList).find((name) => !BASE_CLASS_NAMES.has(name));
  if (!emphasis) {
    return undefined;
  }
  return EMPHASIS_CLASS[emphasis.toLowerCase()] ?? "primary";
};

/**
 * emphasizeNodes 把带 classDef 的节点改写成主题强调色，返回「节点名 → 强调色」供出边取色。
 *
 * 颜色必须经内联样式并带 !important 写入：mermaid 为 classDef 生成的是 `.类名 rect{…!important}`
 * 规则，普通内联样式压不过它，只有内联的 !important 才能覆盖；内联样式同时会被「复制为图片」的
 * SVG 序列化带走，导出图观感一致。
 */
const emphasizeNodes = (
  svg: SVGSVGElement,
  palette: MermaidPalette,
): Map<string, MermaidAccent> => {
  const accents = new Map<string, MermaidAccent>();
  svg.querySelectorAll("g.node").forEach((node) => {
    const key = NODE_ID.exec(node.id)?.[1];
    const accentName = emphasisAccentOf(node);
    if (!key || !accentName) {
      return;
    }
    const accent = palette.accents[accentName];
    accents.set(key, accent);
    const shape = node.querySelector<SVGElement>("rect, polygon, circle, ellipse");
    if (!shape) {
      return;
    }
    shape.style.setProperty("fill", accent.fill, "important");
    shape.style.setProperty("stroke", accent.stroke, "important");
    shape.style.setProperty("stroke-width", "2px", "important");
  });
  return accents;
};

/**
 * colorLinksBySource 让连线颜色跟随起始节点：从带强调色的节点出发的边，取其强调色作为描边与
 * 箭头颜色；普通节点保持主题线色（普通节点填充即画布底色，跟随填充色会让连线不可见）。
 *
 * mermaid 原生不支持按源节点取色，故在 SVG 渲染完成后按边 id 解析源节点做一次后处理。
 */
const colorLinksBySource = (svg: SVGSVGElement, accents: Map<string, MermaidAccent>): void => {
  const markerByColor = new Map<string, string>();
  svg.querySelectorAll<SVGPathElement>("path.flowchart-link").forEach((path) => {
    const source = EDGE_SOURCE.exec(path.id)?.[1] ?? "";
    // 节点名可能含下划线，按“最长匹配”解析源节点，避免 A 与 A_B 混淆
    let sourceKey = "";
    accents.forEach((_accent, key) => {
      if (source.startsWith(`${key}_`) && key.length > sourceKey.length) {
        sourceKey = key;
      }
    });
    if (!sourceKey) {
      return;
    }
    const color = accents.get(sourceKey)?.stroke;
    if (!color) {
      return;
    }
    path.style.stroke = color;

    const markerId = (path.getAttribute("marker-end") ?? "").replace(/^url\(#(.+)\)$/, "$1");
    const marker = markerId ? svg.querySelector(`marker[id="${markerId}"]`) : null;
    if (!marker) {
      return;
    }
    // 箭头 marker 被多条边共用：按颜色克隆一份并复用，避免把所有箭头一起染色
    let markerIdForColor = markerByColor.get(color);
    if (!markerIdForColor) {
      const clone = marker.cloneNode(true) as SVGMarkerElement;
      markerIdForColor = `${markerId}_zk${markerByColor.size}`;
      clone.id = markerIdForColor;
      clone.querySelectorAll<SVGElement>("path, polygon").forEach((shape) => {
        shape.style.fill = color;
      });
      marker.parentNode?.appendChild(clone);
      markerByColor.set(color, markerIdForColor);
    }
    path.setAttribute("marker-end", `url(#${markerIdForColor})`);
  });
};
