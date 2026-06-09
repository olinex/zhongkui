export interface InspectorDailyStatIn {
  days: number;
}

export interface InspectorTaskListIn {
  page: number;
  pageSize: number;
  queue: string;
  state: string;
  group?: string;
}

export interface InspectorStateTaskIn {
  queue: string;
  state: string;
  group?: string;
}
