import React from 'react';
import { CalendarOutlined, CheckCircleOutlined, DatabaseOutlined, FileZipOutlined, ReloadOutlined, SyncOutlined } from '@ant-design/icons';
import { Button, Progress, Space, Switch, Tag, Typography, message } from 'antd';
import { apiGet, buildQueryString } from '../api';
import { buildIngestProgressView } from '../ingestPresentation';
import { ingestStatusText } from '../uiCopy';

const { Text } = Typography;

type DateState = {
  source_id?: string;
  log_date?: string;
  status?: string;
  files_total?: number;
  files_done?: number;
  rows_imported?: number;
  bytes_total?: number;
  bytes_done?: number;
  progress_pct?: number;
  current_file?: string;
  error?: string;
  updated_at?: string;
  auto_scan_policy?: string;
  auto_scan_enabled?: boolean;
  auto_scan_mode?: string;
  next_auto_scan_at?: string;
};

type ProgressResponse = DateState & {
  source_id?: string;
  log_tag?: string;
  current_date?: string;
  dates: DateState[];
  sources?: DateState[];
};

function formatCount(value?: number) {
  return new Intl.NumberFormat('zh-CN').format(value ?? 0);
}

function formatLogDate(value?: string) {
  if (!value) return '-';
  return value.slice(0, 10);
}

function progressStatus(status?: string) {
  if (status === 'failed') return 'exception';
  if (status === 'ready') return 'success';
  return 'active';
}

export function IncrementalProgressPage() {
  const [includeReady, setIncludeReady] = React.useState(false);
  const [loading, setLoading] = React.useState(false);
  const [data, setData] = React.useState<ProgressResponse | null>(null);

  const load = React.useCallback(async () => {
    try {
      setLoading(true);
      setData(await apiGet<ProgressResponse>(`/api/ingest-progress${buildQueryString({ range: 'all', include_ready: includeReady })}`));
    } catch (error) {
      message.error(error instanceof Error ? error.message : '加载入库进度失败');
    } finally {
      setLoading(false);
    }
  }, [includeReady]);

  React.useEffect(() => {
    void load();
    const anyImporting = data?.sources?.some((source) => source.status === 'importing') || data?.status === 'importing';
    const timer = window.setInterval(() => void load(), anyImporting ? 5000 : 30000);
    return () => window.clearInterval(timer);
  }, [load, data?.status, data?.sources]);

  const currentProgressView = buildIngestProgressView(data);
  const autoScanEnabled = data?.auto_scan_enabled === true;
  const nextScanLabel = autoScanEnabled ? '下次扫描' : '自动扫描';
  const nextScanText = autoScanEnabled ? data?.next_auto_scan_at || '计算中' : '未启用';
  const scanPolicyLabel = autoScanEnabled ? '扫描策略' : '触发方式';
  const scanPolicyText = autoScanEnabled ? data?.auto_scan_policy || '配置待完善' : '仅手动触发';

  return (
    <div className="page-stack">
      <section className="page-header">
        <div>
          <span className="eyebrow">日志处理状态</span>
          <h1>入库进度</h1>
        </div>
        <Space>
          <Switch checked={includeReady} onChange={setIncludeReady} checkedChildren="显示已完成" unCheckedChildren="仅未完成" />
          <Button icon={<ReloadOutlined />} loading={loading} onClick={() => void load()} />
        </Space>
      </section>

      <section className="ops-section progress-current-card">
        <div className="section-head">
          <h3>当前任务</h3>
          <Tag color={data?.status === 'failed' ? 'error' : data?.status === 'importing' ? 'processing' : 'default'}>{ingestStatusText(data?.status)}</Tag>
        </div>
        <div className="progress-summary-grid">
          <div className="progress-summary-item">
            <span className="status-icon"><DatabaseOutlined /></span>
            <Text type="secondary">日志来源</Text>
            <strong>{data?.log_tag || '-'}</strong>
          </div>
          <div className="progress-summary-item">
            <span className="status-icon"><CalendarOutlined /></span>
            <Text type="secondary">处理日期</Text>
            <strong>{data?.current_date || data?.log_date || '-'}</strong>
          </div>
          <div className="progress-summary-item">
            <span className="status-icon"><FileZipOutlined /></span>
            <Text type="secondary">处理文件</Text>
            <strong>{data?.current_file || '-'}</strong>
          </div>
          <div className="progress-summary-item">
            <span className="status-icon"><SyncOutlined /></span>
            <Text type="secondary">已入库行数</Text>
            <strong>{formatCount(data?.rows_imported)}</strong>
          </div>
          <div className="progress-summary-item">
            <span className="status-icon"><SyncOutlined /></span>
            <Text type="secondary">{nextScanLabel}</Text>
            <strong>{nextScanText}</strong>
          </div>
          <div className="progress-summary-item">
            <span className="status-icon"><CalendarOutlined /></span>
            <Text type="secondary">{scanPolicyLabel}</Text>
            <strong>{scanPolicyText}</strong>
          </div>
        </div>
        <Progress
          percent={currentProgressView.displayPercent}
          format={() => currentProgressView.percentText}
          status={progressStatus(data?.status)}
        />
      </section>

      <section className="ops-section progress-list-card">
        <div className="section-head">
          <div>
            <h3>按日期查看进度</h3>
            <span>查看每个日志日期的文件处理结果</span>
          </div>
        </div>
        <div className="progress-list">
          <div className="progress-list-row progress-list-head">
            <span>来源标识</span>
            <span>日期</span>
            <span>状态</span>
            <span>文件</span>
            <span>行数</span>
            <span>进度</span>
            <span>当前文件 / 错误信息</span>
          </div>
          {(data?.dates || []).map((row) => {
            const rowProgressView = buildIngestProgressView(row);
            return (
              <div className="progress-list-row" key={`${row.source_id || 'default'}-${row.log_date}`}>
                <strong>{row.source_id || 'default'}</strong>
                <strong>{formatLogDate(row.log_date)}</strong>
                <Tag color={row.status === 'failed' ? 'error' : row.status === 'ready' ? 'success' : 'processing'}>{ingestStatusText(row.status)}</Tag>
                <span className="mono-number">{row.files_done ?? 0}/{row.files_total ?? 0}</span>
                <span className="mono-number">{formatCount(row.rows_imported)}</span>
                <div className="progress-inline">
                  <Progress
                    percent={rowProgressView.displayPercent}
                    format={() => rowProgressView.percentText}
                    size="small"
                    status={progressStatus(row.status)}
                  />
                </div>
                <span className={row.error ? 'progress-error-text' : 'progress-muted-text'}>
                  {row.error || row.current_file || '-'}
                </span>
                {row.status === 'ready' ? <CheckCircleOutlined className="progress-ready-icon" /> : null}
              </div>
            );
          })}
        </div>
      </section>
    </div>
  );
}
