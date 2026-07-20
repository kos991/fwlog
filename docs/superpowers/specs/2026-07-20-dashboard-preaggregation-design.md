# 数据概览预聚合设计

## 背景

生产环境现有约 2 亿行 `nat_logs`。数据概览页面会同时加载概览和 30 天排行，并直接对原始表执行多组聚合。10 分钟生产采样显示：系统 CPU 峰值 99.75%，ClickHouse 峰值约 3.82 核，fwlog GeoIP 聚合峰值约 3.86 核；118 秒系统 CPU 超过 80%，部分概览请求在 20 秒后超时。

目标是保持统计口径准确，同时将概览查询从原始日志表迁移到按日增量聚合表。

## 成功标准

- 数据概览接口 P95 小于 1 秒。
- 排行接口热缓存响应小于 200 毫秒，冷查询小于 3 秒。
- 单次页面加载期间系统 CPU 峰值不超过 30%。
- 排行、趋势和总量与原始表抽样对账一致。
- 日期重建不会造成聚合数据重复或残留。
- 回填期间不影响日志查询；生产切换窗口控制在 1 分钟内。

## 数据模型

新增两个 SummingMergeTree 表。

### dashboard_daily_totals

字段：

- `log_date Date`
- `source_id String`
- `log_tag LowCardinality(String)`
- `rows UInt64`

分区：`(source_id, log_date)`。

排序键：`(log_date, source_id, log_tag)`。

用途：总日志数、今日/昨日行数、14 日趋势、日志标签分布。

### dashboard_daily_ip_counts

字段：

- `log_date Date`
- `source_id String`
- `log_tag LowCardinality(String)`
- `dimension LowCardinality(String)`，限定为 `src_ip`、`dst_ip`、`dst_subnet`
- `address IPv6`
- `rows UInt64`

分区：`(source_id, log_date)`。

排序键：`(dimension, log_date, source_id, address, log_tag)`。

用途：源 IP 排行、目标 IP 排行、目标子网 GeoIP 聚合。

IPv4 继续以 IPv4-mapped IPv6 保存；`dst_subnet` 沿用当前口径：IPv4 按 `/24`，IPv6 按 `/64`。

## 增量聚合

为 `nat_logs` 创建四个物化视图：

- 每个插入块按日期、来源、标签汇总到 `dashboard_daily_totals`。
- 按源 IP 汇总到 `dashboard_daily_ip_counts` 的 `src_ip` 维度。
- 按目标 IP 汇总到 `dst_ip` 维度。
- 按目标子网汇总到 `dst_subnet` 维度。

查询聚合表时始终使用 `sum(rows)`，不依赖后台 merge 是否完成。

## 重建一致性

日期重建开始时，按以下顺序清理同一 `(source_id, log_date)` 分区：

1. `nat_logs`
2. `dashboard_daily_totals`
3. `dashboard_daily_ip_counts`

随后重新插入原始日志，由物化视图重新生成聚合数据。三个分区删除必须全部成功后才开始导入；任一步失败均终止重建并记录失败状态。

该顺序保证旧聚合不会与重建结果叠加。重复执行同一日期重建仍保持幂等。

## API 拆分

新增：

- `GET /api/health-dashboard/summary`
- `GET /api/health-dashboard/rankings`

`summary` 返回数据健康、入库状态、系统状态和日志趋势，只查询状态表与 `dashboard_daily_totals`。

`rankings` 返回源 IP、目标 IP、日志标签、国家和地区排行，只查询两个日聚合表。

原 `/api/health-dashboard` 保留一个补丁版本作为兼容入口，内部组合新服务结果，不再扫描原始表。

## 缓存与并发

- 排行按 `metrics_range + source_id` 缓存 5 分钟。
- 同一缓存键使用 singleflight 合并并发请求。
- 缓存过期时仅允许一个刷新任务；已有成功缓存可作为 stale 数据返回。
- 冷查询 ClickHouse `max_threads` 限制为 2。
- GeoIP 聚合 worker 限制为 2。
- 概览不缓存入库状态；趋势结果允许缓存 30 秒。

## 前端加载

页面先请求 `summary` 并立即展示概览，再请求 `rankings` 填充排行。请求不得并发重复：

- `summary` 空闲时每 30 秒刷新，入库时每 5 秒刷新。
- `rankings` 每 5 分钟刷新。
- 组件卸载或筛选条件变化时取消旧请求。
- 多标签页造成的重复请求由服务端缓存和 singleflight 吸收。

## 历史回填与切换

历史回填使用 staging 表，避免与线上增量写入重复：

1. 创建 staging 聚合表，不创建物化视图。
2. 按日志日期从旧到新回填，每次仅处理一天，查询设置 `max_threads=1`。
3. 每天回填后对账总行数，并记录完成日期和来源。
4. 回填期间线上 fwlog 保持运行，日志查询不受影响。
5. 切换时短暂停止 fwlog，重新计算回填开始后发生变化的日期。
6. 将 staging 表原子重命名为正式表，再创建指向正式表的物化视图。
7. 确认物化视图创建成功后启动新版本，执行总量和抽样排行对账后启用新接口。

若回填耗时超过维护窗口，可以中断并从最后完成日期继续。

## 错误处理与回滚

- 聚合冷查询失败时，排行接口优先返回上一次成功缓存并标记缓存时间。
- 无可用缓存时返回明确错误，不回退到原始表全量扫描，避免再次打满 CPU。
- 部署前备份旧二进制、systemd unit 和当前配置。
- 应用回滚只需恢复旧二进制；新增聚合表和物化视图可保留但暂停使用。
- 数据层回滚先删除物化视图，再删除聚合表，不影响 `nat_logs`。
- 历史回填只写新增表，不修改原始日志。

## 测试与验收

自动测试覆盖：

- 聚合表和物化视图 DDL。
- summary/rankings 路由鉴权与响应结构。
- 缓存命中、过期、stale fallback 和 singleflight。
- 日期重建同时清理三个分区。
- 前端顺序加载、刷新频率和取消旧请求。
- 聚合查询 SQL 不引用 `nat_logs`。

生产验收：

- 随机选择至少 3 个日期，对比原始表与聚合表总量。
- 对比 30 天 Top 10 源 IP、目标 IP、日志标签和国家/地区。
- 使用与本次相同的一秒采样监控 10 分钟。
- 验证 CPU、响应时间、内存、ClickHouse part 数及错误日志。

## 不在本次范围

- 修改原始 `nat_logs` 表结构。
- 改变日志查询接口。
- 改变 GeoIP 归属口径。
- 删除或压缩原始日志数据。
