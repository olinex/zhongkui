const chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789";

// INVALID_PLACEHOLDER 无效 / 不适用取值的统一占位符（em dash）。
// 字段在当前上下文无意义时（如非向量档位的「向量维度」、未填写的可选值）用它与正常取值区分；
// 全站统一为 em dash「—」，避免各地混用 0 / 空串 / 其它符号。约定见 AGENTS.md「无效值占位符」。
export const INVALID_PLACEHOLDER = "—";

export function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

export function arrayToCsv(data: string[][]): string {
  return data
    .map((row) =>
      row
        .map((field) => {
          // 处理包含 [逗号, 换行符, 双引号] 的字段
          if (/[,\n"]/.test(field)) {
            return `"${field.replace(/"/g, '""')}"`; // 转义双引号
          }
          return field;
        })
        .join(","),
    )
    .join("\r\n"); // 使用 CRLF 换行符（CSV 标准）
}

export function generateRandomString(length: number): string {
  let result = "";
  // 循环生成指定长度的字符串
  for (let i = 0; i < length; i++) {
    // 从字符集中随机选取一个字符
    const randomIndex = Math.floor(Math.random() * chars.length);
    result += chars[randomIndex];
  }

  return result;
}
