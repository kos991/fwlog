# fwlog 当前项目架构图与流程图

本文按当前代码实现整理，重点覆盖前端页面、Go HTTP 服务、ClickHouse、日志入库、查询可见性、系统设置、IP 标注和部署链路。

主要代码依据：

- `main.go`
- `app.go`
- `clickhouse_store.go`
- `importer.go`
- `query_service.go`
- `dashboard_service.go`
- `log_scanner.go`
- `ip_engine.go`
- `upgrade_service.go`
- `session_auth.go`
- `web/src/main.tsx`
- `web/src/layout/AppLayout.tsx`
- `web/src/api.ts`

## 1. 系统总架构图

```mermaid
flowchart LR
    User[用户 / 运维人员] --> Browser[浏览器]
    Browser -->|HTTP 8080| GoApp[fwlog Go 服务<br/>net/http ServeMux]

    subgraph Frontend[前端 React + Ant Design Pro]
        Login[登录页]
        Layout[主布局 / 左侧导航]
        Dashboard[数据概览]
        Search[日志查询]
        Progress[入库进度]
        Maintenance[系统设置]
    end

    Browser --> Frontend

    subgraph Backend[后端 Go 应用]
        Router[HTTP 路由]
        Auth[登录会话 / 密码管理]
        QuerySvc[查询服务]
        DashboardSvc[看板与进度服务]
        ImportSvc[归档日志入库服务]
        SettingsSvc[设置持久化]
        UpgradeSvc[自动升级服务]
        IPEngine[IP 标注引擎]
        Static[内嵌前端静态资源]
    end

    GoApp --> Router
    Router --> Auth
    Router --> QuerySvc
    Router --> DashboardSvc
    Router --> ImportSvc
    Router --> SettingsSvc
    Router --> UpgradeSvc
    Router --> IPEngine
    Router --> Static

    subgraph Storage[数据与文件]
        CH[(ClickHouse<br/>nat_logs / ingest_dates / ingest_files / app_settings / log_sources)]
        Logs[/归档日志目录<br/>*.log-YYYYMMDD / *.log-YYYYMMDD.gz/]
    CustomMap[/custom_ip_map.csv/]
    GeoDB[/GeoLite2-City.mmdb/]
    Exports[/导出目录<br/>当前接口占位/]
    GitHubRelease[/GitHub Release<br/>Linux amd64 资产/]
    end

    QuerySvc --> CH
    DashboardSvc --> CH
    ImportSvc --> Logs
    ImportSvc --> CH
    SettingsSvc --> CH
    UpgradeSvc --> GitHubRelease
    IPEngine --> CustomMap
    IPEngine --> GeoDB

    Systemd[systemd<br/>nat-query-service] --> GoApp
```

## 2. 部署架构图

```mermaid
flowchart TB
    subgraph Server142["192.168.0.142"]
        Unit[systemd: nat-query-service]
        Bin[/opt/nat-query/nat-query-service]
        App[Go 服务<br/>监听 0.0.0.0:8080]

        subgraph DataDirs["数据目录"]
            LogDir[/日志目录<br/>默认 /data/sangfor_fw_log/]
            ExportDir[/导出目录<br/>默认 /data/export/]
            GeoPath[/GeoIP 库<br/>默认 /data/index/GeoLite2-City.mmdb/]
        end

        CHServer[ClickHouse Server<br/>127.0.0.1:9000]
        CHData[(ClickHouse 数据)]
    end

    Unit --> Bin
    Bin --> App
    App -->|clickhouse-go/v2 native TCP| CHServer
    CHServer --> CHData
    App --> LogDir
    App --> ExportDir
    App --> GeoPath
```

## 3. 后端运行时模块图

```mermaid
flowchart TB
    Main[main.go<br/>LoadConfig + NewApp + Run] --> App[App]
    App --> Connect[Connect<br/>OpenClickHouse + EnsureTables + LoadSettings]
    App --> Router[Router<br/>注册 HTTP 接口]
    App --> Settings[settings map<br/>运行时配置]
    App --> ImportLock[importMu<br/>防止重复入库任务]
    App --> IPEngine[IP 标注引擎]

    Router --> SessionAPI[/api/session]
    Router --> LoginAPI[/api/login]
    Router --> LogoutAPI[/api/logout]
    Router --> PasswordAPI[/api/password]
    Router --> QueryAPI[/api/query]
    Router --> DashboardAPI[/api/health-dashboard]
    Router --> ProgressAPI[/api/ingest-progress]
    Router --> SettingsAPI[/api/settings]
    Router --> SyncAPI[/api/sync]
    Router --> RebuildAPI[/api/rebuild]
    Router --> UpgradeAPI[/api/upgrade/check<br/>/api/upgrade/status<br/>/api/upgrade/run]
    Router --> IPReloadAPI[/api/ip-data/reload]
    Router --> Static[/ 前端静态资源]

    QueryAPI --> QuerySvc[appQueryService]
    DashboardAPI --> DashboardSvc[appDashboardService]
    ProgressAPI --> DashboardSvc
    PasswordAPI --> SecuritySvc[appSecurityService]
    IPReloadAPI --> SecuritySvc
    SyncAPI --> ImportSvc[startBackgroundImport(false)]
    RebuildAPI --> ImportSvc2[startBackgroundImport(true)]
    UpgradeAPI --> UpgradeSvc[升级检查 / 后台替换二进制]

    QuerySvc --> CH[(ClickHouseStore)]
    DashboardSvc --> CH
    ImportSvc --> CH
    ImportSvc2 --> CH
    SecuritySvc --> IPEngine
```

## 4. 前端页面结构图

```mermaid
flowchart TB
    Root[web/src/main.tsx] --> SessionCheck[启动后请求 /api/session]
    SessionCheck --> Authenticated{已登录?}
    Authenticated -- 否 --> LoginPage[LoginPage]
    Authenticated -- 是 --> AppLayout[AppLayout<br/>Ant Design ProLayout]

    LoginPage -->|POST /api/login| BackendLogin[/api/login]

    AppLayout --> Nav[左侧导航]
    Nav --> Dashboard[数据概览<br/>HealthDashboard]
    Nav --> Search[日志查询<br/>LogSearchPage]
    Nav --> Progress[入库进度<br/>IncrementalProgressPage]
    Nav --> Maintenance[系统设置<br/>SystemMaintenancePage]

    Dashboard -->|GET| HealthAPI[/api/health-dashboard]
    Search -->|GET| QueryAPI[/api/query]
    Progress -->|GET| ProgressAPI[/api/ingest-progress]
    Maintenance -->|GET/POST| SettingsAPI[/api/settings]
    Maintenance -->|POST| PasswordAPI[/api/password]
    Maintenance -->|POST| IPReloadAPI[/api/ip-data/reload]
    Maintenance -->|POST| SyncAPI[/api/sync]
    Maintenance -->|POST| RebuildAPI[/api/rebuild]
    Maintenance -->|GET/POST| UpgradeAPI[/api/upgrade/*]

    AppLayout -->|POST /api/logout| LogoutAPI[/api/logout]
```

## 5. ClickHouse 数据模型图

```mermaid
erDiagram
    nat_logs {
        String source_id
        LowCardinality_String log_tag
        Date log_date
        DateTime timestamp
        IPv4 src_ip
        UInt16 src_port
        IPv4 dst_ip
        UInt16 dst_port
        IPv4 nat_ip
        UInt16 nat_port
        LowCardinality_String protocol
        LowCardinality_String action
        LowCardinality_String source_file
        UInt64 source_offset
        String batch_id
        DateTime ingested_at
    }

    ingest_dates {
        String source_id
        LowCardinality_String log_tag
        Date log_date
        LowCardinality_String status
        UInt64 files_total
        UInt64 files_done
        UInt64 rows_imported
        UInt64 bytes_total
        UInt64 bytes_done
        String current_file
        Float64 progress_pct
        DateTime max_visible_timestamp
        UInt8 retry_count
        DateTime next_retry_at
        String error
        DateTime updated_at
    }

    ingest_files {
        String path
        String source_id
        LowCardinality_String log_tag
        Date log_date
        UInt64 size_bytes
        DateTime mtime
        LowCardinality_String status
        UInt64 rows_imported
        UInt64 bytes_total
        UInt64 bytes_done
        Float64 progress_pct
        UInt8 retry_count
        DateTime next_retry_at
        String error
        DateTime updated_at
    }

    app_settings {
        String key
        String value
        DateTime updated_at
    }

    log_sources {
        String source_id
        String log_dir
        LowCardinality_String log_tag
        UInt8 enabled
        DateTime updated_at
    }

    log_sources ||--o{ nat_logs : writes
    log_sources ||--o{ ingest_dates : tracks
    log_sources ||--o{ ingest_files : tracks
```

关键表职责：

- `nat_logs`：日志查询主表，按 `log_date` 分区，排序键是 `(log_date, source_id, src_ip, timestamp)`。
- `ingest_dates`：日期级入库状态，用来控制查询可见范围和前台进度。
- `ingest_files`：文件级入库状态，用来记录单文件成功、失败和重试信息。
- `app_settings`：系统设置持久化，包括日志源、自动扫描、IP 库路径、CIDR 别名等。
- `log_sources`：预留多日志源数据表；当前运行主要从 `app_settings.log_sources` 或兼容字段解析。

## 6. 服务启动流程图

```mermaid
flowchart TD
    Start([进程启动]) --> LoadConfig[LoadConfig<br/>读取环境变量和默认值]
    LoadConfig --> NewApp[NewApp<br/>初始化密码哈希 / 默认设置 / IPEngine]
    NewApp --> Connect[App.Connect]
    Connect --> OpenCH[OpenClickHouse<br/>连接 127.0.0.1:9000]
    OpenCH --> EnsureTables[EnsureTables<br/>创建 ClickHouse 表]
    EnsureTables --> LoadSettings[LoadSettings<br/>读取 app_settings]
    LoadSettings --> ReloadIP[reloadIPDataFromSettings<br/>加载自定义 IP 映射和 GeoIP]
    ReloadIP --> Router[Router<br/>注册接口和静态资源]
    Router --> Listen[ListenAndServe :8080]
```

## 7. 登录与改密流程图

```mermaid
sequenceDiagram
    participant U as 用户
    participant B as 浏览器
    participant S as Go 服务

    B->>S: GET /api/session
    S-->>B: authenticated=false
    U->>B: 输入管理员密码
    B->>S: POST /api/login
    S->>S: VerifyPassword
    alt 密码正确
        S->>S: 生成 sessionToken
        S-->>B: Set-Cookie fwlog_session
        B->>S: GET /api/session
        S-->>B: authenticated=true
    else 密码错误
        S-->>B: 401 invalid_password
    end

    U->>B: 修改密码
    B->>S: POST /api/password
    S->>S: 校验当前 Cookie 和旧密码
    S->>S: HashPassword(new_password)
    S->>S: 清空 sessionToken
    S-->>B: authenticated=false
    B->>B: 回到登录页
```

## 8. 归档日志扫描流程图

```mermaid
flowchart TD
    Start([ScanArchivedLogFiles]) --> ReadDir[读取日志目录]
    ReadDir --> Loop{遍历目录项}
    Loop --> IsDir{是目录?}
    IsDir -- 是 --> Skip[跳过]
    IsDir -- 否 --> Match{文件名匹配<br/>*.log-YYYYMMDD 或 *.log-YYYYMMDD.gz?}
    Match -- 否 --> Skip
    Match -- 是 --> Stable{mtime 距当前时间 >= 5 分钟?}
    Stable -- 否 --> Skip
    Stable -- 是 --> Extract[从文件名提取 log_date]
    Extract --> Snapshot[生成 LogFileSnapshot<br/>path/name/size/mtime/log_date]
    Snapshot --> Sort[按日期和文件名排序]
    Sort --> Done([返回文件列表])
```

## 9. 入库流程图

```mermaid
flowchart TD
    Start([POST /api/sync 或 /api/rebuild]) --> Lock{importing 锁是否空闲?}
    Lock -- 否 --> Accepted[202<br/>已有入库任务正在执行]
    Lock -- 是 --> Goroutine[启动后台 goroutine]
    Goroutine --> Sources[currentLogSources<br/>读取启用日志源]
    Sources --> Scan[ScanArchivedLogFiles]
    Scan --> Dates[按 log_date 聚合日期]
    Dates --> LoopDate{逐日期处理}

    LoopDate --> Rebuild{rebuild?}
    Rebuild -- 否 --> CheckReady[LatestDateState<br/>ready 日期跳过]
    CheckReady --> ImportNeeded{需要导入?}
    ImportNeeded -- 否 --> SkipDate[跳过该日期]
    ImportNeeded -- 是 --> ImportDate[Importer.ImportDate]
    Rebuild -- 是 --> ImportDate

    ImportDate --> DropPartition[ALTER TABLE nat_logs DROP PARTITION]
    DropPartition --> Sleep[等待 1 秒]
    Sleep --> FilterFiles[筛选该日期归档文件]
    FilterFiles --> DateImporting[写 ingest_dates=importing]
    DateImporting --> LoopFile{逐文件处理}
    LoopFile --> FileImporting[写 ingest_files=importing]
    FileImporting --> Open[打开文件]
    Open --> Gzip{是否 .gz?}
    Gzip -- 是 --> GzipReader[gzip.NewReader]
    Gzip -- 否 --> PlainReader[普通 reader]
    GzipReader --> ParseLoop[逐行 ParseNATLine]
    PlainReader --> ParseLoop
    ParseLoop --> Batch[累计 batch<br/>默认 20000 行]
    Batch --> Insert[PrepareBatch + Send<br/>写入 nat_logs]
    Insert --> FileReady[写 ingest_files=ready]
    FileReady --> DateProgress[更新 ingest_dates 进度]
    DateProgress --> LoopFile
    LoopFile --> DateReady[全部完成后写 ingest_dates=ready]
    DateReady --> LoopDate
    SkipDate --> LoopDate
    LoopDate --> Unlock[任务结束释放 importing 锁]
```

## 10. 入库状态机

```mermaid
stateDiagram-v2
    [*] --> pending: 扫描发现归档日期
    pending --> importing: 开始导入
    importing --> ready: 文件全部写入成功
    importing --> failed: 解压 / 解析 / ClickHouse 写入失败
    failed --> importing: 手动重建或再次同步
    ready --> importing: rebuild=true 时重导该日期
```

## 11. 查询流程图

```mermaid
flowchart TD
    Start([日志查询页面]) --> UserSelect[用户选择时间 / IP / 端口 / 协议 / 日志标识]
    UserSelect --> API[GET /api/query]
    API --> Parse[parseQueryRequest<br/>解析 start/end/page/page_size]
    Parse --> Clamp[page_size 兜底并限制最大 500]
    Clamp --> States[ListDateStates<br/>读取日期入库状态]
    States --> Visibility[BuildVisibleRanges<br/>只保留 ready 或可见 importing 范围]
    Visibility --> HasRange{有可查询范围?}
    HasRange -- 否 --> Empty[返回空 records + visibility]
    HasRange -- 是 --> SQL[BuildQuerySQL<br/>注入 log_date/timestamp 可见范围]
    SQL --> Filter[追加 IP / 端口 / 协议 / 动作 / log_tag 条件]
    Filter --> Sort{有筛选条件?}
    Sort -- 是 --> TimeDesc[ORDER BY timestamp DESC]
    Sort -- 否 --> Fast[不强制排序<br/>利用 ClickHouse 读取路径]
    TimeDesc --> Page[LIMIT/OFFSET]
    Fast --> Page
    Page --> CH[(ClickHouse nat_logs)]
    CH --> Records[扫描查询结果]
    Records --> Enrich[IP 标注<br/>协议数字转 TCP/UDP/ICMP]
    Enrich --> Response[返回 records / page / page_size / query_time_ms / visibility]
```

## 12. 查询可见性流程图

```mermaid
flowchart TD
    Start([BuildVisibleRanges]) --> Dates[按天遍历 start 到 end]
    Dates --> HasState{当天有 ingest_dates 状态?}
    HasState -- 否 --> SkipMissing[加入 skipped_dates<br/>没有入库状态]
    HasState -- 是 --> Status{状态}
    Status -- ready --> ReadyRange[加入 queried_ranges<br/>整天或用户选择范围可查]
    Status -- importing --> HasVisible{max_visible_timestamp 覆盖查询开始?}
    HasVisible -- 否 --> SkipImporting[跳过<br/>尚无可见入库数据]
    HasVisible -- 是 --> PartialRange[加入部分可见范围<br/>end <= max_visible_timestamp]
    Status -- pending --> SkipPending[跳过<br/>未完成入库]
    Status -- failed --> SkipFailed[跳过<br/>入库失败]
    Status -- 其他 --> SkipOther[跳过<br/>状态不可查询]
    ReadyRange --> Next[下一天]
    PartialRange --> Next
    SkipMissing --> Next
    SkipImporting --> Next
    SkipPending --> Next
    SkipFailed --> Next
    SkipOther --> Next
    Next --> Done([生成 visibility])
```

## 13. 数据概览与入库进度流程图

```mermaid
flowchart TD
    Dashboard[数据概览页面] --> HealthAPI[GET /api/health-dashboard]
    ProgressPage[入库进度页面] --> ProgressAPI[GET /api/ingest-progress]

    HealthAPI --> DateStates[ListDateStates<br/>默认近 7 天或指定范围]
    HealthAPI --> Metrics[DashboardMetrics<br/>磁盘占用 / 今日行数 / 昨日行数 / 分布]
    Metrics --> CH[(ClickHouse)]
    DateStates --> BuildHealth[BuildHealthDashboard]
    Metrics --> BuildHealth
    BuildHealth --> HealthJSON[返回 data_health / ingest_health / ip_distribution / geo_distribution]

    ProgressAPI --> ProgressStates[ListDateStates<br/>默认近 30 天]
    ProgressStates --> BuildProgress[BuildIngestProgress]
    BuildProgress --> ProgressJSON[返回当前日期 / 当前文件 / 文件进度 / 字节进度 / rows_imported / dates]
```

## 14. 系统设置流程图

```mermaid
flowchart TD
    Maintenance[系统设置页面] --> GetSettings[GET /api/settings]
    GetSettings --> SettingsMap[返回 app.settings + 默认入库状态字段]

    Maintenance --> Save[POST /api/settings]
    Save --> Decode[解析 JSON payload]
    Decode --> Update[updateSettings<br/>写入内存 settings map]
    Update --> Persist[saveSettings<br/>保存到 ClickHouse app_settings]
    Persist --> ReloadIP[reloadIPDataFromSettings<br/>仅热加载 IP 标注配置]
    ReloadIP --> Response[返回最新 settings]
```

保存系统设置只负责配置落库和 IP 标注配置热加载，不再隐式触发日志扫描或入库。日志入库只通过 `POST /api/sync`、`POST /api/rebuild`，以及后续独立定时调度器触发。

## 15. IP 标注流程图

```mermaid
flowchart TD
    Start([reloadIPDataFromSettings]) --> Config[读取 custom_ip_map_path / geoip_db_path / cidr_aliases / enabled 开关]
    Config --> TempEngine[创建或加载新的 IPEngine]
    TempEngine --> Builtin[内置网段<br/>172.18.0.0/17<br/>172.28.128.0/19<br/>2.0.0.0/8]
    Builtin --> CIDR[加载 CIDR 别名]
    CIDR --> Custom{启用自定义 IP 映射?}
    Custom -- 是 --> LoadCSV[读取 custom_ip_map.csv]
    Custom -- 否 --> Geo
    LoadCSV --> Geo{启用 GeoIP?}
    Geo -- 是 --> LoadGeo[读取 GeoLite2-City.mmdb]
    Geo -- 否 --> Swap
    LoadGeo --> Swap[替换 app.ipEngine 和 ipStatus]
    Swap --> QueryEnrich[查询结果标注 src_ip_label / dst_geo]

    QueryEnrich --> MatchOrder[标注优先级<br/>手工覆盖 > CIDR 网段 > 私网 > GeoIP > 未知公网]
```

## 16. API 总览图

```mermaid
flowchart TB
    API[HTTP API :8080] --> Static[GET / 和前端 assets]
    API --> Session[GET /api/session]
    API --> Login[POST /api/login]
    API --> Logout[POST /api/logout]
    API --> Password[POST /api/password]
    API --> Query[GET /api/query]
    API --> Health[GET /api/health-dashboard]
    API --> Progress[GET /api/ingest-progress]
    API --> SettingsGet[GET /api/settings]
    API --> SettingsPost[POST /api/settings]
    API --> Sync[POST /api/sync]
    API --> Rebuild[POST /api/rebuild]
    API --> UpgradeCheck[GET /api/upgrade/check]
    API --> UpgradeStatus[GET /api/upgrade/status]
    API --> UpgradeRun[POST /api/upgrade/run]
    API --> IPReload[POST /api/ip-data/reload]
    API --> Export[POST /api/export<br/>当前占位 501]
```

## 17. CI 与 142 部署流程图

```mermaid
flowchart LR
    Dev[本地代码修改] --> Test[本地 go test / web build]
    Test --> Binary[构建 Linux amd64 二进制<br/>dist/nat-query-service-linux-amd64]
    Binary --> Upload[上传到 142<br/>/tmp/nat-query-service-new]
    Upload --> Backup[备份旧二进制<br/>/opt/nat-query/nat-query-service.bak.*]
    Backup --> Stop[systemctl stop nat-query-service]
    Stop --> Install[install 新二进制到 /opt/nat-query/nat-query-service]
    Install --> Start[systemctl start nat-query-service]
    Start --> Verify[验证 /api/session 和首页 assets hash]

    Dev --> GitHub[GitHub 仓库]
    GitHub --> CI[GitHub Actions<br/>测试和构建]
    CI --> Artifacts[CI artifacts / Release assets]
```

## 18. 核心数据流总图

```mermaid
flowchart LR
    RawLogs[/归档 NAT 日志/] --> Scanner[文件扫描器]
    Scanner --> DateQueue[日期集合]
    DateQueue --> Importer[Importer.ImportDate]
    Importer --> Parser[ParseNATLine]
    Parser --> Batch[ClickHouse PrepareBatch]
    Batch --> NatLogs[(nat_logs)]

    Importer --> IngestFiles[(ingest_files)]
    Importer --> IngestDates[(ingest_dates)]

    Browser[浏览器页面] --> QueryAPI[/api/query]
    QueryAPI --> IngestDates
    QueryAPI --> NatLogs
    QueryAPI --> IPEngine[IP 标注引擎]
    IPEngine --> Browser

    Browser --> HealthAPI[/api/health-dashboard]
    HealthAPI --> IngestDates
    HealthAPI --> NatLogs

    Browser --> SettingsAPI[/api/settings]
    SettingsAPI --> AppSettings[(app_settings)]
```

## 19. v1.1.0 查询优化补充

`/api/query` 在保留旧 `page` / `page_size` 参数的基础上新增 `cursor` 参数。查询结果新增 `next_cursor` 和 `has_more`，前端日志查询页通过游标栈实现上一页 / 下一页，避免持续翻页时产生深 `OFFSET`。

游标使用 base64url JSON 编码，解码后的结构如下：

```json
{
  "timestamp": "2026-07-04 12:00:00",
  "source_id": "default",
  "source_file": "fw.log-20260704",
  "source_offset": 123456
}
```

游标查询会追加稳定下一页谓词：

```sql
AND (
  timestamp < ?
  OR (
    timestamp = ?
    AND (source_id, source_file, source_offset) < (?, ?, ?)
  )
)
ORDER BY timestamp DESC, source_id DESC, source_file DESC, source_offset DESC
LIMIT page_size + 1
```

查询保护规则：

- 无筛选条件时最大查询跨度为 24 小时。
- 带 IP、端口、协议、动作或日志标识筛选时最大查询跨度为 7 天。
- 旧分页 `page > 20` 会返回 `query_too_broad`，提示改用游标或缩小范围。
- 查询超时为 10 秒，返回 `query_timeout`。
- 全局并发查询闸门为 4，超过时返回 `query_busy`。

第二到第五优先级仍保持规划状态：物化视图预聚合看板、`dst_ip` / `nat_ip` skip index、异步导出、Dictionary / Projection / 冷热分层，均需要先在真实查询与 142 资源占用下验证收益，再进入生产变更。

## 20. 自动升级补充

系统设置页维护标签新增 Release 升级卡片。该功能当前只支持人工触发，不做定时自动升级，也不绕过登录态。

后端新增接口：

- `GET /api/upgrade/check`：检查 GitHub 最新 Release，确认 `nat-query-service_linux_amd64`、`nat-query-service.service`、`deploy-142-from-release.sh` 三个 Linux 发布资产是否齐全。
- `GET /api/upgrade/status`：返回当前进程内升级状态，状态包括 `idle`、`running`、`succeeded`、`failed`。
- `POST /api/upgrade/run`：接收明确版本号，例如 `{"version":"v1.1.0"}`，后台下载 Linux amd64 二进制、备份当前 `/opt/nat-query/nat-query-service`、替换文件并执行 `systemctl restart nat-query-service`。

发布构建会通过 `-ldflags "-X main.appVersion=$version"` 注入当前版本号，供升级页显示。升级失败时记录错误和备份路径，不自动回滚。
