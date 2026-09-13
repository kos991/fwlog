# IP 数据路径统一修复报告

## 1. 问题与范围

维护页同时显示 `/opt/fwlog/custom_ip_map.csv` 和 `/data/index/GeoLite2-City.mmdb`，程序安装目录与 GeoIP 数据目录不一致。本轮将 IP 映射文件和 GeoIP 数据库统一放到 `/opt/fwlog`，日志、ClickHouse、导出和密钥目录保持原约定。

## 2. 改动文件

- [`internal/config/config.go`](../../../internal/config/config.go)
- [`internal/config/config_test.go`](../../../internal/config/config_test.go)
- [`internal/server/server.go`](../../../internal/server/server.go)
- [`internal/server/service_test.go`](../../../internal/server/service_test.go)
- [`fwlog.service`](../../../fwlog.service)
- [`packaging/systemd/fwlog.service`](../../../packaging/systemd/fwlog.service)
- [`packaging/build-server-packages.sh`](../../../packaging/build-server-packages.sh)
- [`packaging/rpm/fwlog.spec`](../../../packaging/rpm/fwlog.spec)
- [`scripts/deploy-142-from-release.sh`](../../../scripts/deploy-142-from-release.sh)
- [`web/src/api.ts`](../../../web/src/api.ts)
- [`web/tests/maintenanceLayout.test.ts`](../../../web/tests/maintenanceLayout.test.ts)

## 3. 关键修复内容

- 默认路径统一为 `/opt/fwlog/custom_ip_map.csv` 与 `/opt/fwlog/GeoLite2-City.mmdb`。
- systemd、Deb/RPM 打包和发布部署脚本同步使用统一路径。
- 已保存的旧默认 GeoIP 路径 `/data/index/GeoLite2-City.mmdb` 在应用启动时自动迁移到新路径；用户明确配置的其他路径不受影响。
- `192.168.0.130` 已更新服务单元并重启，原二进制与服务单元均保留备份。

## 4. 验收方式

已执行：

- `go test ./internal/config ./internal/server ./packaging`：通过。
- `npm.cmd test`：60 项全部通过。
- `npm.cmd run build`：通过。
- 远端 `systemctl is-active fwlog.service`：`active`。
- 远端 systemd 环境变量：`CUSTOM_IP_MAP=/opt/fwlog/custom_ip_map.csv`、`GEOIP_DB=/opt/fwlog/GeoLite2-City.mmdb`。
- 远端 `https://127.0.0.1:8080/`：HTTP `200`。

## 5. 版本同步

版本保持 `v2.1.2`，未修改版本号、发布说明或安装包版本。

## 6. 风险与备注

- 当前目标机未发现旧 GeoIP 文件，未执行文件搬迁；后续安装或上传 GeoIP 文件时应放到 `/opt/fwlog/GeoLite2-City.mmdb`。
- `/opt/fwlog` 已包含在 systemd `ReadWritePaths` 中，服务可在受保护模式下读取该文件。

## 7. 结论

程序默认配置、维护页默认值、systemd 和安装包路径已统一到 `/opt/fwlog`，远端服务运行正常。
