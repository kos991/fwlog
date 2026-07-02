import React from 'react';
import { SearchOutlined, SyncOutlined } from '@ant-design/icons';
import { ProTable } from '@ant-design/pro-components';
import type { ProColumns } from '@ant-design/pro-components';
import { Alert, Button, DatePicker, Form, Input, Select, Space, Tag, Typography, message } from 'antd';
import dayjs, { type Dayjs } from 'dayjs';
import { apiGet, buildQueryString, type QueryVisibility } from '../api';

const { RangePicker } = DatePicker;
const { Text } = Typography;

type SearchRecord = {
  id?: string;
  timestamp?: string;
  time?: string;
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
  src_label?: string;
  dst_country_region?: string;
  dst_geo?: string;
  source_file?: string;
  source_offset?: number;
  source_id_detail?: string;
  log_date?: string;
  ingested_at?: string;
};

type QueryResponse = {
  records: SearchRecord[];
  total: number;
  page: number;
  page_size: number;
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

function makeRecordKey(record: SearchRecord, index?: number) {
  return record.id
    ?? `${record.timestamp || record.time || ''}-${record.source_file || ''}-${record.source_offset || 0}-${index ?? 0}`;
}

function renderAddress(ip?: string, port?: number) {
  if (!ip) return '-';
  return `${ip}${port !== undefined && port !== null ? `:${port}` : ''}`;
}

export function LogSearchPage({ onOpenProgress }: LogSearchPageProps) {
  const [form] = Form.useForm<SearchFormValues>();
  const [loading, setLoading] = React.useState(false);
  const [response, setResponse] = React.useState<QueryResponse | null>(null);

  React.useEffect(() => {
    form.setFieldsValue({
      range: [dayjs().subtract(7, 'day').startOf('day'), dayjs().endOf('day')]
    });
    void handleSearch();
  }, [form]);

  const handleSearch = async () => {
    try {
      const values = await form.validateFields();
      const range = values.range;
      const params = {
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
        log_tag: values.log_tag
      };

      setLoading(true);
      const data = await apiGet<QueryResponse>(`/api/query${buildQueryString(params)}`);
      setResponse(data);
    } catch (error) {
      if (error && typeof error === 'object' && 'errorFields' in error) {
        return;
      }
      message.error(error instanceof Error ? error.message : '日志检索失败');
    } finally {
      setLoading(false);
    }
  };

  const columns: ProColumns<SearchRecord>[] = [
    {
      title: '时间',
      dataIndex: 'timestamp',
      width: 180,
      render: (_, record) => record.timestamp || record.time || '-'
    },
    { title: '日志标识', dataIndex: 'log_tag', width: 160, renderText: (value) => value || '-' },
    {
      title: '源 IP / 端口',
      key: 'src',
      width: 180,
      render: (_, record) => renderAddress(record.src_ip, record.src_port)
    },
    {
      title: '目标 IP / 端口',
      key: 'dst',
      width: 180,
      render: (_, record) => renderAddress(record.dst_ip, record.dst_port)
    },
    {
      title: 'NAT IP / 端口',
      key: 'nat',
      width: 180,
      render: (_, record) => renderAddress(record.nat_ip, record.nat_port)
    },
    { title: '协议', dataIndex: 'protocol', width: 100, renderText: (value) => value || '-' },
    {
      title: '动作',
      dataIndex: 'action',
      width: 100,
      render: (_, record) => {
        const action = record.action || '-';
        const color = /deny|drop|reject|拒绝/i.test(action) ? 'error' : 'processing';
        return <Tag color={color}>{action}</Tag>;
      }
    },
    {
      title: '源 IP 标注',
      width: 180,
      render: (_, record) => record.src_ip_label || record.src_label || '-'
    },
    {
      title: '目标 IP 国家地区',
      width: 180,
      render: (_, record) => record.dst_country_region || record.dst_geo || '-'
    }
  ];

  return (
    <div className="page-stack">
      <section className="ops-section">
        <div className="section-head">
          <h3>基础筛选</h3>
        </div>
        <Form form={form} layout="vertical" className="dense-form">
          <div className="filter-grid">
            <Form.Item
              name="range"
              label="时间范围"
              rules={[{ required: true, message: '请选择时间范围' }]}
            >
              <RangePicker showTime style={{ width: '100%' }} />
            </Form.Item>
            <Form.Item name="ip" label="任意 IP">
              <Input placeholder="支持任意 IP" />
            </Form.Item>
            <Form.Item name="src_ip" label="源 IP">
              <Input placeholder="源 IP" />
            </Form.Item>
            <Form.Item name="dst_ip" label="目标 IP">
              <Input placeholder="目标 IP" />
            </Form.Item>
            <Form.Item name="nat_ip" label="NAT IP">
              <Input placeholder="NAT IP" />
            </Form.Item>
            <Form.Item name="src_port" label="源端口">
              <Input placeholder="源端口" />
            </Form.Item>
            <Form.Item name="dst_port" label="目标端口">
              <Input placeholder="目标端口" />
            </Form.Item>
            <Form.Item name="nat_port" label="NAT 端口">
              <Input placeholder="NAT 端口" />
            </Form.Item>
            <Form.Item name="protocol" label="协议">
              <Select
                allowClear
                options={['TCP', 'UDP', 'ICMP'].map((item) => ({ label: item, value: item }))}
              />
            </Form.Item>
            <Form.Item name="action" label="动作">
              <Select
                allowClear
                options={['ALLOW', 'DENY'].map((item) => ({ label: item, value: item }))}
              />
            </Form.Item>
            <Form.Item name="log_tag" label="日志标识">
              <Input placeholder="日志标识" />
            </Form.Item>
          </div>
          <Space>
            <Button type="primary" icon={<SearchOutlined />} onClick={() => void handleSearch()} loading={loading}>
              查询
            </Button>
          </Space>
        </Form>
      </section>

      {response?.visibility.partial ? (
        <Alert
          type="warning"
          showIcon
          className="block-alert"
          message={response.visibility.message || '所选时间包含未完成入库日期，已自动缩小到可查询范围。'}
          action={
            <Button type="link" icon={<SyncOutlined />} onClick={onOpenProgress}>
              跳转增量进度
            </Button>
          }
        />
      ) : null}

      <section className="ops-section">
        <div className="section-head section-head-row">
          <h3>检索结果</h3>
          <Text type="secondary">
            {response ? `共 ${response.total} 条，查询耗时 ${response.query_time_ms} ms` : '尚未返回结果'}
          </Text>
        </div>
        <ProTable<SearchRecord>
          rowKey={makeRecordKey}
          loading={loading}
          columns={columns}
          dataSource={response?.records ?? []}
          search={false}
          options={false}
          pagination={false}
          size="small"
          cardBordered={false}
          expandable={{
            expandedRowRender: (record) => (
              <dl className="detail-grid">
                <div><dt>source_file</dt><dd className="mono-cell">{record.source_file || '-'}</dd></div>
                <div><dt>source_offset</dt><dd>{record.source_offset ?? '-'}</dd></div>
                <div><dt>source_id</dt><dd>{record.source_id_detail || record.source_id || '-'}</dd></div>
                <div><dt>log_date</dt><dd>{record.log_date || '-'}</dd></div>
                <div><dt>ingested_at</dt><dd>{record.ingested_at || '-'}</dd></div>
                <div><dt>源 IP 标注</dt><dd>{record.src_ip_label || record.src_label || '-'}</dd></div>
                <div><dt>目标 IP 国家地区</dt><dd>{record.dst_country_region || record.dst_geo || '-'}</dd></div>
              </dl>
            )
          }}
        />
      </section>
    </div>
  );
}
