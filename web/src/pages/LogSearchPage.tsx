import React from 'react';
import { DownOutlined, InfoCircleOutlined, SearchOutlined, UpOutlined } from '@ant-design/icons';
import { ProTable } from '@ant-design/pro-components';
import type { ProColumns } from '@ant-design/pro-components';
import { Alert, Button, DatePicker, Descriptions, Form, Input, Select, Tag, message } from 'antd';
import type { FilterDropdownProps } from 'antd/es/table/interface';
import dayjs, { type Dayjs } from 'dayjs';
import { apiGet, buildQueryString, type QueryVisibility } from '../api';
import { formatIPPort, geoFallback, matchCidrAlias, type CidrAliasSetting } from '../ipAddress';
import { ingestStatusText } from '../uiCopy';

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
  source_id?: string;
  log_tag?: string;
  log_date?: string;
  status?: string;
  progress_pct?: number;
  rows_imported?: number;
  current_file?: string;
  error?: string;
};

type SettingsResponse = {
  cidr_aliases?: CidrAliasSetting[] | string;
  log_sources?: LogSourceSetting[] | string;
};

type LogSourceSetting = {
  source_id?: string;
  log_tag?: string;
  enabled?: boolean;
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
  source_id?: string;
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
  title: string;
};

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

function parseLogSources(value?: LogSourceSetting[] | string): LogSourceSetting[] {
  if (Array.isArray(value)) return value;
  if (!value) return [];
  try {
    const parsed = JSON.parse(value) as LogSourceSetting[];
    return Array.isArray(parsed) ? parsed : [];
  } catch {
    return [];
  }
}

function sourceDisplayName(source?: Pick<LogSourceSetting, 'source_id' | 'log_tag'>) {
  if (!source) return '-';
  if (source.log_tag && source.source_id) return `${source.log_tag}（${source.source_id}）`;
  return source.log_tag || source.source_id || '-';
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

function textFilterDropdown(placeholder: string, onApply: (value: string) => void) {
  return (props: FilterDropdownProps) => {
    const { selectedKeys, setSelectedKeys, confirm, clearFilters } = props;
    return (
      <div style={{ padding: 8 }}>
        <Input
          placeholder={placeholder}
          value={selectedKeys[0] as string}
          onChange={(e) => setSelectedKeys(e.target.value ? [e.target.value] : [])}
          onPressEnter={() => {
            const value = String(selectedKeys[0] || '').trim();
            confirm({ closeDropdown: true });
            onApply(value);
          }}
          style={{ width: 188, marginBottom: 8, display: 'block' }}
          allowClear
        />
        <div style={{ display: 'flex', gap: 8 }}>
          <Button type="primary" size="small" onClick={() => {
            const value = String(selectedKeys[0] || '').trim();
            confirm({ closeDropdown: true });
            onApply(value);
          }}>筛选</Button>
          <Button size="small" onClick={() => {
            clearFilters?.();
            confirm({ closeDropdown: true });
            onApply('');
          }}>重置</Button>
        </div>
      </div>
    );
  };
}

export function LogSearchPage(_props: LogSearchPageProps) {
  const [form] = Form.useForm<SearchFormValues>();
  const [loading, setLoading] = React.useState(false);
  const [booting, setBooting] = React.useState(false);
  const [response, setResponse] = React.useState<QueryResponse | null>(null);
  const [advancedOpen, setAdvancedOpen] = React.useState(false);
  const [dateStates, setDateStates] = React.useState<DateState[]>([]);
  const [cidrAliases, setCidrAliases] = React.useState<CidrAliasSetting[]>([]);
  const [logSourceOptions, setLogSourceOptions] = React.useState<Array<{ label: string; value: string }>>([]);
  const [queryValues, setQueryValues] = React.useState<SearchFormValues | null>(null);
  const [queryPageSize, setQueryPageSize] = React.useState(defaultQueryPageSize);
  const [cursorStack, setCursorStack] = React.useState<string[]>(['']);
  const selectedSourceID = Form.useWatch('source_id', form);

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
        source_id: values.source_id,
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
    } catch (error) {
      message.error(error instanceof Error ? error.message : '日志查询失败');
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

  const applyColumnFilter = React.useCallback((field: keyof SearchFormValues, value: string) => {
    form.setFieldValue(field, value || undefined);
    const values = { ...form.getFieldsValue(), [field]: value || undefined } as SearchFormValues;
    setAdvancedOpen(true);
    setQueryValues(values);
    void runSearch(values, 1, queryPageSize, undefined, true);
  }, [form, queryPageSize, runSearch]);

  React.useEffect(() => {
    let active = true;
    const loadInitialData = async () => {
      setBooting(true);
      try {
        const progress = await apiGet<ProgressResponse>('/api/ingest-progress?range=all&include_ready=true');
        const settings = await apiGet<SettingsResponse>('/api/settings');
        if (!active) return;
        const states = progress.dates || [];
        const configuredSources = parseLogSources(settings.log_sources).filter((source) => source.enabled !== false);
        const sourceMap = new Map<string, LogSourceSetting>();
        configuredSources.forEach((source) => {
          if (source.source_id) sourceMap.set(source.source_id, source);
        });
        states.forEach((state) => {
          if (state.source_id && !sourceMap.has(state.source_id)) {
            sourceMap.set(state.source_id, { source_id: state.source_id, log_tag: state.log_tag });
          }
        });
        setDateStates(states);
        setCidrAliases(parseCidrAliases(settings.cidr_aliases));
        setLogSourceOptions(Array.from(sourceMap.values())
          .sort((left, right) => (left.source_id || '').localeCompare(right.source_id || '', 'zh-CN'))
          .map((source) => ({ value: source.source_id || '', label: sourceDisplayName(source) }))
          .filter((option) => option.value));
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
    {
      title: '来源标识', dataIndex: 'source_id', width: 150,
      render: (_, row) => <Tag color="blue">{row.source_id || '-'}</Tag>,
    },
    {
      title: '来源名称', dataIndex: 'log_tag', width: 150,
      filteredValue: queryValues?.log_tag ? [queryValues.log_tag] : null,
      filterDropdown: textFilterDropdown('输入来源名称', (value) => applyColumnFilter('log_tag', value)),
    },
    {
      title: '源 IP / 端口', width: 180,
      render: (_, row) => mono(formatIPPort(row.src_ip, row.src_port)),
      filteredValue: queryValues?.src_ip ? [queryValues.src_ip] : null,
      filterDropdown: textFilterDropdown('输入源 IP', (value) => applyColumnFilter('src_ip', value)),
    },
    {
      title: '目标 IP / 端口', width: 180,
      render: (_, row) => mono(formatIPPort(row.dst_ip, row.dst_port)),
      filteredValue: queryValues?.dst_ip ? [queryValues.dst_ip] : null,
      filterDropdown: textFilterDropdown('输入目标 IP', (value) => applyColumnFilter('dst_ip', value)),
    },
    {
      title: 'NAT IP / 端口', width: 180,
      render: (_, row) => mono(formatIPPort(row.nat_ip, row.nat_port)),
      filteredValue: queryValues?.nat_ip ? [queryValues.nat_ip] : null,
      filterDropdown: textFilterDropdown('输入 NAT IP', (value) => applyColumnFilter('nat_ip', value)),
    },
    {
      title: '协议', dataIndex: 'protocol', width: 90,
      render: (_, row) => mono(normalizeProtocolText(row.protocol)),
      filteredValue: queryValues?.protocol ? [queryValues.protocol] : null,
      filterDropdown: textFilterDropdown('输入 TCP、UDP 或 ICMP', (value) => applyColumnFilter('protocol', value.toUpperCase())),
    },
    {
      title: '结果', dataIndex: 'action', width: 100,
      render: (_, row) => <Tag color={row.action === 'DENY' ? 'error' : 'processing'}>{actionText(row.action)}</Tag>,
      filteredValue: queryValues?.action ? [queryValues.action] : null,
      filterDropdown: textFilterDropdown('输入 ALLOW 或 DENY', (value) => applyColumnFilter('action', value.toUpperCase())),
    },
    {
      title: '源 IP 标注', dataIndex: 'src_ip_label', width: 160,
      render: (_, row) => row.src_ip_label || matchCidrAlias(row.src_ip, cidrAliases) || '-',
    },
    {
      title: '目标地区', dataIndex: 'dst_geo', width: 160,
      render: (_, row) => row.dst_geo || geoFallback(row.dst_ip),
    },
  ];

  const visibility = response?.visibility;
  const currentPage = response?.page || 1;
  const currentPageSize = response?.page_size || queryPageSize;
  const currentTotal = response?.total || 0;
  const currentRecordCount = response?.records?.length || 0;
  const pagerTotal = currentTotal;
  const visibleDateState = React.useMemo(() => {
    const dates = new Map<string, CalendarState>();
    const priority: Record<CalendarState['kind'], number> = {
      failed: 5,
      importing: 4,
      pending: 3,
      skipped: 2,
      ready: 1,
    };

    dateStates.forEach((item) => {
      if (selectedSourceID && item.source_id !== selectedSourceID) return;
      const date = item.log_date?.slice(0, 10);
      if (!date) return;
      const sourceText = sourceDisplayName(item);
      let next: CalendarState;
      if (item.status === 'ready') {
        next = {
          kind: 'ready',
          title: `${sourceText}：已完成，可查询 ${formatCount(item.rows_imported)} 行`,
        };
      } else if (item.status === 'importing') {
        const pct = Math.round(item.progress_pct ?? 0);
        next = {
          kind: 'importing',
          title: `${sourceText}：正在入库，${pct}%${item.current_file ? `，${item.current_file}` : ''}`,
        };
      } else if (item.status === 'failed') {
        next = {
          kind: 'failed',
          title: `${sourceText}：${item.error || '处理失败'}`,
        };
      } else {
        next = {
          kind: 'pending',
          title: `${sourceText}：${ingestStatusText(item.status)}`,
        };
      }
      const previous = dates.get(date);
      if (!previous || priority[next.kind] > priority[previous.kind]) {
        dates.set(date, next);
      }
    });

    visibility?.queried_ranges?.forEach((range) => {
      dates.set(range.log_date.slice(0, 10), {
        kind: 'ready',
        title: '已完成，可查询',
      });
    });
    visibility?.skipped_dates?.forEach((item) => {
      const date = item.log_date.slice(0, 10);
      if (dates.has(date)) return;
      dates.set(date, {
        kind: 'skipped',
        title: `${ingestStatusText(item.status)}，${item.reason}`,
      });
    });
    return dates;
  }, [dateStates, selectedSourceID, visibility]);

  const renderDateCell = React.useCallback((current: Dayjs | string | number, info: { originNode: React.ReactNode; type: string }) => {
    if (info.type !== 'date' || !dayjs.isDayjs(current)) return info.originNode;
    const state = visibleDateState.get(current.format('YYYY-MM-DD'));
    if (!state) return info.originNode;
    return (
      <div className={`visible-date-cell visible-date-cell-${state.kind}`} title={state.title}>
        {info.originNode}
        <span className="visible-date-marker" />
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
                    <span><i className="legend-dot legend-dot-ready" />已完成，可查询</span>
                    <span><i className="legend-dot legend-dot-importing" />正在入库</span>
                    <span><i className="legend-dot legend-dot-pending" />等待处理</span>
                    <span><i className="legend-dot legend-dot-failed" />处理失败</span>
                  </div>
                )}
                style={{ width: '100%' }}
              />
            </Form.Item>
            <Form.Item name="source_id" label="日志来源">
              <Select
                allowClear
                placeholder="全部日志来源"
                options={logSourceOptions}
              />
            </Form.Item>
            <Form.Item name="ip" label="任意 IP"><Input /></Form.Item>
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
            <Form.Item name="action" label="访问结果"><Select allowClear options={[{ value: 'ALLOW', label: '放行' }, { value: 'DENY', label: '拒绝' }]} /></Form.Item>
            <Form.Item name="log_tag" label="来源名称"><Input /></Form.Item>
          </div>
          )}
          <div className="query-limit-hint" aria-label="查询范围说明">
            <InfoCircleOutlined />
            <span>无筛选查询最多支持 1 天。</span>
            <span>填写任一 IP、端口、协议、访问结果或来源名称后，最多支持 31 天。</span>
          </div>
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
          showTotal: () => response ? `第 ${currentPage} 页，共 ${currentTotal} 条` : '',
          onChange: (page, pageSize) => {
            if (!queryValues) return;
            if (pageSize !== currentPageSize) {
              void runSearch(queryValues, 1, pageSize, undefined, true);
              return;
            }
            if (page === currentPage + 1) {
              const nextCursor = cursorStack[currentPage] || response?.next_cursor;
              if (!nextCursor) return;
              void runSearch(queryValues, page, pageSize, nextCursor);
              return;
            }
            if (page === currentPage - 1) {
              const previousCursor = cursorStack[page - 1] || undefined;
              void runSearch(queryValues, page, pageSize, previousCursor);
              return;
            }
            void runSearch(queryValues, page, pageSize, undefined);
          },
        }}
        expandable={{
          expandedRowRender: (row) => (
            <Descriptions className="record-detail" size="small" column={3}>
              <Descriptions.Item label="来源文件">{row.source_file || '-'}</Descriptions.Item>
              <Descriptions.Item label="文件内位置">{row.source_offset ?? '-'}</Descriptions.Item>
              <Descriptions.Item label="来源标识">{row.source_id || '-'}</Descriptions.Item>
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
                第 <span className="mono-number">{currentPage}</span> 页，共 <span className="mono-number">{currentTotal}</span> 条，本页显示 <span className="mono-number">{currentRecordCount}</span> 条，耗时{' '}
                <span className="mono-number">{response.query_time_ms ?? 0}</span> ms
              </span>
            ) : <span>请选择时间范围后查询</span>}
          </div>
        )}
      />
    </div>
  );
}
