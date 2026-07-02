import React from 'react';
import { ReloadOutlined } from '@ant-design/icons';
import { ProTable } from '@ant-design/pro-components';
import type { ProColumns } from '@ant-design/pro-components';
import { Button, Progress, Space, Statistic, Switch, Tag, Typography, message } from 'antd';
import { apiGet, buildQueryString } from '../api';

const { Text } = Typography;

type DateIngestState = {
  source_id: string;
  log_tag: string;
  log_date: string;
  status: string;
  files_total: number;
  files_done: number;
  rows_imported: number;
  bytes_total: number;
  bytes_done: number;
  current_file: string;
  progress_pct: number;
  error: string;
  updated_at: string;
};

type IngestProgressResponse = {
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
  elapsed_sec: number;
  eta_sec: number;
  last_auto_scan_at: string;
  next_auto_scan_at: string;
  error: string;
  dates: DateIngestState[];
};

const statusTextMap: Record<string, string> = {
  ready: '已入库',
  importing: '入库中',
  pending: '待入库',
  failed: '入库失败',
  idle: '空闲',
  scanning: '扫描中',
  succeeded: '已完成'
};

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

function formatDuration(seconds: number) {
  if (!seconds) return '-';
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  const remainSeconds = seconds % 60;
  return `${hours}时 ${minutes}分 ${remainSeconds}秒`;
}

export function IncrementalProgressPage() {
  const [loading, setLoading] = React.useState(false);
  const [includeReady, setIncludeReady] = React.useState(false);
  const [data, setData] = React.useState<IngestProgressResponse | null>(null);

  const loadData = React.useCallback(async () => {
    setLoading(true);
    try {
      const response = await apiGet<IngestProgressResponse>(
        `/api/ingest-progress${buildQueryString({ include_ready: includeReady })}`
      );
      setData(response);
    } catch (error) {
      message.error(error instanceof Error ? error.message : '增量进度加载失败');
    } finally {
      setLoading(false);
    }
  }, [includeReady]);

  React.useEffect(() => {
    void loadData();
  }, [loadData]);

  React.useEffect(() => {
    const interval = window.setInterval(() => {
      void loadData();
    }, data?.status === 'importing' || data?.status === 'scanning' ? 3000 : 30000);

    return () => {
      window.clearInterval(interval);
    };
  }, [data?.status, loadData]);

  const columns: ProColumns<DateIngestState>[] = [
    { title: '日志源', dataIndex: 'source_id', width: 180 },
    { title: '日志标识', dataIndex: 'log_tag', width: 140 },
    { title: '日期', dataIndex: 'log_date', width: 120 },
    {
      title: '状态',
      dataIndex: 'status',
      width: 110,
      render: (_, record) => {
        const text = statusTextMap[record.status] || record.status || '-';
        const color = record.status === 'failed' ? 'error' : record.status === 'importing' ? 'processing' : record.status === 'pending' ? 'warning' : 'success';
        return <Tag color={color}>{text}</Tag>;
      }
    },
    {
      title: '文件进度',
      width: 120,
      render: (_, record) => `${record.files_done}/${record.files_total}`
    },
    {
      title: '字节进度',
      width: 180,
      render: (_, record) => `${formatBytes(record.bytes_done)} / ${formatBytes(record.bytes_total)}`
    },
    {
      title: '行数',
      dataIndex: 'rows_imported',
      width: 120,
      align: 'right',
      renderText: (value) => new Intl.NumberFormat('zh-CN').format(Number(value ?? 0))
    },
    {
      title: '当前文件',
      dataIndex: 'current_file',
      ellipsis: true,
      renderText: (value) => value || '-'
    },
    { title: '最近更新', dataIndex: 'updated_at', width: 180, renderText: (value) => value || '-' },
    {
      title: '错误',
      dataIndex: 'error',
      width: 220,
      ellipsis: true,
      render: (_, record) => record.error ? <Tag color="error">{record.error}</Tag> : '-'
    }
  ];

  return (
    <div className="page-stack">
      <section className="ops-section">
        <div className="section-head section-head-row">
          <h3>当前增量状态</h3>
          <Space>
            <span className="switch-label">显示已入库日期</span>
            <Switch checked={includeReady} onChange={setIncludeReady} />
            <Button icon={<ReloadOutlined />} onClick={() => void loadData()} loading={loading}>
              刷新
            </Button>
          </Space>
        </div>
        <div className="stat-grid">
          <div className="stat-block">
            <Statistic title="状态" value={statusTextMap[data?.status || 'idle'] || data?.status || '空闲'} />
            <Text type="secondary">当前日志源 {data?.source_id || '-'}</Text>
          </div>
          <div className="stat-block">
            <Statistic title="当前日期" value={data?.current_date || '-'} />
            <Text type="secondary">日志标识 {data?.log_tag || '-'}</Text>
          </div>
          <div className="stat-block">
            <Statistic title="文件进度" value={data ? `${data.files_done}/${data.files_total}` : '-'} />
            <Text type="secondary">字节进度 {data ? `${formatBytes(data.bytes_done)} / ${formatBytes(data.bytes_total)}` : '-'}</Text>
          </div>
          <div className="stat-block">
            <Statistic title="已入库行数" value={data?.rows_imported ?? 0} />
            <Text type="secondary">耗时 {formatDuration(data?.elapsed_sec ?? 0)}，预计剩余 {formatDuration(data?.eta_sec ?? 0)}</Text>
          </div>
        </div>
        <div className="progress-summary">
          <div>
            <span>当前文件</span>
            <strong className="mono-cell">{data?.current_file || '-'}</strong>
          </div>
          <div>
            <span>最近自动增量</span>
            <strong>{data?.last_auto_scan_at || '-'}</strong>
          </div>
          <div>
            <span>下一次自动增量</span>
            <strong>{data?.next_auto_scan_at || '-'}</strong>
          </div>
          <div>
            <span>错误</span>
            <strong>{data?.error || '-'}</strong>
          </div>
        </div>
        <Progress percent={Math.round(data?.progress_pct ?? 0)} status={data?.status === 'failed' ? 'exception' : 'active'} />
      </section>

      <section className="ops-section">
        <div className="section-head">
          <h3>日期队列</h3>
        </div>
        <ProTable<DateIngestState>
          rowKey={(record) => `${record.source_id}-${record.log_date}`}
          loading={loading}
          columns={columns}
          dataSource={data?.dates ?? []}
          search={false}
          options={false}
          pagination={false}
          size="small"
          cardBordered={false}
        />
      </section>
    </div>
  );
}
