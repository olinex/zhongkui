// token 数量单位规则：AI 场景的 token 数（上下文容量、用量）通常以 K / M 计量，
// 本模块是该口径的唯一来源，供输入组件（TokenCountField）与只读展示共用，避免各处就地换算。

// TokenUnit 一个单位档位：factor 为 10 的幂，label 为展示后缀（无单位档为空串）。
export interface TokenUnit {
  factor: number;
  label: string;
}

// TOKEN_UNITS 单位档位，由大到小排列；无单位（factor=1）恒为兜底。
export const TOKEN_UNITS: TokenUnit[] = [
  { factor: 1_000_000, label: "M" },
  { factor: 1_000, label: "K" },
  { factor: 1, label: "" },
];

// splitTokenCount 把 token 数拆成「整数 + 单位因子」：取数值不小于该档且能整除的最大单位，
// 保证整数无损往返（128000 → 128 K；4096 → 4096 无单位；0 → 0 无单位）。
export const splitTokenCount = (n: number): { value: number; factor: number } => {
  const abs = Math.abs(n);
  const hit = TOKEN_UNITS.find((u) => u.factor > 1 && abs >= u.factor && abs % u.factor === 0);
  const factor = hit?.factor ?? 1;
  return { value: n / factor, factor };
};

// joinTokenCount 由「数值 × 单位因子」还原为 token 总量。
export const joinTokenCount = (value: number, factor: number): number => value * factor;

// labelOfFactor 返回某单位因子的展示后缀（无单位档为空串）。
export const labelOfFactor = (factor: number): string =>
  TOKEN_UNITS.find((u) => u.factor === factor)?.label ?? "";

// formatTokenCount 只读展示：按能整除的最大单位加后缀，数字部分带千分位
// （128000 → "128K"，1000000 → "1M"，4096 → "4,096"）。
export const formatTokenCount = (n: number): string => {
  const { value, factor } = splitTokenCount(n);
  return `${value.toLocaleString()}${labelOfFactor(factor)}`;
};
