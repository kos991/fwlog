import React from 'react';
import {
  CalendarOutlined,
  ClockCircleOutlined,
  CloudUploadOutlined,
  DashboardOutlined,
  DatabaseOutlined,
  FieldTimeOutlined,
  FileZipOutlined,
  HddOutlined,
  InboxOutlined,
  SyncOutlined,
} from '@ant-design/icons';
import { Progress, Segmented, Select, Tag, message } from 'antd';
import { apiGet, buildQueryString, type DistributionItem } from '../api';
import { buildIngestProgressView } from '../ingestPresentation';
import { ingestStatusText } from '../uiCopy';

type LogTrendPoint = {
  date: string;
  source_id: string;
  log_tag: string;
  value: number;
};

type HealthDashboardResponse = {
  data_health: {
    total_logs: number;
    ready_dates: number;
    pending_dates: number;
    importing_dates: number;
    failed_dates: number;
    queryable_start_date: string;
    queryable_end_date: string;
    last_successful_ingest_time: string;
    clickhouse_disk_used_bytes: number;
    today_rows: number;
    yesterday_rows: number;
  };
  ingest_health: {
    status: string;
    source_id: string;
    log_tag: string;
    current_date: string;
    current_file: string;
    files_total: number;
    files_done: number;
    bytes_total: number;
    bytes_done: number;
    rows_imported: number;
    progress_pct: number;
    error: string;
    next_auto_scan_at: string;
    auto_scan_policy?: string;
    auto_scan_enabled?: boolean;
    auto_scan_mode?: string;
    last_updated_at: string;
    last_successful_ingest_at: string;
    elapsed_sec: number;
    eta_sec: number;
    sources?: Array<{ source_id: string; status: string; progress_pct: number }>;
  };
  system_health?: {
    cpu: {
      status: string;
      load_percent: number;
      load_average: number;
      cores: number;
      description: string;
    };
    memory: {
      status: string;
      total_bytes: number;
      available_bytes: number;
      used_percent: number;
      description: string;
    };
    database: {
      status: string;
      version: string;
      active_queries: number;
      active_merges: number;
      active_parts: number;
      total_rows: number;
      disk_used_bytes: number;
      description: string;
    };
  };
  log_trend?: LogTrendPoint[];
  ip_distribution: {
    top_source_ips: DistributionItem[];
    top_destination_ips: DistributionItem[];
    top_nat_ips?: DistributionItem[];
    address_type_shares: DistributionItem[];
    log_tag_distribution: DistributionItem[];
  };
  geo_distribution: {
    top_countries: DistributionItem[];
    top_regions: DistributionItem[];
    unrecognized_ip_rate: number;
    geoip_loaded: boolean;
    geoip_status: string;
  };
};

type HealthDashboardProps = {
  onOpenProgress: () => void;
};

type RankingKey = 'source' | 'destination' | 'country';

type RankingRow = DistributionItem & {
  rank: number;
};

function formatCount(value?: number) {
  return new Intl.NumberFormat('zh-CN').format(value ?? 0);
}

function formatTrendAxisValue(value: number) {
  return formatCount(value);
}

function formatBytes(bytes?: number) {
  if (!bytes) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let size = bytes;
  let unit = 0;
  while (size >= 1024 && unit < units.length - 1) {
    size /= 1024;
    unit += 1;
  }
  return `${size.toFixed(size >= 100 ? 0 : 1)} ${units[unit]}`;
}

function formatPercent(value?: number) {
  if (value === undefined || Number.isNaN(value)) return '-';
  const percent = clampMetric(value);
  return `${percent.toFixed(percent >= 10 ? 0 : 1)}%`;
}

function healthTone(status?: string) {
  if (status === 'critical') return 'critical';
  if (status === 'warning' || status === 'busy') return 'warning';
  if (status === 'ok') return 'ok';
  return 'unknown';
}

function clampMetric(value: number) {
  if (Number.isNaN(value)) return 0;
  return Math.max(0, Math.min(100, value));
}

function statusLineValues(value: number) {
  const current = clampMetric(value);
  const low = Math.max(2, current * 0.14);
  const mid = Math.max(4, current * 0.38);
  const peak = Math.max(12, current);
  return [low, low + 1, low, mid, low + 2, low + 1, peak, low + 3, low + 1, low + 2, low, low + 4];
}

function databaseStatusScore(status?: string) {
  if (status === 'critical') return 88;
  if (status === 'warning' || status === 'busy') return 58;
  if (status === 'ok') return 24;
  return 12;
}

function MiniStatusLine({ values }: { values: number[] }) {
  const width = 118;
  const height = 30;
  const max = Math.max(...values, 100);
  const points = values.map((value, index) => {
    const x = (index / Math.max(values.length - 1, 1)) * width;
    const y = height - 2 - (clampMetric(value) / max) * (height - 4);
    return `${x.toFixed(1)},${y.toFixed(1)}`;
  });

  return (
    <svg className="system-health-line" viewBox={`0 0 ${width} ${height}`} preserveAspectRatio="none" aria-hidden="true">
      <polyline points={points.join(' ')} />
    </svg>
  );
}

function SystemStatusStrip({ health }: { health?: HealthDashboardResponse['system_health'] }) {
  const cpu = health?.cpu;
  const memory = health?.memory;
  const database = health?.database;
  const items = [
    {
      key: 'cpu',
      icon: <DashboardOutlined />,
      label: '处理器',
      value: formatPercent(cpu?.load_percent),
      meta: cpu?.cores ? `${cpu.cores} 核 · ${cpu.description || '负载采集中'}` : cpu?.description || '负载采集中',
      tone: healthTone(cpu?.status),
      lineValues: statusLineValues(cpu?.load_percent ?? 0),
    },
    {
      key: 'memory',
      icon: <HddOutlined />,
      label: '内存',
      value: formatPercent(memory?.used_percent),
      meta: memory?.total_bytes ? `可用 ${formatBytes(memory.available_bytes)} / ${formatBytes(memory.total_bytes)}` : memory?.description || '内存采集中',
      tone: healthTone(memory?.status),
      lineValues: statusLineValues(memory?.used_percent ?? 0),
    },
    {
      key: 'database',
      icon: <DatabaseOutlined />,
      label: '数据库',
      value: database?.status === 'busy' ? 'BUSY' : database?.status === 'ok' ? 'OK' : 'N/A',
      meta: database?.version ? `CH ${database.version} · ${database.active_parts ?? 0} parts` : database?.description || '连接检查中',
      tone: healthTone(database?.status),
      lineValues: statusLineValues(databaseStatusScore(database?.status)),
    },
  ];

  return (
    <div className="system-health-strip" aria-label="系统资源状态">
      {items.map((item) => (
        <article className={`system-health-card system-health-${item.tone}`} key={item.key} title={`${item.label} ${item.value}，${item.meta}`}>
          <div className="system-health-card-head">
            <span className="system-health-icon">{item.icon}</span>
            <span className="system-health-label">{item.label}</span>
            <strong>{item.value}</strong>
          </div>
          <MiniStatusLine values={item.lineValues} />
        </article>
      ))}
    </div>
  );
}

function Sparkline({ values, className = '' }: { values: number[]; className?: string }) {
  const max = Math.max(...values, 1);
  const min = Math.min(...values);
  const range = Math.max(max - min, 1);
  const points = values.map((value, index) => {
    const x = (index / Math.max(values.length - 1, 1)) * 100;
    const y = 32 - ((value - min) / range) * 28;
    return `${x},${y}`;
  });
  return (
    <svg className={`sparkline ${className}`} viewBox="0 0 100 36" preserveAspectRatio="none" aria-hidden="true">
      <path className="sparkline-area" d={`M0 36 L${points.join(' L')} L100 36 Z`} />
      <polyline className="sparkline-line" points={points.join(' ')} />
    </svg>
  );
}

function MetricCard(props: {
  icon: React.ReactNode;
  label: string;
  value: React.ReactNode;
  meta: string;
  tone: 'blue' | 'green' | 'amber' | 'cyan';
  bars: number[];
}) {
  return (
    <article className={`metric-card metric-card-${props.tone}`}>
      <div className="metric-label-row">
        <span className="metric-icon">{props.icon}</span>
        <span className="metric-label">{props.label}</span>
      </div>
      <div className="metric-value">{props.value}</div>
      <div className="metric-meta">{props.meta}</div>
      <Sparkline values={props.bars} className="metric-sparkline" />
    </article>
  );
}

function DashboardFlowArt() {
  return (
    <svg className="dashboard-flow-art" viewBox="0 0 360 150" role="img" aria-label="日志流转状态">
      <defs>
        <linearGradient id="flowLine" x1="0" y1="0" x2="1" y2="0">
          <stop offset="0%" stopColor="#9cc9ff" stopOpacity="0.12" />
          <stop offset="45%" stopColor="#2563eb" stopOpacity="0.78" />
          <stop offset="100%" stopColor="#64748b" stopOpacity="0.18" />
        </linearGradient>
        <linearGradient id="flowNode" x1="0" y1="0" x2="1" y2="1">
          <stop offset="0%" stopColor="#ffffff" />
          <stop offset="100%" stopColor="#dbeafe" />
        </linearGradient>
      </defs>
      <path className="flow-line flow-line-a" d="M26 76 C82 18 132 116 188 58 S280 34 334 76" />
      <path className="flow-line flow-line-b" d="M28 104 C92 72 132 92 178 104 S270 138 334 96" />
      <g className="flow-node flow-node-a">
        <rect x="24" y="58" width="46" height="36" rx="10" />
        <path d="M38 70h18M38 80h24" />
      </g>
      <g className="flow-node flow-node-b">
        <rect x="154" y="38" width="52" height="42" rx="12" />
        <path d="M170 54h20M170 64h14" />
      </g>
      <g className="flow-node flow-node-c">
        <rect x="286" y="58" width="48" height="38" rx="11" />
        <path d="M301 72h17M301 82h22" />
      </g>
      <circle className="flow-dot flow-dot-a" r="5" />
      <circle className="flow-dot flow-dot-b" r="4" />
    </svg>
  );
}

type TrendSourceOption = {
  label: string;
  value: string;
};

type TrendSeries = {
  labels: string[];
  values: number[];
};

const allTrendSourcesValue = '__all__';

function parseDateKey(date: string) {
  const [year, month, day] = date.split('-').map((part) => Number(part));
  if (!year || !month || !day) {
    return null;
  }
  return new Date(year, month - 1, day);
}

function toDateKey(date: Date) {
  const year = date.getFullYear();
  const month = `${date.getMonth() + 1}`.padStart(2, '0');
  const day = `${date.getDate()}`.padStart(2, '0');
  return `${year}-${month}-${day}`;
}

export function formatTrendDateLabel(date: string) {
  const parsed = parseDateKey(date);
  if (!parsed) {
    return date;
  }
  return `${`${parsed.getMonth() + 1}`.padStart(2, '0')}-${`${parsed.getDate()}`.padStart(2, '0')}`;
}

export function recentDateKeys(days = 14, now = new Date()) {
  const safeDays = Math.max(1, Math.floor(days));
  const end = new Date(now.getFullYear(), now.getMonth(), now.getDate());
  const start = new Date(end);
  start.setDate(start.getDate() - (safeDays - 1));

  return Array.from({ length: safeDays }, (_, index) => {
    const date = new Date(start);
    date.setDate(start.getDate() + index);
    return toDateKey(date);
  });
}

export function buildTrendSeries(points: LogTrendPoint[] | undefined, selectedSource: string): TrendSeries {
  const sourceScopedPoints = selectedSource === allTrendSourcesValue
    ? points || []
    : (points || []).filter((point) => point.source_id === selectedSource);
  const valuesByDate = new Map<string, number>();
  for (const point of sourceScopedPoints) {
    valuesByDate.set(point.date, (valuesByDate.get(point.date) || 0) + point.value);
  }
  const labels = recentDateKeys();
  return {
    labels,
    values: labels.map((label) => valuesByDate.get(label) || 0),
  };
}

function buildTrendSourceOptions(points: LogTrendPoint[] | undefined): TrendSourceOption[] {
  const sourceLabels = new Map<string, string>();
  for (const point of points || []) {
    if (!point.source_id) {
      continue;
    }
    sourceLabels.set(point.source_id, point.log_tag ? `${point.log_tag}（${point.source_id}）` : point.source_id);
  }
  return [
    { label: '全部设备', value: allTrendSourcesValue },
    ...Array.from(sourceLabels.entries())
      .sort(([left], [right]) => left.localeCompare(right, 'zh-CN'))
      .map(([value, label]) => ({ label, value })),
  ];
}

function TrafficTrendPanel({
  labels,
  values,
  sourceOptions,
  selectedSource,
  onSourceChange,
}: {
  labels: string[];
  values: number[];
  sourceOptions: TrendSourceOption[];
  selectedSource: string;
  onSourceChange: (value: string) => void;
}) {
  const rawMax = Math.max(...values, 0);
  const yTicks = buildCountTicks(rawMax);
  const chartMax = Math.max(yTicks[yTicks.length - 1] ?? 0, 1);
  const width = 1000;
  const height = 320;
  const padding = { top: 22, right: 24, bottom: 34, left: 72 };
  const innerWidth = width - padding.left - padding.right;
  const innerHeight = height - padding.top - padding.bottom;
  const chartPoints = values.map((value, index) => {
    const x = padding.left + (index / Math.max(values.length - 1, 1)) * innerWidth;
    const y = padding.top + innerHeight - (value / chartMax) * innerHeight;
    return { x, y, value };
  });
  const points = chartPoints.map((point) => `${point.x},${point.y}`);
  const areaPath = `M${padding.left},${height - padding.bottom} L${points.join(' L')} L${width - padding.right},${height - padding.bottom} Z`;
  const xLabels = labels.map(formatTrendDateLabel);
  const selectedSourceLabel = sourceOptions.find((option) => option.value === selectedSource)?.label || selectedSource;
  const markerPoint = [...chartPoints].reverse().find((point) => point.value > 0) || chartPoints[chartPoints.length - 1];
  const markerOnRight = Boolean(markerPoint && markerPoint.x > width * 0.68);

  return (
    <section className="traffic-trend-section">
      <div className="section-head trend-head">
        <div>
          <h3>日志趋势</h3>
          <span>按日期统计入库日志量</span>
        </div>
        <div className="trend-toolbar">
          <Select
            aria-label="趋势设备筛选"
            size="small"
            value={selectedSource}
            options={sourceOptions}
            onChange={onSourceChange}
          />
          <span className="trend-range">最近 14 天</span>
        </div>
      </div>
      <div className="trend-chart-wrap">
        <div className="trend-legend">
          <span className="legend-dot legend-dot-queried" />
          <span>日志量</span>
          <strong>{formatCount(values.reduce((total, value) => total + value, 0))}</strong>
        </div>
        <svg className="trend-chart" viewBox={`0 0 ${width} ${height}`} role="img" aria-label="最近日期日志趋势">
          {yTicks.map((tick) => {
            const y = padding.top + innerHeight - (tick / chartMax) * innerHeight;
            return (
              <g key={tick}>
                <line className="trend-grid-line" x1={padding.left} x2={width - padding.right} y1={y} y2={y} />
                <text className="trend-y-label" x={padding.left - 12} y={y + 4}>{formatTrendAxisValue(tick)}</text>
              </g>
            );
          })}
          {xLabels.map((label, index) => {
            const x = padding.left + (index / Math.max(xLabels.length - 1, 1)) * innerWidth;
            return (
              <text className="trend-x-label" key={label} x={x} y={height - 8}>{label}</text>
            );
          })}
          <path className="trend-area" d={areaPath} />
          <polyline className="trend-line" points={points.join(' ')} />
          {markerPoint ? (
            <g className="trend-source-marker">
              <circle cx={markerPoint.x} cy={markerPoint.y} r="4" />
              <text
                x={markerPoint.x + (markerOnRight ? -10 : 10)}
                y={markerPoint.y < 42 ? markerPoint.y + 20 : markerPoint.y - 12}
                textAnchor={markerOnRight ? 'end' : 'start'}
              >
                {selectedSourceLabel}
              </text>
              <title>{selectedSourceLabel}</title>
            </g>
          ) : null}
        </svg>
      </div>
    </section>
  );
}

export function buildCountTicks(maxValue: number): number[] {
  if (!Number.isFinite(maxValue) || maxValue <= 0) {
    return [0];
  }
  if (maxValue <= 4) {
    return Array.from({ length: Math.ceil(maxValue) + 1 }, (_, index) => index);
  }
  const step = Math.max(1, Math.ceil(maxValue / 4));
  const chartMax = Math.ceil(maxValue / step) * step;
  return Array.from({ length: Math.floor(chartMax / step) + 1 }, (_, index) => index * step);
}

function CompactRankingPanel(props: {
  active: RankingKey;
  onChange: (key: RankingKey) => void;
  rows: RankingRow[];
}) {
  const max = Math.max(...props.rows.map((item) => item.value), 1);
  return (
    <section className="traffic-rank-section">
      <div className="section-head rank-head">
        <div>
          <h3>流量排行</h3>
          <span>{props.active === 'country' ? '按目标 IP 归属地统计' : '当前维度 Top 8'}</span>
        </div>
        <Segmented
          value={props.active}
          onChange={(value) => props.onChange(value as RankingKey)}
          options={[
            { label: '源', value: 'source' },
            { label: '目标', value: 'destination' },
            { label: '目标地区', value: 'country' },
          ]}
        />
      </div>
      <div className="compact-rank-list">
        {props.rows.length === 0 ? (
          <div className="compact-rank-empty">
            <InboxOutlined />
            <strong>暂无排行数据</strong>
            <span>该维度当前没有可展示的统计结果</span>
          </div>
        ) : props.rows.slice(0, 8).map((row) => {
          const percent = Math.round((row.value / max) * 100);
          return (
            <div className="compact-rank-row" key={`${props.active}-${row.name}`}>
              <span className="compact-rank-index">{row.rank}</span>
              <strong>{row.name}</strong>
              <span className="compact-rank-value">{formatCount(row.value)}</span>
              <span className="compact-rank-track">
                <span style={{ width: `${percent}%` }} />
              </span>
            </div>
          );
        })}
      </div>
    </section>
  );
}

function hasDistributionPayload(data: HealthDashboardResponse) {
  return Boolean(
    data.ip_distribution?.top_source_ips?.length ||
      data.ip_distribution?.top_destination_ips?.length ||
      data.geo_distribution?.top_countries?.length,
  );
}

function mergeDashboardPayload(
  previous: HealthDashboardResponse | null,
  next: HealthDashboardResponse,
): HealthDashboardResponse {
  if (!previous || hasDistributionPayload(next)) {
    return next;
  }
  return {
    ...next,
    ip_distribution: previous.ip_distribution,
    geo_distribution: previous.geo_distribution,
  };
}

export function HealthDashboard(_props: HealthDashboardProps) {
  const [, setLoading] = React.useState(false);
  const [data, setData] = React.useState<HealthDashboardResponse | null>(null);
  const [rankingKey, setRankingKey] = React.useState<RankingKey>('source');
  const [selectedTrendSource, setSelectedTrendSource] = React.useState(allTrendSourcesValue);

  const loadSummary = React.useCallback(async () => {
    try {
      setLoading(true);
      const payload = await apiGet<HealthDashboardResponse>(
        '/api/health-dashboard?range=all&include_distributions=false',
      );
      setData((previous) => mergeDashboardPayload(previous, payload));
    } catch (error) {
      message.error(error instanceof Error ? error.message : '加载数据概览失败');
    } finally {
      setLoading(false);
    }
  }, []);

  const loadRankings = React.useCallback(async () => {
    try {
      const payload = await apiGet<HealthDashboardResponse>(
        `/api/health-dashboard${buildQueryString({
          range: 'all',
          metrics_range: '30d',
          include_distributions: true,
          source_id: selectedTrendSource === allTrendSourcesValue ? undefined : selectedTrendSource,
        })}`,
      );
      setData(payload);
    } catch (error) {
      message.error(error instanceof Error ? error.message : '加载流量排行失败');
    }
  }, [selectedTrendSource]);

  React.useEffect(() => {
    void loadSummary();
    void loadRankings();
    const rankingTimer = window.setInterval(() => void loadRankings(), 300000);
    return () => {
      window.clearInterval(rankingTimer);
    };
  }, [loadSummary, loadRankings]);

  React.useEffect(() => {
    const summaryTimer = window.setInterval(
      () => void loadSummary(),
      data?.ingest_health?.sources?.some((source) => source.status === 'importing') || data?.ingest_health?.status === 'importing' ? 5000 : 30000,
    );
    return () => window.clearInterval(summaryTimer);
  }, [loadSummary, data?.ingest_health?.status, data?.ingest_health?.sources]);

  const health = data?.data_health;
  const ingest = data?.ingest_health;
  const queryRange = `${health?.queryable_start_date || '-'} ~ ${health?.queryable_end_date || '-'}`;
  const ingestView = buildIngestProgressView(ingest);
  const nextScanText = ingest?.auto_scan_enabled ? ingest?.next_auto_scan_at || '计算中' : '未启用';
  const scanPolicyText = ingest?.auto_scan_policy || (ingest?.auto_scan_enabled ? '配置待完善' : '未启用');
  const autoScanValue = ingest?.auto_scan_enabled ? scanPolicyText : '未启用';
  const autoScanMeta = ingest?.auto_scan_enabled ? `下次 ${nextScanText}` : '不会自动触发';
  const trendSourceOptions = React.useMemo(() => buildTrendSourceOptions(data?.log_trend), [data?.log_trend]);
  React.useEffect(() => {
    if (!trendSourceOptions.some((option) => option.value === selectedTrendSource)) {
      setSelectedTrendSource(allTrendSourcesValue);
    }
  }, [selectedTrendSource, trendSourceOptions]);
  const trendSeries = React.useMemo(() => {
    return buildTrendSeries(data?.log_trend, selectedTrendSource);
  }, [data?.log_trend, selectedTrendSource]);
  const rankingRows = React.useMemo(() => {
    const sourceMap: Record<RankingKey, DistributionItem[] | undefined> = {
      source: data?.ip_distribution?.top_source_ips,
      destination: data?.ip_distribution?.top_destination_ips,
      country: data?.geo_distribution?.top_countries,
    };
    return (sourceMap[rankingKey] || []).map((item, index) => ({ ...item, rank: index + 1 }));
  }, [data, rankingKey]);

  return (
    <div className="page-stack">
      <section className="page-header">
        <div className="dashboard-title-block">
          <span className="eyebrow">系统状态</span>
          <h1>数据概览</h1>
          <span>查看日志数据范围、入库进度和 NAT 流量分布</span>
        </div>
        <SystemStatusStrip health={data?.system_health} />
      </section>

      <section className="metric-grid">
        <MetricCard
          icon={<DatabaseOutlined />}
          label="已入库日志"
          value={formatCount(health?.total_logs)}
          meta={`覆盖 ${health?.ready_dates ?? 0} 个日志日期`}
          tone="blue"
          bars={[42, 58, 64, 71, 86, 78, 92]}
        />
        <MetricCard
          icon={<CalendarOutlined />}
          label="可查询日期范围"
          value={queryRange}
          meta={`等待处理 ${health?.pending_dates ?? 0} 天 · 正在入库 ${health?.importing_dates ?? 0} 天`}
          tone="green"
          bars={[30, 42, 57, 63, 68, 78, 88]}
        />
        <MetricCard
          icon={<CloudUploadOutlined />}
          label="今日新增日志"
          value={formatCount(health?.today_rows)}
          meta={`昨日 ${formatCount(health?.yesterday_rows)} 行`}
          tone="amber"
          bars={[22, 35, 51, 45, 66, 74, 62]}
        />
        <MetricCard
          icon={<HddOutlined />}
          label="日志存储占用"
          value={formatBytes(health?.clickhouse_disk_used_bytes)}
          meta="ClickHouse 日志数据"
          tone="cyan"
          bars={[18, 24, 31, 38, 44, 53, 61]}
        />
      </section>

      <section className="ops-section ingest-card">
        <div className="section-head">
          <div>
            <h3>当前入库任务</h3>
          </div>
          <Tag color={ingest?.status === 'failed' ? 'error' : ingest?.status === 'importing' ? 'processing' : 'success'}>
            {ingestStatusText(ingest?.status)}
          </Tag>
        </div>
        <div className="status-grid">
          <div><span className="status-icon"><DatabaseOutlined /></span><span className="status-label">日志来源</span><strong>{ingest?.log_tag || '-'}</strong></div>
          <div><span className="status-icon"><FieldTimeOutlined /></span><span className="status-label">处理日期</span><strong>{ingest?.current_date || '-'}</strong></div>
          <div><span className="status-icon"><FileZipOutlined /></span><span className="status-label">处理文件</span><strong>{ingestView.currentFileText}</strong></div>
          <div className="status-grid-note">
            <span className="status-icon"><ClockCircleOutlined /></span>
            <span className="status-label">自动扫描</span>
            <strong>{autoScanValue}</strong>
            <small>{autoScanMeta}</small>
          </div>
        </div>
        <div className="ingest-progress-line">
          <Progress percent={ingestView.displayPercent} format={() => ingestView.percentText} status={ingest?.status === 'failed' ? 'exception' : 'active'} />
          <div className="ingest-progress-meta">
            <span className="ingest-progress-note">{ingestView.detailText}</span>
            <span className="ingest-progress-item"><FileZipOutlined /><strong className="mono-number">{ingestView.fileProgressText}</strong></span>
            <span className="ingest-progress-item"><CloudUploadOutlined /><strong className="mono-number">{ingestView.rowsText}</strong></span>
            <span className="ingest-progress-item"><HddOutlined /><strong className="mono-number">{ingestView.bytesText}</strong></span>
            <span className="ingest-progress-item"><FieldTimeOutlined />更新 {ingestView.updatedText}</span>
          </div>
        </div>
      </section>

      <section className="ops-section analysis-section">
        <TrafficTrendPanel
          labels={trendSeries.labels}
          values={trendSeries.values}
          sourceOptions={trendSourceOptions}
          selectedSource={selectedTrendSource}
          onSourceChange={setSelectedTrendSource}
        />
        <CompactRankingPanel active={rankingKey} onChange={setRankingKey} rows={rankingRows} />
      </section>
    </div>
  );
}
