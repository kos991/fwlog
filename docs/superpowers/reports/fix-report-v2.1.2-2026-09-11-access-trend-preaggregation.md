# 访问趋势预聚合修复报告

## 1. 问题与范围

访问趋势此前直接从 `nat_logs` 扫描时间范围内的原始日志，并按来源与五元组执行高基数聚合。`LIMIT 500` 位于聚合之后，不能降低扫描和分组成本；生产数据增长后还会受到查询时间和读取行数限制。

本轮先将访问趋势切换到专用日级五元组聚合表；真实数据验证发现最近七天仍需向 Go 进程传输 `1,558,762` 个五元组状态，接口首次请求耗时约 `22.2s`、后续冷请求约 `16.0–16.6s`。最终请求路径改为复用既有 `dashboard_daily_ip_counts` 的 `dst_subnet` 日聚合，只按目标网段完成国内、国外和未知地域统计，并删除不再需要的线路展示。访问趋势响应由 `flows` 调整为 `points`，原始日志表与日志查询接口不变。

## 2. 改动文件

- [`internal/storage/clickhouse/dashboard_aggregates.go`](../../../internal/storage/clickhouse/dashboard_aggregates.go)：新增访问趋势聚合表及物化视图。
- [`internal/storage/clickhouse/access_trend.go`](../../../internal/storage/clickhouse/access_trend.go)：查询切换到既有目标网段日聚合表。
- [`internal/model/types.go`](../../../internal/model/types.go)：访问趋势响应由五元组线路改为目标地域节点。
- [`internal/server/access_trend_adapter.go`](../../../internal/server/access_trend_adapter.go)：增加精确自然日窗口，并一次性完成地域汇总和各模式 Top 50 节点快照。
- [`internal/server/dashboard_cache.go`](../../../internal/server/dashboard_cache.go)：缓存完整访问趋势快照，模式切换不再重复执行 GeoIP 分类。
- [`internal/server/settings_controller.go`](../../../internal/server/settings_controller.go)：GeoIP 数据重载后清除相关地域缓存。
- [`internal/importer/importer.go`](../../../internal/importer/importer.go)：日期重建同步清理访问趋势聚合分区。
- [`scripts/backfill-access-trend-aggregates.sh`](../../../scripts/backfill-access-trend-aggregates.sh)：新增最近七天的历史回填、逐日对账和原子切换脚本。
- [`packaging/build-server-packages.sh`](../../../packaging/build-server-packages.sh) 与 [`packaging/rpm/fwlog.spec`](../../../packaging/rpm/fwlog.spec)：将回填脚本接入 DEB/RPM 安装流程。
- [`web/src/pages/AccessTrendPreview.tsx`](../../../web/src/pages/AccessTrendPreview.tsx)：删除线路图层、线路选择器和线路详情，只保留目标地域节点、时间范围与国内/国外模式。
- [`web/src/styles.css`](../../../web/src/styles.css)：删除线路相关样式，将访问总量、国内和国外统计收进地图顶部，避免右侧空白栏。
- [`web/tests/accessTrendPreview.test.ts`](../../../web/tests/accessTrendPreview.test.ts)：增加无线路图层、无线路选择器、`points` 响应及紧凑布局回归断言。
- 对应 Go、Web、Shell 和打包测试同步更新。

## 3. 关键修复

- 请求不再读取 `access_trend_daily_flows`，改从 `dashboard_daily_ip_counts` 的 `dimension='dst_subnet'` 读取，按 `address` 汇总 `sum(rows)`；SQL 不引用 `nat_logs`。
- 最近七天唯一目标网段由五元组状态的 `1,558,762` 条降为 `157,143` 条，数据库聚合实测约 `0.089s`。
- Go 层只对目标网段执行一次 GeoIP 分类，生成完整汇总及 all/domestic/overseas 三组 Top 50 节点；五分钟缓存保存该完整快照，切换模式不再遍历原始聚合结果。
- `access_trend_daily_flows` 表和物化视图暂时保留，避免本轮引入删除迁移风险，但线上请求路径不再使用。
- 默认窗口改为包含今天在内的七个自然日，并增加排他的日期上界。
- 访问趋势沿用五分钟缓存和并发请求合并；刷新失败显式返回错误，不使用未标记的过期结果；GeoIP 重载会主动清除缓存。
- 历史回填仅处理最近七天，按来源和日期逐日对账；切换期间使用 staging 表和原子重命名。

## 4. 验收与验证

本轮实际执行：

- `go test -count=1 ./...`：通过。
- `go build ./...`：通过。
- `npm.cmd test`：75 项全部通过。
- `npm.cmd run build`：通过；保留既有 ECharts 大分包警告。
- `bash scripts/test-backfill-dashboard-aggregates.sh`：通过。
- `bash scripts/test-backfill-access-trend-aggregates.sh`：通过。
- 测试机 ClickHouse `25.8.27.1` 只读验证：`argMaxState`/`argMaxMerge` 支持 `Tuple(IPv6, UInt16)` 值和 `Tuple(DateTime, String, UInt64)` 排序状态，返回 NAT 地址、端口、计数和最后时间符合预期。
- 测试机 `192.168.0.130` 已部署 `v2.1.2-test`：服务为 `active`，PID `152244`，`NRestarts=0`；最终二进制 SHA-256 为 `88d759cc665512df3d418a7b8ce578286e664858fe1aac89cb54a3c235645b51`，备份位于 `/opt/fwlog/backups/20260911-134017`。
- 测试机补齐 `/opt/fwlog/GeoLite2-City.mmdb` 并通过 systemd drop-in 启用 `GEOIP_ENABLED=true`；数据文件 SHA-256 为 `e3e524f0e22815b0f7d0b49921cffe40f88923c9e4fe2a78518f13d297355c7d`。
- 使用项目同一 GeoIP 引擎验证：`121.46.8.162` 识别为中国，`121.32.254.197` 识别为中国/广州市，`8.8.8.8` 识别为美国，均返回有效经纬度。
- 测试机最近七天共有 `157,143` 个唯一目标网段、`3,097,616` 次访问，与页面总访问量一致。
- 真实接口冷加载为 `2.525s`，相比旧实现 `16.0–22.2s` 降低约 84%–89%；缓存命中和国内/国外模式切换请求约 `0–11ms`。
- 使用真实登录态进行 1440×900 浏览器验证：线路文字数量为 0，目标地域节点、三项统计均可见，控制台错误为 0；截图见 [`../../../.tmp-preview/access-trend-final.png`](../../../.tmp-preview/access-trend-final.png)。

Windows 本机没有可用 CGO/GCC，未执行 `go test -race -count=1 ./...`；Race 检测仍由 Linux CI 完成。DEB/RPM 真实产物构建依赖 Linux 工具链，本轮只执行了 `go test ./packaging` 中的内容与安装顺序断言。

## 5. 版本同步

版本保持 `v2.1.2`。用户未要求升版，本轮继续作为当前未发布工作区的一部分；未修改 release tag、release notes、`VERSION` 或包版本。

## 6. 风险与回滚边界

- 冷加载仍需在 Go 侧分类约 15.7 万个目标网段，测试机实测约 2.5 秒；五分钟缓存命中后为毫秒级。继续压缩冷加载需要增加地域维度预聚合及相应回填流程，不属于本轮最小修复。
- API 从 `flows` 改为 `points`，当前仓库前端已同步；独立使用旧 `flows` 字段的外部客户端需要一并升级。
- 回滚只需恢复备份二进制；保留的 `access_trend_daily_flows` 与现有目标网段聚合不会影响 `nat_logs`。

## 7. 结论

访问趋势已停止读取高基数五元组聚合，改为复用目标网段日聚合并缓存完整地域快照。线路图层、选择器、统计与详情已全部删除，页面只展示可定位的访问目标节点及国内/国外统计。自动化测试、真实浏览器、测试机部署和冷/热接口耗时均已验证。
