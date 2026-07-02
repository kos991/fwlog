import React from 'react';
import {
  CalendarOutlined,
  CloudUploadOutlined,
  DatabaseOutlined,
  FieldTimeOutlined,
  FileZipOutlined,
  HddOutlined,
  SyncOutlined,
} from '@ant-design/icons';
import { Progress, Segmented, Tag, message } from 'antd';
import { apiGet, type DistributionItem } from '../api';

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
  };
  ip_distribution: {
    top_source_ips: DistributionItem[];
    top_destination_ips: DistributionItem[];
    top_nat_ips: DistributionItem[];
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

type RankingKey = 'source' | 'destination' | 'nat' | 'country';

type RankingRow = DistributionItem & {
  rank: number;
};

function statusText(status?: string) {
  const map: Record<string, string> = {
    importing: '入库中',
    ready: '已入库',
    failed: '失败',
    idle: '空闲',
  };
  return status ? map[status] || status : '空闲';
}

function formatCount(value?: number) {
  return new Intl.NumberFormat('zh-CN').format(value ?? 0);
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

function TrafficTrendPanel({ values }: { values: number[] }) {
  const max = Math.max(...values, 1);
  const width = 1000;
  const height = 320;
  const padding = { top: 22, right: 24, bottom: 34, left: 44 };
  const innerWidth = width - padding.left - padding.right;
  const innerHeight = height - padding.top - padding.bottom;
  const points = values.map((value, index) => {
    const x = padding.left + (index / Math.max(values.length - 1, 1)) * innerWidth;
    const y = padding.top + innerHeight - (value / max) * innerHeight;
    return `${x},${y}`;
  });
  const areaPath = `M${padding.left},${height - padding.bottom} L${points.join(' L')} L${width - padding.right},${height - padding.bottom} Z`;
  const gridValues = [0, 0.25, 0.5, 0.75, 1];
  const xLabels = ['00:00', '04:00', '08:00', '12:00', '16:00', '20:00'];

  return (
    <section className="traffic-trend-section">
      <div className="section-head trend-head">
        <div>
          <h3>日志趋势</h3>
          <span>过去 24 小时入库日志量</span>
        </div>
        <span className="trend-range">过去 24 小时</span>
      </div>
      <div className="trend-chart-wrap">
        <div className="trend-legend">
          <span className="legend-dot legend-dot-queried" />
          <span>日志量</span>
          <strong>{formatCount(values.reduce((total, value) => total + value, 0))}</strong>
        </div>
        <svg className="trend-chart" viewBox={`0 0 ${width} ${height}`} role="img" aria-label="过去 24 小时日志趋势">
          {gridValues.map((ratio) => {
            const y = padding.top + innerHeight - ratio * innerHeight;
            return (
              <g key={ratio}>
                <line className="trend-grid-line" x1={padding.left} x2={width - padding.right} y1={y} y2={y} />
                <text className="trend-y-label" x={padding.left - 12} y={y + 4}>{Math.round(max * ratio)}</text>
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
        </svg>
      </div>
    </section>
  );
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
          <span>当前维度 Top 8</span>
        </div>
        <Segmented
          value={props.active}
          onChange={(value) => props.onChange(value as RankingKey)}
          options={[
            { label: '源', value: 'source' },
            { label: '目标', value: 'destination' },
            { label: 'NAT', value: 'nat' },
            { label: '地区', value: 'country' },
          ]}
        />
      </div>
      <div className="compact-rank-list">
        {props.rows.slice(0, 8).map((row) => {
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

export function HealthDashboard(_props: HealthDashboardProps) {
  const [, setLoading] = React.useState(false);
  const [data, setData] = React.useState<HealthDashboardResponse | null>(null);
  const [rankingKey, setRankingKey] = React.useState<RankingKey>('source');

  const load = React.useCallback(async () => {
    try {
      setLoading(true);
      setData(await apiGet<HealthDashboardResponse>('/api/health-dashboard'));
    } catch (error) {
      message.error(error instanceof Error ? error.message : '加载数据概览失败');
    } finally {
      setLoading(false);
    }
  }, []);

  React.useEffect(() => {
    void load();
  }, [load]);

  const health = data?.data_health;
  const ingest = data?.ingest_health;
  const queryRange = `${health?.queryable_start_date || '-'} ~ ${health?.queryable_end_date || '-'}`;
  const ingestPercent = Math.round(ingest?.progress_pct ?? 0);
  const trendValues = React.useMemo(
    () => [0, 2, 1, 1, 4, 152, 6, 0, 2, 128, 0, 0, 46, 0, 130, 8, 67, 3, 0, 2, 1, 5, 0, 0, 2, 0, 4, 0, 1, 3, 0, 138, 1, 2, 0, 1, 0, 2, 20, 0, 1, 6, 0, 2, 26, 1],
    [],
  );
  const rankingRows = React.useMemo(() => {
    const sourceMap: Record<RankingKey, DistributionItem[] | undefined> = {
      source: data?.ip_distribution?.top_source_ips,
      destination: data?.ip_distribution?.top_destination_ips,
      nat: data?.ip_distribution?.top_nat_ips,
      country: data?.geo_distribution?.top_countries,
    };
    return (sourceMap[rankingKey] || []).map((item, index) => ({ ...item, rank: index + 1 }));
  }, [data, rankingKey]);

  return (
    <div className="page-stack">
      <section className="page-header">
        <div className="dashboard-title-block">
          <span className="eyebrow">运行概览</span>
          <h1>数据概览</h1>
          <span>入库范围、查询状态和 NAT 流量排行</span>
        </div>
        <DashboardFlowArt />
      </section>

      <section className="metric-grid">
        <MetricCard
          icon={<DatabaseOutlined />}
          label="日志总量"
          value={formatCount(health?.total_logs)}
          meta={`${health?.ready_dates ?? 0} 天已入库`}
          tone="blue"
          bars={[42, 58, 64, 71, 86, 78, 92]}
        />
        <MetricCard
          icon={<CalendarOutlined />}
          label="可查日期"
          value={queryRange}
          meta={`${health?.pending_dates ?? 0} 天待入库，${health?.importing_dates ?? 0} 天入库中`}
          tone="green"
          bars={[30, 42, 57, 63, 68, 78, 88]}
        />
        <MetricCard
          icon={<CloudUploadOutlined />}
          label="今日入库"
          value={formatCount(health?.today_rows)}
          meta={`昨日 ${formatCount(health?.yesterday_rows)} 行`}
          tone="amber"
          bars={[22, 35, 51, 45, 66, 74, 62]}
        />
        <MetricCard
          icon={<HddOutlined />}
          label="存储占用"
          value={formatBytes(health?.clickhouse_disk_used_bytes)}
          meta="MergeTree 数据目录"
          tone="cyan"
          bars={[18, 24, 31, 38, 44, 53, 61]}
        />
      </section>

      <section className="ops-section ingest-card">
        <div className="section-head">
          <div>
            <h3>入库状态</h3>
          </div>
          <Tag color={ingest?.status === 'failed' ? 'error' : ingest?.status === 'importing' ? 'processing' : 'success'}>
            {statusText(ingest?.status)}
          </Tag>
        </div>
        <div className="status-grid">
          <div><span className="status-icon"><DatabaseOutlined /></span><span className="status-label">日志源</span><strong>{ingest?.log_tag || '-'}</strong></div>
          <div><span className="status-icon"><FieldTimeOutlined /></span><span className="status-label">当前日期</span><strong>{ingest?.current_date || '-'}</strong></div>
          <div><span className="status-icon"><FileZipOutlined /></span><span className="status-label">当前文件</span><strong>{ingest?.current_file || '-'}</strong></div>
          <div><span className="status-icon"><SyncOutlined /></span><span className="status-label">下次扫描</span><strong>{ingest?.next_auto_scan_at || '-'}</strong></div>
        </div>
        <div className="ingest-progress-line">
          <Progress percent={ingestPercent} />
        </div>
      </section>

      <section className="ops-section analysis-section">
        <TrafficTrendPanel values={trendValues} />
        <CompactRankingPanel active={rankingKey} onChange={setRankingKey} rows={rankingRows} />
      </section>
    </div>
  );
}
