# fwlog V2

fwlog 是面向防火墙/NAT 日志的导入、接收、查询和运维服务。当前仓库以 V2 作为唯一主线推进：代码入口、服务名、安装路径、打包产物均已迁移到 `fwlog`。

## 当前定位

- V2：多源日志导入、接收、查询和运维面板。
- V3：后续接入 AI 能力，不在当前主线范围内。

## 目录结构

- `cmd/fwlog`：服务唯一入口。
- `internal/server`：HTTP 服务、路由、中间件、静态文件和 API 适配层。
- `internal/importer`：日志扫描、解析、导入协调和定时扫描。
- `internal/query`：查询请求、可见范围、SQL 构建和查询保护规则。
- `internal/dashboard`：健康面板、入库进度和分布指标组装。
- `internal/security`：密码认证和 Session 管理。
- `internal/storage`：ClickHouse 存储、迁移、设置和导入状态存储。
- `internal/config`：配置加载和配置类型。
- `internal/ip`：自定义 IP 映射、CIDR 别名和 GeoIP 加载。
- `internal/health`：系统健康检查。
- `internal/upgrade`：升级服务。
- `internal/version`：版本服务。
- `web`：前端工程，构建产物输出到 `internal/server/web/dist`。
- `packaging`、`scripts`、`docs`：打包、部署脚本和文档。

## 本地运行

```powershell
go run ./cmd/fwlog
```

常用配置包括 ClickHouse 地址、日志目录、管理员密码和 IP 数据路径，具体字段见 `internal/config`。

## 构建与验证

```powershell
go test ./...

Push-Location web
npm.cmd test
npm.cmd run build
Pop-Location

go build ./cmd/fwlog
```

前端构建产物不提交到仓库，由 Vite 输出到 `internal/server/web/dist` 后被 Go 静态文件服务使用。

## 部署约定

- 服务名：`fwlog.service`
- ClickHouse 私有运行时服务名：`fwlog-clickhouse.service`
- 安装路径：`/opt/fwlog`
- 数据与备份路径：`/data/fwlog`
- Linux amd64 二进制产物：`fwlog_linux_amd64`
- RPM/DEB 升级包：`fwlog-upgrade-v{version}.x86_64.rpm`、`fwlog-upgrade_{version}_amd64.deb`
- 离线完整包：`fwlog-full-v{version}-amd64.tar.gz`
