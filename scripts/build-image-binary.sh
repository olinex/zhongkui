#!/usr/bin/env bash
#
# build-image-binary.sh - 在镜像构建环境（Linux、无 protoc）中编译 zki 二进制
#
# 输出路径固定为 /out/zki，与 Dockerfile 中的 COPY 路径保持一致。
# 版本信息通过环境变量 VERSION / COMMIT / BUILD_TIME / BUILDER 注入，
# 由 Dockerfile 在构建阶段生成 dist/build.env 后 export，VERSION 必填。
#
# 用法：
#   scripts/build-image-binary.sh
#
# 退出码：0 成功；1 失败。
set -euo pipefail

make build-release OUT=/out/zki
