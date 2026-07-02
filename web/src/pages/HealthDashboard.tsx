import React from 'react';
import { ReloadOutlined, SyncOutlined } from '@ant-design/icons';
import { ProTable } from '@ant-design/pro-components';
import type { ProColumns } from '@ant-design/pro-components';
import { Button, Segmented, Space, Statistic, Tag, Typography, message } from 'antd';
import { apiGet, buildQueryString } from '../api';

const { Text } = Typography;

type DistributionItem = {
  name: string;
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
    last_auto_scan_at: string;
    next_auto_scan_at: string;
    elapsed_sec: number;
    eta_sec: number;
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

const rangeOptions = [
  { label: '今天', value: 'today' },
  { label: '昨天', value: 'yesterday' },
  { label: '最近 7 天', value: '7d' },
  { label: '最近 30 天', value: '30d' },
  { label: '全部', value: 'all' }
] as const;

function formatCount(value: number) {
  return new Intl.NumberFormat('zh-CN').format(value ?? 0);
}

function formatBytes(bytes: number) {
  if (!bytes) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let size = bytes;
  let unitIndex = 0;
  while (size >= 1024 && unitIndex < units.length - 1) {
    size /= 1024;
    unitIndex += 1;
  }
  return `${size.toFixed(size >= 100 ? 0 : 1)} ${units[unitIndex]}`;
}

function DistributionTable(props: { title: string; data: DistributionItem[] }) {
  const columns: ProColumns<DistributionItem>[] = [
    { title: '项目', dataIndex: 'name', search: false },
    {
      title: '数量',
      dataIndex: 'value',
      search: false,
      width: 140,
      align: 'right',
      renderText: (value) => formatCount(Number(value))
    }
  ];

  return (
    <section className="ops-section">
      <div className="section-head">
        <h3>{props.title}</h3>
      </div>
      <ProTable<DistributionItem>
        rowKey="name"
        columns={columns}
        dataSource={props.data}
        search={false}
        options={false}
        pagination={false}
        size="small"
        cardBordered={false}
      />
    </section>
  );
}

export function HealthDashboard({ onOpenProgress }: HealthDashboardProps) {
  const [range, setRange] = React.useState<(typeof rangeOptions)[number]['value']>('7d');
  const [loading, setLoading] = React.useState(false);
  const [data, setData] = React.useState<HealthDashboardResponse | null>(null);

  const loadData = React.useCallback(async () => {
    setLoading(true);
    try {
      const response = await apiGet<HealthDashboardResponse>(
        `/api/health-dashboard${buildQueryString({ range })}`
      );
      setData(response);
    } catch (error) {
      message.error(error instanceof Error ? error.message : '监控大屏加载失败');
    } finally {
      setLoading(false);
    }
  }, [range]);

  React.useEffect(() => {
    void loadData();
  }, [loadData]);

  const stats = data?.data_health;
  const ingest = data?.ingest_health;

  return (
    <div className="page-stack">
      <section className="ops-section">
        <div className="section-head section-head-row">
          <Space>
            <Segmented
              options={rangeOptions.map((item) => ({ label: item.label, value: item.value }))}
              value={range}
              onChange={(value) => setRange(value as typeof range)}
            />
          </Space>
          <Button icon={<ReloadOutlined />} onClick={() => void loadData()} loading={loading}>
            刷新
          </Button>
        </div>
        <div className="stat-grid">
          <div className="stat-block">
            <Statistic title="总日志量" value={stats?.total_logs ?? 0} formatter={(value) => formatCount(Number(value))} />
            <Text type="secondary">默认最近 7 天 ready 数据</Text>
          </div>
          <div className="stat-block">
            <Statistic title="已入库日期" value={stats?.ready_dates ?? 0} />
            <Text type="secondary">待入库 {stats?.pending_dates ?? 0} / 入库中 {stats?.importing_dates ?? 0} / 失败 {stats?.failed_dates ?? 0}</Text>
          </div>
          <div className="stat-block">
            <Statistic title="ClickHouse 磁盘占用" value={formatBytes(stats?.clickhouse_disk_used_bytes ?? 0)} />
            <Text type="secondary">今日新增 {formatCount(stats?.today_rows ?? 0)} 行，昨日新增 {formatCount(stats?.yesterday_rows ?? 0)} 行</Text>
          </div>
          <div className="stat-block">
            <Statistic title="当前增量状态" value={ingest?.status || 'idle'} />
            <Text type="secondary">
              可查询范围 {stats?.queryable_start_date || '-'} 至 {stats?.queryable_end_date || '-'}
            </Text>
          </div>
        </div>
      </section>

      <div className="two-column-grid">
        <section className="ops-section">
          <div className="section-head">
            <h3>数据健康</h3>
          </div>
          <dl className="kv-grid">
            <div><dt>最近成功入库</dt><dd>{stats?.last_successful_ingest_time || '-'}</dd></div>
            <div><dt>可查询起始日期</dt><dd>{stats?.queryable_start_date || '-'}</dd></div>
            <div><dt>可查询结束日期</dt><dd>{stats?.queryable_end_date || '-'}</dd></div>
            <div><dt>失败日期数</dt><dd>{stats?.failed_dates ?? 0}</dd></div>
          </dl>
        </section>

        <section className="ops-section">
          <div className="section-head section-head-row">
            <h3>导入健康</h3>
            <Button type="link" icon={<SyncOutlined />} onClick={onOpenProgress}>
              查看增量进度
            </Button>
          </div>
          <dl className="kv-grid">
            <div><dt>当前日志源</dt><dd>{ingest?.source_id || '-'}</dd></div>
            <div><dt>当前日期</dt><dd>{ingest?.current_date || '-'}</dd></div>
            <div><dt>当前文件</dt><dd className="mono-cell">{ingest?.current_file || '-'}</dd></div>
            <div><dt>文件进度</dt><dd>{ingest ? `${ingest.files_done}/${ingest.files_total}` : '-'}</dd></div>
            <div><dt>字节进度</dt><dd>{ingest ? `${formatBytes(ingest.bytes_done)} / ${formatBytes(ingest.bytes_total)}` : '-'}</dd></div>
            <div><dt>最近自动增量</dt><dd>{ingest?.last_auto_scan_at || '-'}</dd></div>
            <div><dt>下一次自动增量</dt><dd>{ingest?.next_auto_scan_at || '-'}</dd></div>
            <div>
              <dt>错误</dt>
              <dd>{ingest?.error ? <Tag color="error">{ingest.error}</Tag> : <Tag color="success">无错误</Tag>}</dd>
            </div>
          </dl>
        </section>
      </div>

      <div className="two-column-grid">
        <DistributionTable title="IP 分布 / 源 IP" data={data?.ip_distribution.top_source_ips ?? []} />
        <DistributionTable title="IP 分布 / 目标 IP" data={data?.ip_distribution.top_destination_ips ?? []} />
      </div>

      <div className="two-column-grid">
        <DistributionTable title="IP 分布 / NAT IP" data={data?.ip_distribution.top_nat_ips ?? []} />
        <DistributionTable title="地址类型分布" data={data?.ip_distribution.address_type_shares ?? []} />
      </div>

      <div className="two-column-grid">
        <DistributionTable title="国家分布" data={data?.geo_distribution.top_countries ?? []} />
        <section className="ops-section">
          <div className="section-head">
            <h3>国家地区分布</h3>
          </div>
          <div className="maintenance-grid">
            <div className="stat-block compact">
              <Statistic title="GeoIP 状态" value={data?.geo_distribution.geoip_loaded ? '已加载' : '未加载'} />
              <Text type="secondary">{data?.geo_distribution.geoip_status || '-'}</Text>
            </div>
            <div className="stat-block compact">
              <Statistic title="未识别 IP 占比" value={Number(((data?.geo_distribution.unrecognized_ip_rate ?? 0) * 100).toFixed(2))} suffix="%" />
            </div>
          </div>
          <div className="simple-list">
            {(data?.geo_distribution.top_regions ?? []).map((item) => (
              <div key={item.name} className="simple-list-row">
                <span>{item.name}</span>
                <strong>{formatCount(item.value)}</strong>
              </div>
            ))}
          </div>
        </section>
      </div>
    </div>
  );
}
