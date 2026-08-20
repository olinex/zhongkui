// Go time.Duration 字符串（如 "5s"、"1m30s"）的解析与格式化工具。
// 后端 duration 字段（serializer.Duration）在 JSON 中以该格式输出。
// 解析委托给 parse-duration 库（支持 Go 全部单位 ns/us/ms/s/m/h 与复合格式），
// 返回值为毫秒。
import parse from "parse-duration";

// durationToMillis 将 Go time.Duration 字符串解析为毫秒数；解析失败返回 NaN。
export const durationToMillis = (input: string): number => parse(input) ?? NaN;

// durationToMinutes 将 Go time.Duration 字符串解析为分钟数；解析失败返回 0。
export const durationToMinutes = (input: string): number => {
  const millis = durationToMillis(input);
  return Number.isNaN(millis) ? 0 : millis / 60000;
};

// minutesToDurationString 将分钟数格式化为 Go time.Duration 字符串（如 "5m"）。
export const minutesToDurationString = (minutes: number): string => {
  return `${minutes}m`;
};
