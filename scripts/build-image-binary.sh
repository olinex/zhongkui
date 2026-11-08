#!/usr/bin/env bash
#
# build-image-binary.sh - 在镜像构建环境（Linux、无 protoc）中编译 zki 二进制
#
# 输出路径固定为 /out/zki，与 Dockerfile 中的 COPY 路径保持一致。
# 版本信息通过环境变量 VERSION / COMMIT / BUILD_TIME / BUILDER 注入，
# 由 Dockerfile 在构建阶段生成 dist/build.env 后 export，VERSION 必填。
#
# 编译参数与 make build-release 完全一致（-trimpath、-s -w、同一套 ldflags）；
# 唯一区别是不触发 Makefile 的 BUILD_DEPS 按需重烘焙，直接复用仓库内已提交的
# RAG 快照：镜像构建环境没有增量编译优势，arm64 经 QEMU 重烘焙耗时且快照内容
# 与 CPU 架构无关；快照与代码的一致性由 CI 的 docs-check 在合入前把关。
#
# 用法：
#   scripts/build-image-binary.sh
#
# 退出码：0 成功；1 失败。
set -euo pipefail

make build-image OUT=/out/zki
