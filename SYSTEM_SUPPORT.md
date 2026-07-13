# fwlog V2 系统支持说明

本文记录 fwlog V2 主线的系统支持范围、安装形态和验证约束。

## 支持矩阵

| 系统 | 架构 | 包格式 | 状态 | 说明 |
| --- | --- | --- | --- | --- |
| 银河麒麟高级服务器 V10 | x86_64 / amd64 | RPM | 已验证 | 适用于离线完整安装包和应用升级包。 |
| RHEL / Rocky Linux 9 | x86_64 / amd64 | RPM | CI 验证 | 用于 RPM 安装/升级事务验证。 |
| Ubuntu 24.04 | x86_64 / amd64 | DEB | CI 验证 | 用于 DEB 安装/升级事务验证。 |
| ARM64 Linux | arm64 | 暂无 | 计划 | 当前发布主线先交付 x86_64 / amd64。 |

## 安装内容

V2 使用统一服务名和安装路径：

- 应用二进制：`/opt/fwlog/fwlog`
- 应用服务：`fwlog.service`
- 私有 ClickHouse 服务：`fwlog-clickhouse.service`
- 应用版本文件：`/opt/fwlog/VERSION`
- 运行时版本文件：`/opt/fwlog/RUNTIME_VERSION`
- 数据与备份目录：`/data/fwlog`

## 发布产物

- Linux amd64 二进制：`fwlog_linux_amd64`
- 离线完整包：`fwlog-full-v{version}-amd64.tar.gz`
- RPM 完整包：`fwlog-full-v{version}.x86_64.rpm`
- DEB 完整包：`fwlog-full_{version}_amd64.deb`
- RPM 升级包：`fwlog-upgrade-v{version}.x86_64.rpm`
- DEB 升级包：`fwlog-upgrade_{version}_amd64.deb`

## 运行约束

- fwlog 默认监听 `0.0.0.0:8080`。
- ClickHouse 私有运行时默认由 `fwlog-clickhouse.service` 管理。
- 应用升级包只更新应用二进制、Web 产物、版本文件和应用服务，不覆盖私有 ClickHouse 运行时。
- 生产环境升级前会尽量备份 `app_settings` 到 `/data/fwlog/backups/`。
