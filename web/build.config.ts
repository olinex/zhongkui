/**
 * build.config.ts
 *
 * 前端构建标识配置：页面角落展示的构建 tag / commit 的默认值与环境变量映射。
 *
 * 取值优先级：环境变量 > 本文件默认值。生产镜像与 CI 通过环境变量注入真实的
 * tag / commit（见 deploys/containers/web.Dockerfile 与 scripts/image.sh），
 * 本地开发或未注入时回落到下面的默认值，保证产物里总能看到一组可读标识。
 */

/** 构建标识：与一次构建对应的版本 tag 和 git commit */
export interface BuildInfo {
  /** 构建 tag，如 v1.2.3；未注入时为默认值 */
  tag: string;
  /** git commit（短哈希），未注入时为默认值 */
  commit: string;
}

/** 未注入环境变量时的兜底标识（本地开发 / 非发布构建） */
export const defaultBuildInfo: BuildInfo = {
  tag: "dev",
  commit: "unknown",
};

/** 注入构建标识的环境变量名（Vite 构建期读取并编译进产物） */
export const buildInfoEnv = {
  tag: "VITE_BUILD_TAG",
  commit: "VITE_BUILD_COMMIT",
} as const;
