# 变更日志

本文件记录面向用户的显著变更。格式参考 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，
版本号遵循 [语义化版本](https://semver.org/lang/zh-CN/)。

## [Unreleased]

### 新增

- 文档体系按来源隔离：`docs/generated/` 由代码声明生成，`docs/curated/` 由人策展。
- `zki create docs --check` 校验生成物与代码声明一致。
- 文档总目录 `docs/index.md` 与导航文件 `docs/nav.json`。
- 系统总览、领域模型、术语表与故障排查指南。

### 变更

- 教程与操作指南改为策展文档，不再由包注释生成。
- 根 `README.md` 改为手写产品门面。
- 新增 VitePress 文档站点，侧边栏由 `docs/nav.json` 生成。
- 新增 `make license-check` 许可证门禁，拒绝强 copyleft、非商业与未知许可证。
- 新增 `THIRD_PARTY_LICENSES.md` 第三方依赖与许可证清单。

### 修复

- 文档导出失败时命令返回非零退出码。

[Unreleased]: https://gitee.com/olinex/zhongkui/compare/master...HEAD
