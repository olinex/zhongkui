export interface InspectorDailyStatIn {
  days: number;
}

export interface InspectorTaskListIn {
  page: number;
  pageSize: number;
  queue: string;
  state: string;
  // 任务类型筛选（可选，取值见 taskType 枚举类）
  type?: string;
  group?: string;
}

export interface InspectorStateTaskIn {
  queue: string;
  state: string;
  group?: string;
}

// 定时任务完整配置输入：cronSpec 与 interval 互斥二选一
export interface SchedulerTaskIn {
  typeName: string;
  cronSpec?: string;
  interval?: string;
  queue: string;
  timeout?: string;
  maxRetry?: number;
  enabled?: boolean;
  payload?: Record<string, unknown>;
  backlogEnabled?: boolean;
  backlogWindow?: string;
  backlogLimit?: number;
  retention?: string;
  lookback?: string;
}

export interface SchedulerTaskToggleIn {
  typeName: string;
  enabled: boolean;
}

export interface SchedulerTaskKeyIn {
  typeName: string;
}
