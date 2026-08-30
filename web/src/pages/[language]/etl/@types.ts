export type TaskState =
  "pending" | "active" | "aggregating" | "scheduled" | "retry" | "archived" | "completed";

export interface ProcessingTask {
  id: string;
  queue: string;
  type: string;
  payload: string;
  started: string;
  deadline: string;
}

export interface InspectorServerListItem {
  id: string;
  host: string;
  pid: number;
  totalConcurrency: number;
  queueWeights: Record<string, number>;
  started: string;
  status: string;
  processingTasks: ProcessingTask[];
  show: boolean;
}

export interface InspectorSchedulerListItem {
  typeName: string;
  cronSpec: string;
  interval: string;
  queue: string;
  timeout: string;
  maxRetry: number;
  enabled: boolean;
  payload: Record<string, unknown>;
  backlogEnabled: boolean;
  backlogWindow: string;
  backlogLimit: number;
}

export interface InspectorQueueItem {
  memoryUsage: number;
  latency: string;
  size: number;
  groups: {
    group: string;
    size: number;
  };
  pending: number;
  active: number;
  scheduled: number;
  retry: number;
  archived: number;
  completed: number;
  aggregating: number;
  processed: number;
  failed: number;
  processedTotal: number;
  failedTotal: number;
  paused: boolean;
  retrievalTime: string;
}

export interface InspectorDailyStatItem {
  date: string;
  processed: number;
  failed: number;
}

export interface InspectorQueueListItem {
  name: string;
  memoryUsage: number;
  latency: string;
  size: number;
  groups: {
    group: string;
    size: number;
  };
  pending: number;
  active: number;
  scheduled: number;
  retry: number;
  archived: number;
  completed: number;
  aggregating: number;
  processed: number;
  failed: number;
  processedTotal: number;
  failedTotal: number;
  running: boolean;
  retrievalTime: string;
}

export interface InspectorTaskListItem {
  id: string;
  queue: string;
  type: string;
  payload: string;
  result: string;
  state: string;
  maxRetry: number;
  retried: number;
  lastErr: string;
  lastFailedAt: string | null;
  nextProcessAt: string | null;
  completedAt: string | null;
  deadline: string | null;
  timeout: string;
  retention: string;
  group: string;
  isOrphaned: boolean;
}

export type InspectorServerListOut = InspectorServerListItem[];

export type InspectorSchedulerListOut = InspectorSchedulerListItem[];

export type InspectorQueueListOut = Record<string, InspectorQueueItem>;

export type InspectorDailyStatListOut = Record<string, InspectorDailyStatItem[]>;

export type InspectorTaskListOut = InspectorTaskListItem[];
