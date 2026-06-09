type DebounceFunction<T extends (...args: any[]) => any> = (
  ...args: Parameters<T>
) => void;

const debounce = <T extends (...args: any[]) => any>(
  func: T,
  delay: number
): DebounceFunction<T> => {
  let timeout: NodeJS.Timeout | null = null;

  return (...args: Parameters<T>) => {
    // 清除之前的计时器
    if (timeout != null) {
      clearTimeout(timeout);
    }

    // 设置新的计时器
    timeout = setTimeout(() => {
      func.apply(null, args);
      timeout = null;
    }, delay);
  };
};
