// 服务状态实例出参类型。
export interface ServiceItemOut {
  serviceName: string;
  instanceId: string;
  host: string;
  version: string;
  pid: number;
  startedAt: string;
  live: boolean;
  ready: boolean;
  started: boolean;
}

// 服务状态列表出参：从 etcd 服务目录读取的全部运行实例。
export interface ServiceListOut {
  items: ServiceItemOut[];
}