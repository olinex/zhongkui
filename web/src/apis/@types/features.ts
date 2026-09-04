// 全局功能开关接口类型定义。
// Feature-flag interface type definitions.

// FeatureOut 功能开关输出项：键 + 启停状态。
export interface FeatureOut {
  key: string;
  enabled: boolean;
}

// FeaturesListGetOut 功能开关列表输出：依装配顺序返回全部功能开关。
export type FeaturesListGetOut = FeatureOut[];