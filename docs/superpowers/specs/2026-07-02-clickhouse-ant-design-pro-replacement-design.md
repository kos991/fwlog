# ClickHouse 版完全替换与 Ant Design Pro 前端设计

## 1. 结论

NAT Query Service 新版本采用 ClickHouse 版完全替换旧 DuckDB 版。DuckDB 查询库、旧索引重建流程、`tmp_import.csv` 导入链路、旧 Vue/Element Plus 单页和全局“索引重建中”阻断查询都进入废弃范围，不再作为新系统的生产路径。

新系统目标架构：

```text
Go + Gin
ClickHouse native TCP
Ant Design Pro / React
单一管理员密码登录
静态前端嵌入 Go 二进制
```

前端彻底替换为 Ant Design Pro。最终部署仍保持一个 `nat-query-service` 二进制，Go 服务直接提供 API 和前端静态文件。

## 2. 产品导航

主导航保留四个入口：

```text
监控大屏
日志检索
增量进度
系统维护
```

不单独暴露“日志源设置”“IP 库设置”作为一级菜单；它们归入系统维护。

## 3. 监控大屏

监控大屏定位为数据健康和导入健康，不做泛化安全态势页。

默认统计范围为最近 7 天 ready 数据，页面提供范围切换：

```text
今天
昨天
最近 7 天
最近 30 天
全部
```

监控大屏包含四组信息：

```text
数据健康
导入健康
IP 分布
国家地区分布
```

数据健康展示：

- 总日志量
- ready 日期数
- pending / importing / failed 日期数
- 当前可查询时间范围
- 最近一次成功入库时间
- ClickHouse 磁盘占用
- 今日/昨日新增行数

导入健康展示：

- 当前增量状态
- 当前日志源标识
- 当前处理日期
- 当前处理文件
- 文件进度
- 字节进度
- 已入库行数
- 最近错误
- 下次自动增量时间

IP 分布展示：

- Top 源 IP
- Top 目标 IP
- Top NAT IP
- 内网 / 公网 / 自定义标注占比
- 按日志标识分布

国家地区分布展示：

- 国家 Top N
- 城市/地区 Top N
- 未识别 IP 占比
- GeoIP 库加载状态

国家地区分布第一版不对全量日志逐条做 GeoIP 解析。后端先从 ClickHouse 查询 Top IP 或最近范围内聚合 IP，再由 Go 侧 IP 引擎补充国家地区并汇总，避免大范围扫描造成接口不可用。后续如有必要再增加预聚合表或物化视图。

## 4. 日志检索

日志检索必须支持精确日期时间范围，不只支持日期。

前端使用 Ant Design 的日期时间范围选择器：

```text
YYYY-MM-DD HH:mm:ss 到 YYYY-MM-DD HH:mm:ss
```

基础筛选项：

- 时间范围
- 任意 IP
- 源 IP
- 目标 IP
- NAT IP
- 源端口
- 目标端口
- NAT 端口
- 协议
- 动作
- 日志标识

查询结果主表默认显示：

- 时间
- 日志标识
- 源 IP / 源端口
- 目标 IP / 目标端口
- NAT IP / NAT 端口
- 协议
- 动作
- 源 IP 标注
- 目标 IP 国家地区

展开详情显示：

- `source_file`
- `source_offset`
- `source_id`
- `log_date`
- `ingested_at`
- 完整 IP 标注信息

查询只读取已经入库并通过可见性控制的范围。如果用户选择的时间范围跨越 ready、importing、pending、failed 日期，系统只查询已入库部分，并在页面明确提示。

示例提示：

```text
所选时间包含未完成入库日期，已自动只查询已入库部分。
```

查询响应需要返回可见性信息：

```json
{
  "visibility": {
    "partial": true,
    "message": "所选时间包含未完成入库日期，已自动只查询已入库部分。",
    "queried_ranges": [],
    "skipped_dates": []
  }
}
```

前端在结果表上方显示黄色提示条，并提供跳转到“增量进度”的入口。

## 5. 增量进度

“导入进度”统一改名为“增量进度”。该页面是独立一级页面。

页面展示：

- 当前增量状态：空闲 / 扫描中 / 入库中 / 失败 / 已完成
- 当前日志源：日志目录 + 自定义标识
- 当前处理日期
- 当前处理文件
- 文件进度：已完成 / 总数
- 字节进度：已读取 / 总大小
- 已解析/已入库行数
- 耗时
- 预计剩余
- 最近一次自动增量时间
- 下一次自动增量时间
- 最近错误

日期列表使用中文业务状态：

```text
ready      已入库
importing  入库中
pending    待入库
failed     入库失败
```

日期列表默认显示 `importing`、`failed`、`pending` 日期，避免大量 ready 历史日期淹没问题项。页面提供“显示已入库日期”开关。

每个日期行展示：

- 日志源标识
- 日志日期
- 状态
- 文件总数
- 已完成文件数
- 已入库行数
- 字节进度
- 最近更新时间
- 错误信息
- 重试操作

全量重建保留，但放在增量进度页或系统维护页的“维护操作”区域，作为低频维护动作，不作为主流程入口。

## 6. 系统维护

系统维护使用 Tab 组织：

```text
日志源
IP 库
自动增量
维护操作
登录安全
```

日志源 Tab：

- 日志目录
- 自定义标识
- 启用状态
- 检测目录
- 保存配置

第一版只开放一个启用日志源；数据模型按多源设计，后续可以扩展多个日志源。

IP 库 Tab：

- 自定义 IP 映射 CSV 路径
- GeoIP mmdb 路径
- 启用/禁用自定义 IP 映射
- 启用/禁用 GeoIP
- 重新加载 IP 库
- 最近加载状态
- 最近错误

IP 库路径保存后支持热加载，不要求重启服务。加载失败时保留旧 IP 引擎，并把错误写入设置状态。

自动增量 Tab：

- 自动增量开关
- 调度模式：`hourly` / `daily` / `custom`
- 执行时间
- 时区
- 抖动秒数
- 保存后无需重启服务，下一轮调度立即按新配置计算

维护操作 Tab：

- 立即增量一次
- 重试失败日期
- 指定日期重建
- 全量重建

登录安全 Tab：

- 修改单一管理员密码

不引入用户账号和角色权限。

## 7. ClickHouse 数据模型

核心表：

```text
nat_logs
ingest_dates
ingest_files
log_sources
app_settings
```

第一版建表以“可快速日期级重建、可见性状态清晰、默认不使用 `Nullable`”为准。Go 层负责把缺失值写成默认值，例如空字符串、`0`、`0.0.0.0`。

### 7.1 正式 DDL

```sql
CREATE TABLE IF NOT EXISTS app_settings
(
    key String,
    value String,
    updated_at DateTime DEFAULT now()
)
ENGINE = ReplacingMergeTree(updated_at)
PRIMARY KEY key
ORDER BY key;

CREATE TABLE IF NOT EXISTS log_sources
(
    source_id String,
    log_dir String,
    log_tag LowCardinality(String),
    enabled UInt8 DEFAULT 1,
    updated_at DateTime DEFAULT now()
)
ENGINE = ReplacingMergeTree(updated_at)
PRIMARY KEY source_id
ORDER BY source_id;

CREATE TABLE IF NOT EXISTS ingest_dates
(
    source_id String,
    log_tag LowCardinality(String),
    log_date Date,
    status LowCardinality(String) DEFAULT 'pending',

    files_total UInt64 DEFAULT 0,
    files_done UInt64 DEFAULT 0,
    rows_imported UInt64 DEFAULT 0,
    bytes_total UInt64 DEFAULT 0,
    bytes_done UInt64 DEFAULT 0,

    current_file String DEFAULT '',
    progress_pct Float64 DEFAULT 0,
    max_visible_timestamp DateTime DEFAULT toDateTime(0),

    retry_count UInt8 DEFAULT 0,
    next_retry_at DateTime DEFAULT toDateTime(0),
    started_at DateTime DEFAULT toDateTime(0),
    finished_at DateTime DEFAULT toDateTime(0),
    error String DEFAULT '',
    updated_at DateTime DEFAULT now()
)
ENGINE = ReplacingMergeTree(updated_at)
PRIMARY KEY (source_id, log_date)
ORDER BY (source_id, log_date);

CREATE TABLE IF NOT EXISTS ingest_files
(
    path String,
    source_id String,
    log_tag LowCardinality(String),
    log_date Date,

    size_bytes UInt64 DEFAULT 0,
    mtime DateTime DEFAULT toDateTime(0),
    status LowCardinality(String) DEFAULT 'pending',
    rows_imported UInt64 DEFAULT 0,
    bytes_total UInt64 DEFAULT 0,
    bytes_done UInt64 DEFAULT 0,
    progress_pct Float64 DEFAULT 0,

    stable_seen_count UInt8 DEFAULT 0,
    first_seen_at DateTime DEFAULT toDateTime(0),
    retry_count UInt8 DEFAULT 0,
    next_retry_at DateTime DEFAULT toDateTime(0),
    started_at DateTime DEFAULT toDateTime(0),
    finished_at DateTime DEFAULT toDateTime(0),
    error String DEFAULT '',
    updated_at DateTime DEFAULT now()
)
ENGINE = ReplacingMergeTree(updated_at)
PRIMARY KEY path
ORDER BY path;

CREATE TABLE IF NOT EXISTS nat_logs
(
    source_id String CODEC(ZSTD(3)),
    log_tag LowCardinality(String) CODEC(ZSTD(3)),
    log_date Date CODEC(DoubleDelta, ZSTD(3)),
    timestamp DateTime CODEC(DoubleDelta, ZSTD(3)),

    src_ip IPv4 CODEC(ZSTD(3)),
    src_port UInt16 CODEC(T64, ZSTD(3)),
    dst_ip IPv4 CODEC(ZSTD(3)),
    dst_port UInt16 CODEC(T64, ZSTD(3)),
    nat_ip IPv4 CODEC(ZSTD(3)),
    nat_port UInt16 CODEC(T64, ZSTD(3)),

    protocol LowCardinality(String) CODEC(ZSTD(3)),
    action LowCardinality(String) DEFAULT 'ALLOW' CODEC(ZSTD(3)),

    source_file LowCardinality(String) CODEC(ZSTD(3)),
    source_offset UInt64 CODEC(Delta, ZSTD(3)),
    batch_id String DEFAULT '' CODEC(ZSTD(3)),
    ingested_at DateTime DEFAULT now() CODEC(DoubleDelta, ZSTD(3))
)
ENGINE = MergeTree
PARTITION BY log_date
ORDER BY (log_date, source_id, src_ip, timestamp)
SETTINGS index_granularity = 8192;
```

可选 TTL 不在第一版默认启用。确认保留周期、磁盘卷策略和低峰维护窗口后，再通过 `ALTER TABLE ... MODIFY TTL` 增加：

```sql
ALTER TABLE nat_logs
MODIFY TTL
    timestamp + INTERVAL 30 DAY RECOMPRESS CODEC(ZSTD(6)),
    timestamp + INTERVAL 90 DAY RECOMPRESS CODEC(ZSTD(9));

-- 如果未来确认删除保留期，再单独追加类似规则：
-- ALTER TABLE nat_logs
-- MODIFY TTL
--     timestamp + INTERVAL 30 DAY RECOMPRESS CODEC(ZSTD(6)),
--     timestamp + INTERVAL 90 DAY RECOMPRESS CODEC(ZSTD(9)),
--     timestamp + INTERVAL 180 DAY DELETE;
```

TTL 语法采用 ClickHouse 官方表级 TTL 形式，重压缩使用 `RECOMPRESS CODEC(...)`，删除规则使用 `DELETE`。删除 TTL 不默认写死，避免误删合规留存数据。

### 7.2 分区键和排序键

`nat_logs` 第一版使用：

```text
PARTITION BY log_date
ORDER BY (log_date, source_id, src_ip, timestamp)
```

`PARTITION BY log_date` 用于日期级重建和失败重试。某个日期未最终进入 ready 前，重试可以直接清理单日分区再整日重导。

`ORDER BY` 的选择优先服务第一版主路径：

- `log_date`：让跨天查询先按日期裁剪数据。
- `source_id`：多日志源隔离，查询某个日志源时跳过其他源。
- `src_ip`：内网源 IP 溯源是高频路径，同源 IP 数据更容易聚拢。
- `timestamp`：在日期、日志源、源 IP 缩小范围后做时间截断。

如果后续 `nat_ip + nat_port` 反查成为高频路径，再增加面向 NAT 反查的物化视图或投影，不在第一版主表排序键里过度折中。

`log_tag` 写入导入时刻的自定义标识。页面修改标识只影响后续入库数据，历史数据不自动改名。历史统一改名需要单独维护任务。

`ingest_dates` 负责日期级可见性控制。查询前必须根据所选时间范围读取 ready 或部分可见日期，构造实际可查时间范围。

`ingest_files` 负责文件级进度、重试、错误记录。

`log_sources` 负责日志目录和自定义标识配置。

`app_settings` 负责自动增量配置、IP 库配置和其他运行时设置。

### 7.3 状态表查询规范

`ReplacingMergeTree(updated_at)` 的去重合并是后台异步执行的，同一主键可能短时间存在多个版本。Go 代码读取状态时必须显式取最新版本。

单个日期状态点查：

```sql
SELECT
    status,
    files_total,
    files_done,
    rows_imported,
    bytes_total,
    bytes_done,
    current_file,
    progress_pct,
    max_visible_timestamp,
    error,
    updated_at
FROM ingest_dates
WHERE source_id = ? AND log_date = ?
ORDER BY updated_at DESC
LIMIT 1;
```

增量进度页和监控大屏读取日期列表时，优先使用 `argMax` 获取每个日期的最新状态：

```sql
SELECT
    source_id,
    argMax(log_tag, updated_at) AS log_tag,
    log_date,
    argMax(status, updated_at) AS status,
    argMax(files_total, updated_at) AS files_total,
    argMax(files_done, updated_at) AS files_done,
    argMax(rows_imported, updated_at) AS rows_imported,
    argMax(bytes_total, updated_at) AS bytes_total,
    argMax(bytes_done, updated_at) AS bytes_done,
    argMax(current_file, updated_at) AS current_file,
    argMax(progress_pct, updated_at) AS progress_pct,
    argMax(max_visible_timestamp, updated_at) AS max_visible_timestamp,
    argMax(error, updated_at) AS error,
    max(updated_at) AS updated_at
FROM ingest_dates
WHERE log_date >= ?
GROUP BY source_id, log_date
ORDER BY log_date DESC, source_id ASC;
```

文件状态点查：

```sql
SELECT
    status,
    rows_imported,
    bytes_total,
    bytes_done,
    progress_pct,
    stable_seen_count,
    retry_count,
    next_retry_at,
    error,
    updated_at
FROM ingest_files
WHERE path = ?
ORDER BY updated_at DESC
LIMIT 1;
```

`app_settings` 和 `log_sources` 数据量很小，可以使用 `FINAL` 简化读取：

```sql
SELECT key, value
FROM app_settings FINAL;

SELECT source_id, log_dir, log_tag, enabled
FROM log_sources FINAL
WHERE enabled = 1;
```

不建议在 `ingest_dates` 和 `ingest_files` 的列表接口里使用 `FINAL`。这些表后续可能持续增长，列表读取用 `argMax` 更稳。

## 8. API 契约

前端使用以下 API：

```text
GET  /api/session
POST /api/login
POST /api/logout
POST /api/password

GET  /api/health-dashboard
GET  /api/query
POST /api/export

GET  /api/log-dates
GET  /api/ingest-progress

GET  /api/settings
POST /api/settings

POST /api/sync
POST /api/rebuild
POST /api/ip-data/reload
```

`GET /api/query` 返回：

- `records`
- `total`
- `page`
- `page_size`
- `query_time_ms`
- `visibility`

`GET /api/ingest-progress` 返回：

- 当前增量状态
- 当前日志源
- 当前日期
- 当前文件
- 文件进度
- 字节进度
- 已入库行数
- 耗时
- 预计剩余
- 下次自动增量时间
- 日期状态列表

`GET /api/health-dashboard` 返回：

- `data_health`
- `ingest_health`
- `ip_distribution`
- `geo_distribution`

## 9. 前端实现边界

前端使用 Ant Design Pro / React。

构建后静态产物嵌入 Go 服务，保留单二进制部署模式。

开发阶段可以使用前端 dev server 代理到 Go API；生产阶段由 Go 服务直接提供静态资源。

旧 Vue/Element Plus 单页不继续扩展。新前端完成后删除旧页面或改为不可用的废弃入口，避免用户误用旧功能。

## 10. 废弃范围

以下内容从新版本生产路径移除：

- DuckDB 查询库
- DuckDB 索引重建流程
- `tmp_import.csv`
- 全量落地解压后导入
- 旧 Vue/Element Plus 单页
- 全局“索引重建中，请稍后再试”阻断查询
- 旧 `/api/stats`、`/api/dashboard` 中面向 DuckDB 的全表聚合逻辑

## 11. 验收标准

功能验收：

- Web 前端为 Ant Design Pro，不再使用旧 Vue 单页作为主界面。
- 登录使用单一管理员密码。
- 监控大屏展示数据健康、导入健康、IP 分布、国家地区分布。
- 日志检索支持日期时间范围。
- 查询跨未入库日期时，只查已入库部分并提示。
- 查询结果显示日志标识，并可展开查看来源文件和偏移。
- 增量进度页可看到当前日志源、当前日期、当前文件、文件进度、字节进度、行数、耗时、预计剩余和错误。
- 系统维护页可以配置日志目录、自定义标识、IP 映射库、GeoIP 库和自动增量。
- IP 库支持热加载，失败不破坏旧 IP 引擎。
- 全量重建作为维护操作保留，但不阻断其他 ready 日期查询。

数据验收：

- 只读取归档日志：`.log-YYYYMMDD` 和 `.log-YYYYMMDD.gz`。
- 不读取当天仍在线写入的 `.log` 文件。
- 日期 ready 后才作为完整日期可查询。
- 部分可见日期通过 `max_visible_timestamp` 控制查询上限。
- 导入失败的日期不进入 ready。
- 失败重试前清理该日期分区再整日重导，保证幂等。

性能验收：

- `.gz` 文件流式解压、流式解析、批量写入 ClickHouse。
- Go 端使用官方 `github.com/ClickHouse/clickhouse-go/v2` native TCP 驱动和 `PrepareBatch`。
- 不走 HTTP 写入，不把 `database/sql` 作为生产导入主路径。
- 监控大屏默认最近 7 天 ready 数据，避免默认全量扫描。

## 12. 迁移策略

实现上可以按“先新后删”推进，但最终结果必须是完全替换。

推荐顺序：

1. 建立 ClickHouse 连接、建表和状态存储。
2. 实现日志源、IP 库、自动增量配置。
3. 实现流式导入、状态更新、进度接口。
4. 实现查询可见性和部分查询提示。
5. 搭建 Ant Design Pro 前端并接入真实 API。
6. 完成验收后删除 DuckDB 生产路径和旧页面。

迁移时旧 DuckDB 数据不进入新系统。新系统按归档日志重新入库 ClickHouse。
