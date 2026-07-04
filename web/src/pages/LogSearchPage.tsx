import React from 'react';
import { DownOutlined, SearchOutlined, UpOutlined } from '@ant-design/icons';
import { ProTable } from '@ant-design/pro-components';
import type { ProColumns } from '@ant-design/pro-components';
import { Alert, Button, DatePicker, Descriptions, Form, Input, Select, Tag, message } from 'antd';
import dayjs, { type Dayjs } from 'dayjs';
import { apiGet, buildQueryString, type QueryVisibility } from '../api';

const { RangePicker } = DatePicker;
const defaultQueryPageSize = 100;

type SearchRecord = {
  id?: string;
  timestamp?: string;
  log_tag?: string;
  source_id?: string;
  src_ip?: string;
  src_port?: number;
  dst_ip?: string;
  dst_port?: number;
  nat_ip?: string;
  nat_port?: number;
  protocol?: string;
  action?: string;
  src_ip_label?: string;
  dst_geo?: string;
  source_file?: string;
  source_offset?: number;
  log_date?: string;
  ingested_at?: string;
};

type QueryResponse = {
  records: SearchRecord[];
  total: number;
  page: number;
  page_size: number;
  next_cursor?: string;
  has_more?: boolean;
  query_time_ms: number;
  visibility: QueryVisibility;
};

type DateState = {
  log_date?: string;
  status?: string;
  progress_pct?: number;
  rows_imported?: number;
  current_file?: string;
  error?: string;
};

type CidrAliasSetting = {
  cidr?: string;
  alias?: string;
  enabled?: boolean;
};

type SettingsResponse = {
  cidr_aliases?: CidrAliasSetting[] | string;
};

type ProgressResponse = DateState & {
  dates?: DateState[];
  current_date?: string;
};

type LogSearchPageProps = {
  onOpenProgress: () => void;
};

type SearchFormValues = {
  range?: [Dayjs, Dayjs];
  ip?: string;
  src_ip?: string;
  dst_ip?: string;
  nat_ip?: string;
  src_port?: string;
  dst_port?: string;
  nat_port?: string;
  protocol?: string;
  action?: string;
  log_tag?: string;
};

type CalendarState = {
  kind: 'ready' | 'importing' | 'failed' | 'pending' | 'skipped';
  label: string;
  title: string;
};

function address(ip?: string, port?: number) {
  if (!ip) return '-';
  return `${ip}${port ? `:${port}` : ''}`;
}

function mono(value?: React.ReactNode) {
  return <span className="mono-number">{value || '-'}</span>;
}

function actionText(action?: string) {
  const map: Record<string, string> = {
    ALLOW: '放行',
    DENY: '拒绝',
  };
  return action ? map[action] || action : '-';
}

function normalizeProtocolText(protocol?: string) {
  const value = String(protocol || '').trim().replace(/[;,]+$/g, '').toUpperCase();
  const map: Record<string, string> = {
    '6': 'TCP',
    '17': 'UDP',
    '1': 'ICMP',
  };
  return map[value] || value || '-';
}

function statusText(status?: string) {
  const map: Record<string, string> = {
    ready: '已入库',
    importing: '入库中',
    failed: '失败',
    pending: '待入库',
    skipped: '未查询',
  };
  return status ? map[status] || status : '-';
}

function parseCidrAliases(value?: CidrAliasSetting[] | string): CidrAliasSetting[] {
  if (Array.isArray(value)) return value;
  if (!value) return [];
  try {
    const parsed = JSON.parse(value) as CidrAliasSetting[];
    return Array.isArray(parsed) ? parsed : [];
  } catch {
    return [];
  }
}

function ipv4ToNumber(ip?: string) {
  const parts = String(ip || '').split('.').map((part) => Number(part));
  if (parts.length !== 4 || parts.some((part) => !Number.isInteger(part) || part < 0 || part > 255)) {
    return null;
  }
  return (((parts[0] * 256 + parts[1]) * 256 + parts[2]) * 256 + parts[3]) >>> 0;
}

function matchCidrAlias(ip: string | undefined, aliases: CidrAliasSetting[]) {
  const ipNumber = ipv4ToNumber(ip);
  if (ipNumber === null) return '';

  let best = '';
  let bestPrefix = -1;
  aliases.forEach((item) => {
    if (item.enabled === false || !item.cidr || !item.alias) return;
    const [base, prefixText] = item.cidr.split('/');
    const baseNumber = ipv4ToNumber(base);
    const prefix = Number(prefixText);
    if (baseNumber === null || !Number.isInteger(prefix) || prefix < 0 || prefix > 32) return;
    const mask = prefix === 0 ? 0 : (0xffffffff << (32 - prefix)) >>> 0;
    if ((ipNumber & mask) === (baseNumber & mask) && prefix > bestPrefix) {
      best = item.alias;
      bestPrefix = prefix;
    }
  });
  return best;
}

function geoFallback(ip?: string) {
  const ipNumber = ipv4ToNumber(ip);
  if (ipNumber === null) return '-';
  const a = ipNumber >>> 24;
  const b = (ipNumber >>> 16) & 255;
  if (a === 10 || a === 127 || (a === 172 && b >= 16 && b <= 31) || (a === 192 && b === 168)) {
    return '内网';
  }
  return '未知公网';
}

function formatCount(value?: number) {
  return new Intl.NumberFormat('zh-CN').format(value ?? 0);
}

function latestReadyRange(states: DateState[], fallbackDate?: string): [Dayjs, Dayjs] {
  const readyDates = states
    .filter((item) => item.status === 'ready' && item.log_date)
    .map((item) => String(item.log_date).slice(0, 10))
    .sort();
  const date = readyDates[readyDates.length - 1] || fallbackDate?.slice(0, 10);
  if (date) {
    const day = dayjs(date);
    return [day.startOf('day'), day.endOf('day')];
  }
  return [dayjs().subtract(7, 'day').startOf('day'), dayjs().endOf('day')];
}

export function LogSearchPage(_props: LogSearchPageProps) {
  const [form] = Form.useForm<SearchFormValues>();
  const [loading, setLoading] = React.useState(false);
  const [booting, setBooting] = React.useState(false);
  const [response, setResponse] = React.useState<QueryResponse | null>(null);
  const [advancedOpen, setAdvancedOpen] = React.useState(false);
  const [dateStates, setDateStates] = React.useState<DateState[]>([]);
  const [cidrAliases, setCidrAliases] = React.useState<CidrAliasSetting[]>([]);
  const [queryValues, setQueryValues] = React.useState<SearchFormValues | null>(null);
  const [queryPageSize, setQueryPageSize] = React.useState(defaultQueryPageSize);
  const [cursorStack, setCursorStack] = React.useState<string[]>(['']);

  const runSearch = React.useCallback(async (
    values: SearchFormValues,
    page = 1,
    pageSize = queryPageSize,
    cursor?: string,
    resetCursorStack = false,
  ) => {
    const range = values.range;
    setLoading(true);
    try {
      const nextResponse = await apiGet<QueryResponse>(`/api/query${buildQueryString({
        start: range?.[0]?.format('YYYY-MM-DD HH:mm:ss'),
        end: range?.[1]?.format('YYYY-MM-DD HH:mm:ss'),
        ip: values.ip,
        src_ip: values.src_ip,
        dst_ip: values.dst_ip,
        nat_ip: values.nat_ip,
        src_port: values.src_port,
        dst_port: values.dst_port,
        nat_port: values.nat_port,
        protocol: values.protocol,
        action: values.action,
        log_tag: values.log_tag,
        page,
        page_size: pageSize,
        cursor,
      })}`);
      setResponse(nextResponse);
      setQueryPageSize(pageSize);
      setCursorStack((previous) => {
        const next = resetCursorStack || page === 1 ? [''] : previous.slice(0, page);
        if (nextResponse.next_cursor) {
          next[page] = nextResponse.next_cursor;
        }
        return next;
      });
    } finally {
      setLoading(false);
    }
  }, [queryPageSize]);

  const handleSearch = async () => {
    try {
      const values = await form.validateFields();
      setQueryValues(values);
      await runSearch(values, 1, queryPageSize, undefined, true);
    } catch (error) {
      if (error && typeof error === 'object' && 'errorFields' in error) return;
      message.error(error instanceof Error ? error.message : '日志查询失败');
    }
  };

  React.useEffect(() => {
    let active = true;
    const loadInitialData = async () => {
      setBooting(true);
      try {
        const progress = await apiGet<ProgressResponse>('/api/ingest-progress?range=all&include_ready=true');
        const settings = await apiGet<SettingsResponse>('/api/settings');
        if (!active) return;
        const states = progress.dates || [];
        setDateStates(states);
        setCidrAliases(parseCidrAliases(settings.cidr_aliases));
      } catch (error) {
        if (!active) return;
        message.error(error instanceof Error ? error.message : '加载入库状态失败');
      } finally {
        if (active) setBooting(false);
      }
    };
    void loadInitialData();
    return () => {
      active = false;
    };
  }, [form, runSearch]);

  const columns: ProColumns<SearchRecord>[] = [
    { title: '时间', dataIndex: 'timestamp', width: 180, render: (_, row) => mono(row.timestamp) },
    { title: '日志名称', dataIndex: 'log_tag', width: 150 },
    { title: '源 IP / 端口', width: 180, render: (_, row) => mono(address(row.src_ip, row.src_port)) },
    { title: '目标 IP / 端口', width: 180, render: (_, row) => mono(address(row.dst_ip, row.dst_port)) },
    { title: 'NAT IP / 端口', width: 180, render: (_, row) => mono(address(row.nat_ip, row.nat_port)) },
    { title: '协议', dataIndex: 'protocol', width: 90, render: (_, row) => mono(normalizeProtocolText(row.protocol)) },
    { title: '结果', dataIndex: 'action', width: 100, render: (_, row) => <Tag color={row.action === 'DENY' ? 'error' : 'processing'}>{actionText(row.action)}</Tag> },
    { title: '源 IP 标注', dataIndex: 'src_ip_label', width: 160, render: (_, row) => row.src_ip_label || matchCidrAlias(row.src_ip, cidrAliases) || '-' },
    { title: '目标地区', dataIndex: 'dst_geo', width: 160, render: (_, row) => row.dst_geo || geoFallback(row.dst_ip) },
  ];

  const visibility = response?.visibility;
  const currentPage = response?.page || 1;
  const currentPageSize = response?.page_size || queryPageSize;
  const currentRecordCount = response?.records?.length || 0;
  const pagerTotal = response
    ? ((currentPage - 1) * currentPageSize) + currentRecordCount + (response.has_more ? 1 : 0)
    : 0;
  const visibleDateState = React.useMemo(() => {
    const dates = new Map<string, CalendarState>();

    dateStates.forEach((item) => {
      const date = item.log_date?.slice(0, 10);
      if (!date) return;
      if (item.status === 'ready') {
        dates.set(date, {
          kind: 'ready',
          label: '可查',
          title: `已入库，${formatCount(item.rows_imported)} 行`,
        });
        return;
      }
      if (item.status === 'importing') {
        const pct = Math.round(item.progress_pct ?? 0);
        dates.set(date, {
          kind: 'importing',
          label: `${pct}%`,
          title: `入库中，${pct}%${item.current_file ? `，${item.current_file}` : ''}`,
        });
        return;
      }
      if (item.status === 'failed') {
        dates.set(date, {
          kind: 'failed',
          label: '失败',
          title: item.error || '入库失败',
        });
        return;
      }
      dates.set(date, {
        kind: 'pending',
        label: '待入',
        title: statusText(item.status),
      });
    });

    visibility?.queried_ranges?.forEach((range) => {
      dates.set(range.log_date.slice(0, 10), {
        kind: 'ready',
        label: '可查',
        title: '已入库，可查询',
      });
    });
    visibility?.skipped_dates?.forEach((item) => {
      const date = item.log_date.slice(0, 10);
      if (dates.has(date)) return;
      dates.set(date, {
        kind: 'skipped',
        label: '未入',
        title: `${statusText(item.status)}，${item.reason}`,
      });
    });
    return dates;
  }, [dateStates, visibility]);

  const renderDateCell = React.useCallback((current: Dayjs | string | number, info: { originNode: React.ReactNode; type: string }) => {
    if (info.type !== 'date' || !dayjs.isDayjs(current)) return info.originNode;
    const state = visibleDateState.get(current.format('YYYY-MM-DD'));
    if (!state) return info.originNode;
    return (
      <div className={`visible-date-cell visible-date-cell-${state.kind}`} title={state.title}>
        {info.originNode}
        <span className="visible-date-label">{state.label}</span>
      </div>
    );
  }, [visibleDateState]);

  return (
    <div className="page-stack">
      <section className="page-header">
        <div>
          <span className="eyebrow">NAT 日志检索</span>
          <h1>日志查询</h1>
        </div>
      </section>

      <section className="ops-section search-panel">
        <Form form={form} layout="vertical" className="dense-form">
          <div className="primary-filter-grid">
            <Form.Item className="time-range-item" name="range" label="日期" rules={[{ required: true, message: '请选择日期' }]}>
              <RangePicker
                showTime
                format="YYYY-MM-DD HH:mm:ss"
                separator="到"
                allowClear={false}
                cellRender={renderDateCell}
                renderExtraFooter={() => (
                  <div className="date-picker-legend">
                    <span><i className="legend-dot legend-dot-ready" />可查</span>
                    <span><i className="legend-dot legend-dot-importing" />入库中</span>
                    <span><i className="legend-dot legend-dot-pending" />未入库</span>
                    <span><i className="legend-dot legend-dot-failed" />失败</span>
                  </div>
                )}
                style={{ width: '100%' }}
              />
            </Form.Item>
            <Form.Item name="ip" label="IP"><Input /></Form.Item>
            <div className="filter-actions">
              <Button type="primary" icon={<SearchOutlined />} loading={loading || booting} onClick={() => void handleSearch()}>
                查询
              </Button>
              <Button
                icon={advancedOpen ? <UpOutlined /> : <DownOutlined />}
                onClick={() => setAdvancedOpen((open) => !open)}
              >
                更多条件
              </Button>
            </div>
          </div>

          {advancedOpen && (
          <div className="filter-grid advanced-filter-grid">
            <Form.Item name="src_ip" label="源 IP"><Input /></Form.Item>
            <Form.Item name="dst_ip" label="目标 IP"><Input /></Form.Item>
            <Form.Item name="nat_ip" label="NAT IP"><Input /></Form.Item>
            <Form.Item name="src_port" label="源端口"><Input /></Form.Item>
            <Form.Item name="dst_port" label="目标端口"><Input /></Form.Item>
            <Form.Item name="nat_port" label="NAT 端口"><Input /></Form.Item>
            <Form.Item name="protocol" label="协议"><Select allowClear options={['TCP', 'UDP', 'ICMP'].map((value) => ({ value, label: value }))} /></Form.Item>
            <Form.Item name="action" label="结果"><Select allowClear options={[{ value: 'ALLOW', label: '放行' }, { value: 'DENY', label: '拒绝' }]} /></Form.Item>
            <Form.Item name="log_tag" label="日志名称"><Input /></Form.Item>
          </div>
          )}
        </Form>
      </section>

      {visibility?.partial && visibility.message ? (
        <Alert className="query-visibility-alert" type="warning" showIcon message={visibility.message} />
      ) : null}

      <ProTable<SearchRecord>
        className="app-data-table search-results-table"
        rowKey={(row) => row.id || `${row.timestamp}-${row.source_file}-${row.source_offset}`}
        columns={columns}
        dataSource={response?.records || []}
        loading={loading || booting}
        search={false}
        options={false}
        pagination={{
          current: currentPage,
          pageSize: currentPageSize,
          total: pagerTotal,
          showSizeChanger: true,
          pageSizeOptions: [50, 100, 200, 500],
          showTotal: () => response ? `第 ${currentPage} 页，当前 ${currentRecordCount} 条` : '',
          onChange: (page, pageSize) => {
            if (!queryValues) return;
            if (pageSize !== currentPageSize) {
              void runSearch(queryValues, 1, pageSize, undefined, true);
              return;
            }
            if (page > currentPage) {
              const nextCursor = cursorStack[currentPage] || response?.next_cursor;
              if (!nextCursor) return;
              void runSearch(queryValues, page, pageSize, nextCursor);
              return;
            }
            const previousCursor = cursorStack[page - 1] || undefined;
            void runSearch(queryValues, page, pageSize, previousCursor);
          },
        }}
        expandable={{
          expandedRowRender: (row) => (
            <Descriptions className="record-detail" size="small" column={3}>
              <Descriptions.Item label="来源文件">{row.source_file || '-'}</Descriptions.Item>
              <Descriptions.Item label="文件偏移">{row.source_offset ?? '-'}</Descriptions.Item>
              <Descriptions.Item label="日志源">{row.source_id || '-'}</Descriptions.Item>
              <Descriptions.Item label="日志日期">{row.log_date || '-'}</Descriptions.Item>
              <Descriptions.Item label="入库时间">{row.ingested_at || '-'}</Descriptions.Item>
              <Descriptions.Item label="IP 标注">
                源：{row.src_ip_label || matchCidrAlias(row.src_ip, cidrAliases) || '-'}；目标：{row.dst_geo || geoFallback(row.dst_ip)}
              </Descriptions.Item>
            </Descriptions>
          ),
        }}
        headerTitle={(
          <div className="table-title-block">
            <h3>查询结果</h3>
            {response ? (
              <span>
                第 <span className="mono-number">{currentPage}</span> 页，显示 <span className="mono-number">{currentRecordCount}</span> 条，耗时{' '}
                <span className="mono-number">{response.query_time_ms ?? 0}</span> ms
              </span>
            ) : <span>请选择时间范围后查询</span>}
          </div>
        )}
      />
    </div>
  );
}
