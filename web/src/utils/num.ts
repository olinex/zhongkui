/**
 * 将数字列表转换为十六进制字符串数组（可选补零）
 * @param numbers 数字列表（需为非负整数）
 * @param padLength 补零后的总长度（默认2位）
 * @returns 十六进制字符串数组
 */
export function numbersToHexArray(numbers: number[], padLength: number = 2): string[] {
  return numbers.map((num) => {
    // 校验数字有效性（需为非负整数）
    if (!Number.isInteger(num) || num < 0) {
      throw new Error(`无效数字: ${num}，需为非负整数`);
    }
    // 转换为十六进制并补零
    return num.toString(16).padStart(padLength, "0");
  });
}

/**
 * 将数字列表转换为合并后的十六进制字符串（可选分隔符）
 * @param numbers 数字列表
 * @param padLength 补零后的总长度（默认2位）
 * @param separator 分隔符（默认无分隔符）
 * @returns 合并后的十六进制字符串
 */
export function numbersToHexString(
  numbers: number[],
  padLength: number = 2,
  separator: string = "",
): string {
  const hexArray = numbersToHexArray(numbers, padLength);
  return hexArray.join(separator);
}
