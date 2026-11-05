// 全局功能开关接口类型定义。

// FeatureOut 功能开关输出项：键 + 启停状态 + 不可用原因。
export interface FeatureOut {
  key: string;
  enabled: boolean;
  // 功能不可用的原因（按请求语言返回；功能可用或后端未提供时为空串），
  // 供侧边栏不可用提示与「功能不可用」说明页展示
  reason: string;
}

// FeaturesListGetOut 功能开关列表输出：依装配顺序返回全部功能开关。
export type FeaturesListGetOut = FeatureOut[];
