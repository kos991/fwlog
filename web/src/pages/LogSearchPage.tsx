import React from 'react';
import { DownOutlined, SearchOutlined, UpOutlined } from '@ant-design/icons';
import { ProTable } from '@ant-design/pro-components';
import type { ProColumns } from '@ant-design/pro-components';
import { Button, DatePicker, Descriptions, Form, Input, Select, Tag, message } from 'antd';
import dayjs, { type Dayjs } from 'dayjs';
import { apiGet, buildQueryString, type QueryVisibility } from '../api';

const { RangePicker } = DatePicker;

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
  query_time_ms: number;
  visibility: QueryVisibility;
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

export function LogSearchPage(_props: LogSearchPageProps) {
  const [form] = Form.useForm<SearchFormValues>();
  const [loading, setLoading] = React.useState(false);
  const [response, setResponse] = React.useState<QueryResponse | null>(null);
  const [advancedOpen, setAdvancedOpen] = React.useState(false);

  const handleSearch = async () => {
    try {
      const values = await form.validateFields();
      const range = values.range;
      setLoading(true);
      setResponse(await apiGet<QueryResponse>(`/api/query${buildQueryString({
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
      })}`));
    } catch (error) {
      if (error && typeof error === 'object' && 'errorFields' in error) return;
      message.error(error instanceof Error ? error.message : '日志查询失败');
    } finally {
      setLoading(false);
    }
  };

  React.useEffect(() => {
    form.setFieldsValue({ range: [dayjs().subtract(7, 'day').startOf('day'), dayjs().endOf('day')] });
    void handleSearch();
  }, []);

  const columns: ProColumns<SearchRecord>[] = [
    { title: '时间', dataIndex: 'timestamp', width: 180, render: (_, row) => mono(row.timestamp) },
    { title: '日志名称', dataIndex: 'log_tag', width: 150 },
    { title: '源 IP / 端口', width: 180, render: (_, row) => mono(address(row.src_ip, row.src_port)) },
    { title: '目标 IP / 端口', width: 180, render: (_, row) => mono(address(row.dst_ip, row.dst_port)) },
    { title: 'NAT IP / 端口', width: 180, render: (_, row) => mono(address(row.nat_ip, row.nat_port)) },
    { title: '协议', dataIndex: 'protocol', width: 90, render: (_, row) => mono(row.protocol) },
    { title: '结果', dataIndex: 'action', width: 100, render: (_, row) => <Tag color={row.action === 'DENY' ? 'error' : 'processing'}>{actionText(row.action)}</Tag> },
    { title: '源 IP 标注', dataIndex: 'src_ip_label', width: 160 },
    { title: '目标地区', dataIndex: 'dst_geo', width: 160 },
  ];

  const visibility = response?.visibility;
  const visibleDateState = React.useMemo(() => {
    const dates = new Map<string, { kind: 'queried' | 'skipped'; label: string; title: string }>();
    visibility?.queried_ranges?.forEach((range) => {
      dates.set(range.log_date, { kind: 'queried', label: '可查', title: '已入库，可查询' });
    });
    visibility?.skipped_dates?.forEach((item) => {
      dates.set(item.log_date, { kind: 'skipped', label: '未查', title: `${item.status}：${item.reason}` });
    });
    return dates;
  }, [visibility]);

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
                    <span><i className="legend-dot legend-dot-queried" />可查</span>
                    <span><i className="legend-dot legend-dot-skipped" />未入库</span>
                  </div>
                )}
                style={{ width: '100%' }}
              />
            </Form.Item>
            <Form.Item name="ip" label="IP"><Input /></Form.Item>
            <div className="filter-actions">
              <Button type="primary" icon={<SearchOutlined />} loading={loading} onClick={() => void handleSearch()}>
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

      <ProTable<SearchRecord>
        className="app-data-table search-results-table"
        rowKey={(row) => row.id || `${row.timestamp}-${row.source_file}-${row.source_offset}`}
        columns={columns}
        dataSource={response?.records || []}
        loading={loading}
        search={false}
        options={false}
        pagination={{ pageSize: 50 }}
        expandable={{
          expandedRowRender: (row) => (
            <Descriptions className="record-detail" size="small" column={3}>
              <Descriptions.Item label="来源文件">{row.source_file || '-'}</Descriptions.Item>
              <Descriptions.Item label="文件偏移">{row.source_offset ?? '-'}</Descriptions.Item>
              <Descriptions.Item label="日志源">{row.source_id || '-'}</Descriptions.Item>
              <Descriptions.Item label="日志日期">{row.log_date || '-'}</Descriptions.Item>
              <Descriptions.Item label="入库时间">{row.ingested_at || '-'}</Descriptions.Item>
              <Descriptions.Item label="IP 标注">
                源：{row.src_ip_label || '-'}；目标：{row.dst_geo || '-'}
              </Descriptions.Item>
            </Descriptions>
          ),
        }}
        headerTitle={(
          <div className="table-title-block">
            <h3>查询结果</h3>
            <span>
              共 <span className="mono-number">{response?.total ?? 0}</span> 条，耗时{' '}
              <span className="mono-number">{response?.query_time_ms ?? 0}</span> ms
            </span>
          </div>
        )}
      />
    </div>
  );
}
