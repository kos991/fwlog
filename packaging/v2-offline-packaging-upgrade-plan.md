# V2 离线安装包与升级包计划

fwlog V2 发布分为两类交付物：

- 完整离线安装包：用于新机器首次部署，包含应用、Web 产物、systemd 服务和私有 ClickHouse 运行时。
- 应用升级包：用于已有环境的小版本升级，只更新应用侧文件，不覆盖 ClickHouse 运行时和历史数据。

## 产物命名

示例版本使用 `v2.0.0`：

```text
fwlog_linux_amd64
fwlog-full-v2.0.0-amd64.tar.gz
fwlog-full-v2.0.0.x86_64.rpm
fwlog-full_2.0.0_amd64.deb
fwlog-upgrade-v2.0.0.x86_64.rpm
fwlog-upgrade_2.0.0_amd64.deb
checksums.txt
latest.json
```

## 完整离线包内容

```text
/opt/fwlog/fwlog
/opt/fwlog/VERSION
/opt/fwlog/RUNTIME_VERSION
/opt/fwlog/clickhouse/bin/clickhouse
/opt/fwlog/clickhouse/etc/config.xml
/opt/fwlog/clickhouse/etc/users.xml
/etc/systemd/system/fwlog.service
/etc/systemd/system/fwlog-clickhouse.service
```

完整包安装后需要启用并启动：

```bash
systemctl enable fwlog-clickhouse.service fwlog.service
systemctl restart fwlog-clickhouse.service
systemctl restart fwlog.service
```

## 应用升级包内容

```text
/opt/fwlog/fwlog
/opt/fwlog/VERSION
/etc/systemd/system/fwlog.service
```

升级包不包含：

```text
/opt/fwlog/clickhouse/
/etc/systemd/system/fwlog-clickhouse.service
```

升级流程：

1. 安装前尽量备份 `app_settings` 到 `/data/fwlog/backups/`。
2. 安装新的应用二进制和服务文件。
3. 写入新的 `/opt/fwlog/VERSION`。
4. 重启 `fwlog.service`。
5. 验证 `fwlog.service` 处于 active 状态。

## latest.json

发布流水线生成 `latest.json`，用于前端或运维脚本展示最新版本和可下载包：

```json
{
  "app_version": "v2.0.0",
  "runtime_version": "clickhouse-25.8.27.1",
  "arch": "amd64",
  "assets": {
    "rpm": {
      "name": "fwlog-upgrade-v2.0.0.x86_64.rpm",
      "sha256": "..."
    },
    "deb": {
      "name": "fwlog-upgrade_2.0.0_amd64.deb",
      "sha256": "..."
    },
    "full": {
      "name": "fwlog-full-v2.0.0-amd64.tar.gz",
      "sha256": "..."
    }
  }
}
```

## 验证清单

- `go test ./...`
- `npm.cmd test`
- `npm.cmd run build`
- `go build ./cmd/fwlog`
- 完整包内包含 `/opt/fwlog/clickhouse/`
- 升级包内不包含 `/opt/fwlog/clickhouse/`
- RPM/DEB 安装后 `fwlog.service` 可重启成功
- `checksums.txt` 能校验所有发布资产
