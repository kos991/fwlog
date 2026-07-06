# 系统支持列表

本文记录 fwlog v1 系列安装包的系统支持范围、验证状态和约束。

## 支持等级

| 等级 | 含义 |
| --- | --- |
| 已验证 | 已在对应系统完成安装、启动和基础接口验证 |
| 支持 | CI 会生成安装包，按同系系统兼容性支持 |
| 计划 | 暂不发布安装包，后续按现场需求补充 |

## v1 系列支持矩阵

| 系统 / 发行版 | 架构 | 包格式 | 支持等级 | 验证状态 |
| --- | --- | --- | --- | --- |
| 银河麒麟高级服务器 V10 | x86_64 / amd64 | RPM | 已验证 | 已在 `192.168.244.244` 验证安装、ClickHouse、GeoIP、`/api/session` 和首页 |
| Debian 12 | x86_64 / amd64 | DEB | 支持 | CI 生成安装包，待现场完整验证 |
| Ubuntu Server 22.04 / 24.04 | x86_64 / amd64 | DEB | 支持 | 按 Debian 系兼容支持，待现场完整验证 |
| 通用 Linux systemd 服务器 | x86_64 / amd64 | 二进制 | 支持 | 需要手动配置 systemd、ClickHouse 和数据目录 |
| ARM64 Linux | arm64 | 暂无 | 计划 | v1 暂不发布 |
| Windows Server | x86_64 | 暂无 | 计划 | 仅用于本地开发，不作为生产部署目标 |

## 安装包内容

RPM / DEB 服务器安装包包含：

- `nat-query-service` 服务二进制
- 内置 ClickHouse 运行时
- `fwlog-clickhouse.service`
- `nat-query-service.service`
- 默认 GeoIP 数据库：`/data/index/GeoLite2-City.mmdb`
- 默认日志目录：`/data/sangfor_fw_log`
- 默认导出目录：`/data/export`

## 运行约束

- 目标服务器需要 systemd。
- 当前 v1 仅发布 x86_64 / amd64 架构。
- ClickHouse 默认监听 `127.0.0.1:9000`。
- fwlog 默认监听 `0.0.0.0:8080`。
- 生产环境建议将 ClickHouse 数据目录放到大容量数据盘。
- 银河麒麟 V10 上建议关闭 Transparent Huge Pages，或至少在自检中提示风险。

## v1.0.2 待补强项

- 安装包启动顺序需要等待 ClickHouse ready 后再启动 fwlog。
- 新装默认不应自动开始历史日志扫描，或首次进入页面时由用户确认。
- 入库进度页需要准确展示历史日期导入状态。
- 提供旧 DuckDB 文件清理提示或确认式清理脚本。
