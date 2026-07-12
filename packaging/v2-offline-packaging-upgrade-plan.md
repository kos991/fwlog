# V2 离线安装包与升级包计划

## 目标

V2 版本采用两类交付物：

- 全量离线包：用于首次离线部署，包含应用、前端、GeoIP、ClickHouse、systemd 和安装脚本。
- 升级包：用于后续小版本升级，只包含应用、前端、GeoIP 和应用服务配置，不包含 ClickHouse。

核心原则：

- 大多数现场是离线环境，首次部署必须一包带齐。
- 后续升级不能反复携带 ClickHouse，避免每次升级都是 150 MB 到 220 MB 的大包。
- GeoIP 不单独拆包，跟应用升级包一起走。
- ClickHouse 只在全量包里出现；除非 runtime 必须升级，否则普通升级不触碰 ClickHouse 和历史数据。

## 交付物

### 全量离线包

文件名：

```text
fwlog-full-v{APP_VERSION}-amd64.tar.gz
```

目录结构：

```text
fwlog-full-v1.0.5-amd64/
  rpm/
    fwlog-full-v1.0.5.x86_64.rpm
  deb/
    fwlog-full_1.0.5_amd64.deb
  install.sh
  README.md
  checksums.txt
```

用途：

- 新服务器首次安装。
- 离线环境完整部署。
- ClickHouse/runtime 版本必须升级时的人工全量升级。

全量 RPM/DEB 内容：

```text
/opt/nat-query/nat-query-service
/opt/nat-query/web/
/opt/nat-query/VERSION
/opt/nat-query/RUNTIME_VERSION
/opt/nat-query/clickhouse/bin/clickhouse
/opt/nat-query/clickhouse/config.xml
/opt/nat-query/clickhouse/users.xml
/data/index/GeoLite2-City.mmdb
/etc/systemd/system/nat-query-service.service
/etc/systemd/system/fwlog-clickhouse.service
```

### 升级包

文件名：

```text
fwlog-upgrade-v1.0.5.x86_64.rpm
fwlog-upgrade_1.0.5_amd64.deb
```

用途：

- v1.0.5 -> v1.0.6 这类应用小版本升级。
- UI、后端逻辑、前端静态资源、GeoIP 数据更新。

升级 RPM/DEB 内容：

```text
/opt/nat-query/nat-query-service
/opt/nat-query/web/
/opt/nat-query/VERSION
/data/index/GeoLite2-City.mmdb
/etc/systemd/system/nat-query-service.service
```

升级包不得包含：

```text
/opt/nat-query/clickhouse/
/data/clickhouse/
/etc/systemd/system/fwlog-clickhouse.service
```

升级包不得执行：

- 初始化 ClickHouse 数据目录。
- 覆盖 ClickHouse 配置。
- 删除或迁移历史日志数据。
- 重装 ClickHouse systemd 服务。

## 版本文件

安装后保留两个版本文件：

```text
/opt/nat-query/VERSION
/opt/nat-query/RUNTIME_VERSION
```

示例：

```text
VERSION=v1.0.5
RUNTIME_VERSION=clickhouse-25.8.28.1
```

含义：

- `VERSION`：fwlog 应用版本，普通升级只更新它。
- `RUNTIME_VERSION`：全量安装时携带的 ClickHouse/runtime 版本，普通升级不更新它。

## Release Manifest

每个版本发布一个 `latest.json`，供升级页检查更新。

示例：

```json
{
  "version": "v1.0.6",
  "requiresRuntime": "clickhouse-25.8.28.1",
  "releaseDate": "2026-07-06",
  "packages": {
    "rpm": {
      "name": "fwlog-upgrade-v1.0.6.x86_64.rpm",
      "sha256": "<sha256>",
      "size": 18874368
    },
    "deb": {
      "name": "fwlog-upgrade_1.0.6_amd64.deb",
      "sha256": "<sha256>",
      "size": 18874368
    },
    "full": {
      "name": "fwlog-full-v1.0.6-amd64.tar.gz",
      "sha256": "<sha256>",
      "size": 230000000
    }
  },
  "notes": [
    "修复查询时间范围过大时的页面显示问题",
    "优化升级页状态展示"
  ]
}
```

V2 第一阶段可以先只实现 `version`、`requiresRuntime`、`packages`、`notes`，其余字段用于后续完善。

## 升级判断

后端本地版本接口返回：

```json
{
  "appVersion": "v1.0.5",
  "runtimeVersion": "clickhouse-25.8.28.1",
  "packageType": "full"
}
```

判断规则：

| 条件 | 结果 |
| --- | --- |
| 本机 `appVersion` 等于远端 `version` | 已是最新 |
| 本机 `appVersion` 小于远端 `version` 且 `runtimeVersion` 满足 `requiresRuntime` | 可使用升级包 |
| 本机 `appVersion` 小于远端 `version` 但 `runtimeVersion` 不满足 `requiresRuntime` | 禁止自动升级，提示使用全量离线包 |
| 无法访问远端 manifest | 显示检查失败，允许手动上传升级包 |

普通升级只比较应用版本；runtime 只作为兼容性门槛。

## 页面逻辑

升级页展示：

```text
当前应用版本：v1.0.5
运行环境版本：ClickHouse 25.8.28.1
最新应用版本：v1.0.6
升级方式：应用升级包
```

页面操作：

- 检查更新：请求远端 `latest.json` 并和本地 `/api/version` 对比。
- 下载升级包：只下载 `fwlog-upgrade` rpm/deb。
- 上传升级包：支持离线手工上传 rpm/deb。
- 开始升级：执行后端升级接口，安装升级包并重启应用服务。

页面状态：

- 已是最新。
- 发现新版本，可升级。
- runtime 不满足，需要全量离线包。
- 检查失败，可手动上传升级包。
- 升级中。
- 升级成功，提示刷新页面。
- 升级失败，展示错误日志摘要。

## 后端逻辑

V2 需要补齐：

- `/api/version`：读取 `VERSION` 和 `RUNTIME_VERSION`。
- `/api/update/check`：拉取或读取 manifest，输出页面可直接消费的状态。
- `/api/update/upload`：接收离线上传的升级 rpm/deb。
- `/api/update/install`：安装升级包，执行校验、备份、安装、重启。
- `/api/update/status`：返回升级任务状态和最近日志。

安装升级包前必须校验：

- 包类型是 `fwlog-upgrade`，不是 `fwlog-full`。
- 包架构是 `amd64/x86_64`。
- 包版本高于当前应用版本。
- 包内不包含 ClickHouse 路径。
- sha256 与 manifest 或上传记录一致。

升级前备份：

- `app_settings`。
- 当前 `/opt/nat-query/VERSION`。
- 当前 systemd 应用服务文件。

升级后处理：

- `systemctl daemon-reload`。
- `systemctl restart nat-query-service`。
- 不重启 ClickHouse，除非应用明确需要重新连接。
- 升级失败时保留错误日志，页面可查看。

## CI 与打包任务

V2 发布流水线需要生成：

```text
fwlog-full-v{APP_VERSION}-amd64.tar.gz
fwlog-upgrade-v{APP_VERSION}.x86_64.rpm
fwlog-upgrade_{APP_VERSION}_amd64.deb
latest.json
checksums.txt
```

打包脚本需要支持两种模式：

```bash
PACKAGING_MODE=full
PACKAGING_MODE=upgrade
```

`full` 模式：

- 构建应用二进制。
- 构建前端静态资源。
- 下载或复用 ClickHouse。
- 下载或复用 GeoIP。
- 生成 full rpm/deb。
- 生成外层 tar.gz。

`upgrade` 模式：

- 构建应用二进制。
- 构建前端静态资源。
- 下载或复用 GeoIP。
- 生成 upgrade rpm/deb。
- 不下载 ClickHouse。
- 不包含 ClickHouse systemd 服务。

RPM/DEB 建议名称：

```text
fwlog-full
fwlog-upgrade
```

包描述中明确：

- `fwlog-full`：offline full installer with bundled ClickHouse runtime。
- `fwlog-upgrade`：application upgrade package without ClickHouse runtime。

## install.sh 逻辑

全量离线包里的 `install.sh` 负责：

- 判断系统包管理器。
- Kylin/RHEL 系使用 `rpm -Uvh` 安装 `fwlog-full`。
- Debian/Ubuntu 系使用 `dpkg -i` 安装 `fwlog-full`。
- 执行 `systemctl daemon-reload`。
- 启动并启用 `fwlog-clickhouse.service`。
- 启动并启用 `nat-query-service.service`。
- 输出访问地址和服务状态。

`install.sh` 不处理普通升级包；普通升级由系统包管理器或页面升级流程处理。

## 数据安全约束

任何升级路径都不得删除：

```text
/data/clickhouse/
/data/index/
/opt/nat-query/clickhouse/
```

普通升级不得覆盖：

```text
/opt/nat-query/clickhouse/config.xml
/opt/nat-query/clickhouse/users.xml
/etc/systemd/system/fwlog-clickhouse.service
```

应用配置保留规则：

- 已存在的运行配置优先。
- 包内配置模板只用于首次安装。
- 升级前后必须保留系统设置、扫描计划、日志源、升级开关、管理员密码哈希。

## V2 任务清单

- [x] 拆分 full/upgrade 两种打包模式。
- [x] 新增 DEB 打包产物。
- [x] 新增全量离线 tar.gz 结构与 `install.sh`。
- [x] 生成 `latest.json` 和 `checksums.txt`。
- [x] 新增 `VERSION` 和 `RUNTIME_VERSION` 安装写入逻辑。
- [x] 后端新增本地版本读取接口。
- [ ] 后端新增 manifest 检查和升级状态接口。
- [ ] 后端升级安装流程增加包类型、版本、架构、sha256、ClickHouse 路径校验。
- [x] 升级页改为应用版本检查，不默认判断 ClickHouse 版本。
- [ ] 升级页 runtime 不满足时提示使用全量离线包。
- [x] 升级页支持手动上传离线升级包。
- [x] RPM/DEB 升级脚本保留 app_settings。
- [ ] 验证 Kylin x86_64 全量安装。
- [ ] 验证 Kylin x86_64 升级包升级。
- [ ] 验证 Debian/Ubuntu amd64 全量安装。
- [ ] 验证 Debian/Ubuntu amd64 升级包升级。

## 验收标准

- 首次离线安装只需要 `fwlog-full-v*-amd64.tar.gz`。
- Kylin 服务器能通过 full rpm 一次安装并启动应用和 ClickHouse。
- Debian 系服务器能通过 full deb 一次安装并启动应用和 ClickHouse。
- 从旧应用版本升级到新版本时，只需要安装 `fwlog-upgrade` rpm/deb。
- 升级包体积明显小于全量包，不包含 ClickHouse。
- 升级后历史日志数据仍可查询。
- 升级后系统设置、扫描计划、日志源、IP 库路径、管理员密码哈希不丢失。
- 升级页能正确显示当前应用版本、runtime 版本、最新版本和升级方式。
- runtime 不满足时不会误装升级包，页面提示使用全量离线包。

## 暂不做

- 不拆独立 `fwlog-clickhouse-runtime` 包。
- 不拆独立 GeoIP 包。
- 不引入 ADB、zvec、DuckDB 等替代 ClickHouse。
- 不在普通升级包中升级 ClickHouse。
- 不做在线包仓库依赖安装，优先保证离线现场可用。
