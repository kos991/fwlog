import React from 'react';
import {
  ClockCircleOutlined,
  CloudDownloadOutlined,
  DatabaseOutlined,
  DeleteOutlined,
  FileTextOutlined,
  FolderOpenOutlined,
  GlobalOutlined,
  KeyOutlined,
  LogoutOutlined,
  PlusOutlined,
  ReloadOutlined,
  SaveOutlined,
  SafetyCertificateOutlined,
  SyncOutlined,
  TagsOutlined,
  UploadOutlined,
  WarningOutlined,
} from '@ant-design/icons';
import { Button, DatePicker, Form, Input, InputNumber, Popconfirm, Select, Space, Switch, Tabs, Tag, TimePicker, Typography, Upload, message } from 'antd';
import dayjs, { type Dayjs } from 'dayjs';
import { apiGet, apiPost, apiUpload, type UpgradeCheckResponse, type UpgradeStatus } from '../api';
import { buildUpgradeView } from '../upgradePresentation';

const { Text } = Typography;

type SystemMaintenancePageProps = {
  onRequireLogin: () => void;
};

type LogSourceSetting = {
  source_id?: string;
  log_tag?: string;
  log_dir?: string;
  source_type?: 'file' | 'rsyslog' | string;
  listen_protocol?: 'udp' | 'tcp' | string;
  listen_host?: string;
  listen_port?: number | string;
  spool_dir?: string;
  enabled?: boolean;
};

type CidrAliasSetting = {
  cidr?: string;
  alias?: string;
  enabled?: boolean;
};

type Settings = {
  log_dir?: string;
  log_tag?: string;
  log_sources?: LogSourceSetting[] | string;
  cidr_aliases?: CidrAliasSetting[] | string;
  custom_ip_map_path?: string;
  geoip_db_path?: string;
  auto_scan_enabled?: boolean | string;
  auto_scan_mode?: string;
  auto_scan_times?: string | Dayjs;
  auto_scan_timezone?: string;
  auto_scan_interval_sec?: number | string;
  current_password?: string;
  new_password?: string;
  confirm_new_password?: string;
};

type IngestAction = 'sync' | 'rebuild';
type IngestDateMode = 'all' | 'single' | 'range';

function tabLabel(icon: React.ReactNode, text: string) {
  return <span className="tab-label">{icon}{text}</span>;
}

function parseCidrAliases(value?: CidrAliasSetting[] | string): CidrAliasSetting[] {
  if (Array.isArray(value)) return value;
  if (!value) return [];
  try {
    const parsed = JSON.parse(value);
    return Array.isArray(parsed) ? parsed : [];
  } catch {
    return [];
  }
}

function parseLogSources(value?: LogSourceSetting[] | string): LogSourceSetting[] {
  if (Array.isArray(value)) return value;
  if (!value) return [];
  try {
    const parsed = JSON.parse(value);
    return Array.isArray(parsed) ? parsed : [];
  } catch {
    return [];
  }
}

function defaultSpoolDir(sourceID?: string) {
  return `/data/fwlog/received/${sourceID || 'source'}`;
}

function normalizeLogSourceSetting(source: LogSourceSetting, index: number): LogSourceSetting {
  const sourceID = source.source_id || `source-${index + 1}`;
  const sourceType = source.source_type === 'rsyslog' ? 'rsyslog' : 'file';
  if (sourceType === 'rsyslog') {
    const spoolDir = source.spool_dir || source.log_dir || defaultSpoolDir(sourceID);
    return {
      ...source,
      source_id: sourceID,
      source_type: 'rsyslog',
      listen_protocol: source.listen_protocol?.toLowerCase() === 'tcp' ? 'tcp' : 'udp',
      listen_host: source.listen_host || '0.0.0.0',
      listen_port: Number(source.listen_port || 5514),
      spool_dir: spoolDir,
      log_dir: source.log_dir || spoolDir,
      enabled: source.enabled !== false,
    };
  }
  return {
    ...source,
    source_id: sourceID,
    source_type: 'file',
    enabled: source.enabled !== false,
  };
}

function normalizeLogSourcesForForm(sources: LogSourceSetting[]) {
  return sources.map((source, index) => normalizeLogSourceSetting(source, index));
}

function firstAutoScanTime(value?: string | Dayjs | null) {
  if (dayjs.isDayjs(value)) {
    return value.format('HH:mm');
  }
  const first = String(value || '')
    .split(',')
    .map((item) => item.trim())
    .find((item) => /^([01]\d|2[0-3]):[0-5]\d$/.test(item));
  return first || '01:00';
}

function autoScanTimeValue(value?: string | Dayjs | null) {
  if (dayjs.isDayjs(value)) {
    return value.second(0).millisecond(0);
  }
  const [hour, minute] = firstAutoScanTime(value).split(':').map((item) => Number(item));
  return dayjs().hour(hour).minute(minute).second(0).millisecond(0);
}

function formatAutoScanTime(value?: string | Dayjs | null) {
  return firstAutoScanTime(value);
}

export function SystemMaintenancePage({ onRequireLogin }: SystemMaintenancePageProps) {
  const [form] = Form.useForm<Settings>();
  const [loading, setLoading] = React.useState(false);
  const [upgradeLoading, setUpgradeLoading] = React.useState(false);
  const [upgradeStatus, setUpgradeStatus] = React.useState<UpgradeStatus | null>(null);
  const [upgradeCheck, setUpgradeCheck] = React.useState<UpgradeCheckResponse | null>(null);
  const [upgradeCheckError, setUpgradeCheckError] = React.useState('');
  const [upgradeLastCheckedAt, setUpgradeLastCheckedAt] = React.useState<Date | null>(null);
  const [ingestAction, setIngestAction] = React.useState<IngestAction>('sync');
  const [dateMode, setDateMode] = React.useState<IngestDateMode>('all');
  const [singleDate, setSingleDate] = React.useState<Dayjs | null>(dayjs());
  const [dateRange, setDateRange] = React.useState<[Dayjs | null, Dayjs | null] | null>(null);
  const [importSourceID, setImportSourceID] = React.useState('');
  const [savedLogSources, setSavedLogSources] = React.useState<LogSourceSetting[]>([]);
  const [upgradeRestarting, setUpgradeRestarting] = React.useState(false);
  const [upgradeFile, setUpgradeFile] = React.useState<File | null>(null);
  const geoipPath = Form.useWatch('geoip_db_path', form);
  const customIpPath = Form.useWatch('custom_ip_map_path', form);
  const autoScanEnabled = Form.useWatch('auto_scan_enabled', form);
  const autoScanTimes = Form.useWatch('auto_scan_times', form);

  const load = React.useCallback(async () => {
    try {
      setLoading(true);
      const settings = await apiGet<Settings>('/api/settings');
      const parsedLogSources = parseLogSources(settings.log_sources);
      const logSources = normalizeLogSourcesForForm(parsedLogSources.length ? parsedLogSources : [
        {
          source_id: 'default',
          log_tag: settings.log_tag || '深信服 NAT',
          log_dir: settings.log_dir || '/data/sangfor_fw_log',
          source_type: 'file',
          enabled: true,
        },
      ]);
      const cidrAliases = parseCidrAliases(settings.cidr_aliases);
      setSavedLogSources(logSources);
      form.setFieldsValue({
        ...settings,
        log_sources: logSources,
        cidr_aliases: cidrAliases,
        auto_scan_enabled: settings.auto_scan_enabled === true || settings.auto_scan_enabled === 'true',
        auto_scan_mode: settings.auto_scan_mode || 'daily',
        auto_scan_times: autoScanTimeValue(settings.auto_scan_times),
        auto_scan_timezone: settings.auto_scan_timezone || 'Asia/Shanghai',
        auto_scan_interval_sec: Number(settings.auto_scan_interval_sec || 3600),
      });
    } catch (error) {
      message.error(error instanceof Error ? error.message : '加载设置失败');
    } finally {
      setLoading(false);
    }
  }, [form]);

  React.useEffect(() => {
    void load();
  }, [load]);

  const loadUpgradeStatus = React.useCallback(async () => {
    const status = await apiGet<UpgradeStatus>('/api/upgrade/status');
    setUpgradeStatus(status);
  }, []);

  React.useEffect(() => {
    void loadUpgradeStatus().catch(() => undefined);
  }, [loadUpgradeStatus]);

  React.useEffect(() => {
    if (upgradeStatus?.state !== 'running') return;
    let cancelled = false;
    const timer = window.setInterval(() => {
      if (cancelled) return;
      void loadUpgradeStatus().catch(() => {
        if (!cancelled) setUpgradeRestarting(true);
      });
    }, 3000);
    return () => {
      cancelled = true;
      window.clearInterval(timer);
    };
  }, [loadUpgradeStatus, upgradeStatus?.state]);

  React.useEffect(() => {
    if (upgradeStatus?.state === 'succeeded' || upgradeStatus?.state === 'failed') {
      setUpgradeRestarting(false);
    }
  }, [upgradeStatus?.state]);

  const upgradeView = buildUpgradeView({
    status: upgradeStatus,
    check: upgradeCheck,
    isChecking: upgradeLoading,
    lastCheckedAt: upgradeLastCheckedAt,
  });
  const canCheckUpgrade = upgradeStatus?.state !== 'running' && !upgradeLoading;
  const canRunUpgrade = upgradeView.showUpgradeAction && upgradeStatus?.state !== 'running' && !upgradeLoading;
  const autoScanDisplay = firstAutoScanTime(autoScanTimes);
  const autoScanSummary = `自动扫描：${autoScanEnabled ? '已开启' : '已关闭'}；每天 ${autoScanDisplay} 扫描全部启用日志源，按增量入库处理。`;
  const upgradeSummary = `更新维护：当前版本 ${upgradeView.currentVersion}；${upgradeCheckError || upgradeView.message}`;

  const save = async () => {
    try {
      setLoading(true);
      const values = form.getFieldsValue();
      const logSources = normalizeLogSourcesForForm(parseLogSources(values.log_sources));
      const firstSource = logSources[0];
      await apiPost('/api/settings', {
        ...values,
        cidr_aliases: JSON.stringify(values.cidr_aliases || []),
        log_sources: JSON.stringify(logSources),
        log_dir: firstSource?.log_dir || values.log_dir,
        log_tag: firstSource?.log_tag || values.log_tag,
        auto_scan_enabled: String(Boolean(values.auto_scan_enabled)),
        auto_scan_mode: values.auto_scan_mode || 'daily',
        auto_scan_times: formatAutoScanTime(values.auto_scan_times),
        auto_scan_timezone: values.auto_scan_timezone || 'Asia/Shanghai',
        auto_scan_interval_sec: String(Number(values.auto_scan_interval_sec) || 3600),
      });
      setSavedLogSources(logSources);
      if (importSourceID && !logSources.some((source) => source.enabled !== false && source.source_id === importSourceID)) {
        setImportSourceID('');
      }
      message.success('设置已保存');
    } catch (error) {
      message.error(error instanceof Error ? error.message : '保存设置失败');
    } finally {
      setLoading(false);
    }
  };

  const enabledLogSources = React.useMemo(
    () => savedLogSources.filter((source) => source.enabled !== false),
    [savedLogSources],
  );
  const selectedLogSource = enabledLogSources.find((source) => (source.source_id || 'default') === importSourceID);
  const selectedSourceLabel = importSourceID
    ? selectedLogSource?.log_tag || selectedLogSource?.source_id || importSourceID
    : '全部启用日志源';
  const actionLabel = ingestAction === 'sync' ? '手动入库' : '全量重建';
  const dateScopeLabel = dateMode === 'all'
    ? '所有历史日期'
    : dateMode === 'single'
      ? singleDate?.format('YYYY-MM-DD') || '未选择日期'
      : dateRange?.[0] && dateRange?.[1]
        ? `${dateRange[0].format('YYYY-MM-DD')} 至 ${dateRange[1].format('YYYY-MM-DD')}`
        : '未选择日期范围';
  const buttonLabel = ingestAction === 'sync' ? '执行入库' : '执行全量重建';
  const actionSummary = `本次操作：日志源 = ${selectedSourceLabel}；日期 = ${dateScopeLabel}；动作 = ${actionLabel}。`;
  const rebuildConfirmDescription = `本次将对「${selectedSourceLabel}」的「${dateScopeLabel}」执行全量重建。该操作会重新处理目标范围内的数据，耗时可能较长。`;
  const ingestActionDisabled = dateMode === 'single'
    ? !singleDate
    : dateMode === 'range'
      ? !dateRange?.[0] || !dateRange?.[1] || dateRange[0].isAfter(dateRange[1], 'day')
      : false;

  function buildIngestPath() {
    const basePath = ingestAction === 'sync' ? '/api/sync' : '/api/rebuild';
    const params = new URLSearchParams();
    const sourceID = importSourceID.trim();
    if (sourceID) {
      params.set('source_id', sourceID);
    }
    if (dateMode === 'single' && singleDate) {
      params.set('date', singleDate.format('YYYY-MM-DD'));
    }
    if (dateMode === 'range' && dateRange?.[0] && dateRange?.[1]) {
      params.set('date_from', dateRange[0].format('YYYY-MM-DD'));
      params.set('date_to', dateRange[1].format('YYYY-MM-DD'));
    }
    const query = params.toString();
    return query ? `${basePath}?${query}` : basePath;
  }

  const triggerIngestAction = async () => {
    if (dateMode === 'single' && !singleDate) {
      message.error('请先选择日期');
      return;
    }
    if (dateMode === 'range') {
      if (!dateRange?.[0] || !dateRange?.[1]) {
        message.error('请先选择日期范围');
        return;
      }
      if (dateRange[0].isAfter(dateRange[1], 'day')) {
        message.error('开始日期不能晚于结束日期');
        return;
      }
    }
    try {
      setLoading(true);
      await apiPost(buildIngestPath());
      message.success(ingestAction === 'sync' ? '已开始入库' : '已触发全量重建');
    } catch (error) {
      message.error(error instanceof Error ? error.message : '操作失败');
    } finally {
      setLoading(false);
    }
  };

  const trigger = async (path: string, ok: string) => {
    try {
      setLoading(true);
      await apiPost(path);
      message.success(ok);
    } catch (error) {
      message.error(error instanceof Error ? error.message : '操作失败');
    } finally {
      setLoading(false);
    }
  };

  const checkUpgrade = async () => {
    try {
      setUpgradeLoading(true);
      setUpgradeCheckError('');
      const response = await apiGet<UpgradeCheckResponse>('/api/upgrade/check');
      setUpgradeCheck(response);
      setUpgradeStatus(response.status);
      setUpgradeLastCheckedAt(new Date());
      if (response.update_available && response.assets_ready) {
        message.success(`发现可升级版本 ${response.latest_version}`);
      } else if (response.update_available) {
        message.warning('发现新版本，但 Release 资产不齐，暂不能升级');
      } else {
        message.success('当前已是最新版本');
      }
    } catch (error) {
      setUpgradeCheckError(error instanceof Error ? error.message : '检查更新失败');
      message.error(error instanceof Error ? error.message : '检查更新失败');
    } finally {
      setUpgradeLoading(false);
    }
  };



  const runUpgrade = async () => {
    const version = upgradeView.state === 'available' ? upgradeView.latestVersion.trim() : '';
    if (!version) {
      message.error('请先检查更新并确认 Release 资产齐全');
      return;
    }
    if (!/^v\d+\.\d+\.\d+$/.test(version)) {
      message.error('检查到的升级版本必须使用 vX.Y.Z 格式');
      return;
    }
    try {
      setUpgradeLoading(true);
      setUpgradeCheckError('');
      const status = await apiPost<UpgradeStatus>('/api/upgrade/run', { version });
      setUpgradeStatus(status);
      message.success('升级任务已开始');
    } catch (error) {
      setUpgradeCheckError(error instanceof Error ? error.message : '启动升级失败');
      message.error(error instanceof Error ? error.message : '启动升级失败');
    } finally {
      setUpgradeLoading(false);
    }
  };

  const uploadUpgrade = async () => {
    if (!upgradeFile) {
      message.error('请选择 fwlog-upgrade RPM 或 DEB 包');
      return;
    }
    try {
      setUpgradeLoading(true);
      const status = await apiUpload<UpgradeStatus>('/api/upgrade/upload', upgradeFile);
      setUpgradeStatus(status);
      setUpgradeFile(null);
      message.success('离线升级包已上传，安装任务已开始');
    } catch (error) {
      message.error(error instanceof Error ? error.message : '上传升级包失败');
    } finally {
      setUpgradeLoading(false);
    }
  };

  const changePassword = async () => {
    try {
      const values = await form.validateFields(['current_password', 'new_password', 'confirm_new_password']);
      setLoading(true);
      await apiPost('/api/password', {
        current_password: values.current_password,
        new_password: values.new_password,
      });
      message.success('密码已更新，请重新登录');
      onRequireLogin();
    } catch (error) {
      if (error && typeof error === 'object' && 'errorFields' in error) return;
      message.error(error instanceof Error ? error.message : '更新密码失败');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="page-stack">
      <section className="page-header">
        <div>
          <span className="eyebrow">配置和维护</span>
          <h1>系统设置</h1>
        </div>
        <Space>
          <Button icon={<ReloadOutlined />} onClick={() => void load()} loading={loading} />
          <Button type="primary" icon={<SaveOutlined />} onClick={() => void save()} loading={loading}>保存</Button>
        </Space>
      </section>

      <Form form={form} layout="vertical">
        <Tabs
          items={[
            {
              key: 'source',
              label: tabLabel(<FolderOpenOutlined />, '日志源'),
              children: (
                <section className="ops-section maintenance-card">
                  <Form.List name="log_sources">
                    {(fields, { add, remove }) => (
                      <div className="source-list-editor">
                        <div className="source-list-head">
                          <div>
                            <strong>日志源配置</strong>
                          </div>
                          <Space wrap>
                            <Button
                              icon={<PlusOutlined />}
                              onClick={() => add({ source_id: `source-${fields.length + 1}`, log_tag: '', log_dir: '', source_type: 'file', enabled: true })}
                            >
                              添加文件目录源
                            </Button>
                            <Button
                              icon={<PlusOutlined />}
                              onClick={() => add({
                                source_id: `rsyslog-${fields.length + 1}`,
                                log_tag: '',
                                source_type: 'rsyslog',
                                listen_protocol: 'udp',
                                listen_host: '0.0.0.0',
                                listen_port: 5514,
                                spool_dir: defaultSpoolDir(`rsyslog-${fields.length + 1}`),
                                enabled: true,
                              })}
                            >
                              添加 RSyslog 接收源
                            </Button>
                          </Space>
                        </div>
                        {fields.map((field) => (
                          <Form.Item noStyle shouldUpdate key={field.key}>
                            {({ getFieldValue }) => {
                              const sourceType = getFieldValue(['log_sources', field.name, 'source_type']) || 'file';
                              const sourceTitle = sourceType === 'rsyslog' ? 'RSyslog 接收源' : '文件目录源';
                              return (
                                <div className={`source-item-card source-item-card--${sourceType}`}>
                                  <div className="source-item-head">
                                    <Tag color={sourceType === 'rsyslog' ? 'processing' : 'default'}>{sourceTitle}</Tag>
                                    <Space>
                                      <Form.Item name={[field.name, 'enabled']} valuePropName="checked" noStyle>
                                        <Switch checkedChildren="启用" unCheckedChildren="停用" />
                                      </Form.Item>
                                      <Popconfirm
                                        title="删除这个日志源？"
                                        okText="删除"
                                        cancelText="取消"
                                        onConfirm={() => remove(field.name)}
                                      >
                                        <Button danger icon={<DeleteOutlined />} aria-label="删除日志源" />
                                      </Popconfirm>
                                    </Space>
                                  </div>
                                  <div className="source-fields-grid">
                                    <Form.Item name={[field.name, 'source_id']} label="设备 ID" rules={[{ required: true, message: '请输入设备 ID' }]}>
                                      <Input prefix={<DatabaseOutlined />} placeholder="device-id" />
                                    </Form.Item>
                                    <Form.Item name={[field.name, 'log_tag']} label="日志名称" rules={[{ required: true, message: '请输入日志名称' }]}>
                                      <Input prefix={<TagsOutlined />} placeholder="日志名称" />
                                    </Form.Item>
                                    <Form.Item name={[field.name, 'source_type']} label="日志源类型" initialValue="file">
                                      <Select
                                        options={[
                                          { value: 'file', label: '文件目录源' },
                                          { value: 'rsyslog', label: 'RSyslog 接收源' },
                                        ]}
                                      />
                                    </Form.Item>
                                    {sourceType === 'rsyslog' ? (
                                      <>
                                        <Form.Item name={[field.name, 'listen_protocol']} label="接收协议" initialValue="udp">
                                          <Select
                                            options={[
                                              { value: 'udp', label: 'UDP' },
                                              { value: 'tcp', label: 'TCP' },
                                            ]}
                                          />
                                        </Form.Item>
                                        <Form.Item name={[field.name, 'listen_port']} label="监听端口" initialValue={5514} rules={[{ required: true, message: '请输入监听端口' }]}>
                                          <InputNumber min={1} max={65535} prefix={<GlobalOutlined />} placeholder="默认 5514" style={{ width: '100%' }} />
                                        </Form.Item>
                                        <Form.Item className="source-field-wide" name={[field.name, 'spool_dir']} label="落盘目录" rules={[{ required: true, message: '请输入落盘目录' }]}>
                                          <Input prefix={<FolderOpenOutlined />} placeholder="/data/fwlog/received/device-id" />
                                        </Form.Item>
                                        <Form.Item name={[field.name, 'listen_host']} initialValue="0.0.0.0" hidden>
                                          <Input />
                                        </Form.Item>
                                      </>
                                    ) : (
                                      <Form.Item className="source-field-wide" name={[field.name, 'log_dir']} label="文件目录" rules={[{ required: true, message: '请输入文件目录' }]}>
                                        <Input prefix={<FolderOpenOutlined />} placeholder="/data/device_fw_log" />
                                      </Form.Item>
                                    )}
                                  </div>
                                </div>
                              );
                            }}
                          </Form.Item>
                        ))}
                      </div>
                    )}
                  </Form.List>
                </section>
              ),
            },
            {
              key: 'ip',
              label: tabLabel(<GlobalOutlined />, 'IP 库'),
              children: (
                <section className="ops-section maintenance-card">
                  <div className="setting-grid">
                    <div className="setting-fields">
                      <Form.Item name="custom_ip_map_path" label="自定义 IP 映射 CSV">
                        <Input prefix={<FileTextOutlined />} />
                      </Form.Item>
                      <Form.Item name="geoip_db_path" label="GeoIP 数据库">
                        <Input prefix={<DatabaseOutlined />} />
                      </Form.Item>
                      <Form.List name="cidr_aliases">
                        {(fields, { add, remove }) => (
                          <div className="source-list-editor cidr-list-editor ip-cidr-list-editor">
                            <div className="source-list-head">
                              <div>
                                <strong>CIDR 别名</strong>
                                <Text type="secondary">用于把内网网段显示为业务名称</Text>
                              </div>
                              <Button
                                icon={<PlusOutlined />}
                                onClick={() => add({ cidr: '', alias: '', enabled: true })}
                              >
                                添加
                              </Button>
                            </div>
                            <div className="source-row cidr-row source-row-header">
                              <span>CIDR 网段</span>
                              <span>别名</span>
                              <span>启用</span>
                              <span>操作</span>
                            </div>
                            {fields.map((field) => (
                              <div className="source-row cidr-row" key={field.key}>
                                <Form.Item
                                  name={[field.name, 'cidr']}
                                  rules={[
                                    { required: true, message: '必填' },
                                    { pattern: /^(\d{1,3}\.){3}\d{1,3}\/\d{1,2}$/, message: '格式示例：10.10.0.0/16' },
                                  ]}
                                >
                                  <Input prefix={<GlobalOutlined />} placeholder="10.10.0.0/16" />
                                </Form.Item>
                                <Form.Item name={[field.name, 'alias']} rules={[{ required: true, message: '必填' }]}>
                                  <Input prefix={<TagsOutlined />} placeholder="办公网段" />
                                </Form.Item>
                                <Form.Item name={[field.name, 'enabled']} valuePropName="checked">
                                  <Switch />
                                </Form.Item>
                                <Popconfirm
                                  title="删除这个网段别名？"
                                  okText="删除"
                                  cancelText="取消"
                                  onConfirm={() => remove(field.name)}
                                >
                                  <Button danger icon={<DeleteOutlined />} />
                                </Popconfirm>
                              </div>
                            ))}
                          </div>
                        )}
                      </Form.List>
                    </div>
                    <aside className="setting-summary setting-summary-green">
                      <GlobalOutlined />
                      <Text type="secondary">当前 IP 库</Text>
                      <strong>GeoIP + 自定义映射</strong>
                      <code>{geoipPath || '-'}</code>
                      <code>{customIpPath || '-'}</code>
                      <Button icon={<ReloadOutlined />} onClick={() => void trigger('/api/ip-data/reload', 'IP 库已重新加载')} loading={loading}>
                        重新加载
                      </Button>
                    </aside>
                  </div>
                </section>
              ),
            },
            {
              key: 'ops',
              label: tabLabel(<WarningOutlined />, '维护'),
              children: (
                <section className="ops-section maintenance-card maintenance-ops-card">
                  <div className="maintenance-plan-card maintenance-plan-card--schedule">
                    <div className="maintenance-card-head">
                      <div>
                        <span className="maintenance-card-kicker"><ClockCircleOutlined /> 自动扫描</span>
                        <strong>扫描计划</strong>
                      </div>
                    </div>

                    <div className="maintenance-plan-grid">
                      <div className="maintenance-field">
                        <label>状态</label>
                        <div className="maintenance-inline-control">
                          <Form.Item name="auto_scan_enabled" valuePropName="checked" noStyle>
                            <Switch />
                          </Form.Item>
                          <Tag color={autoScanEnabled ? 'processing' : 'default'}>{autoScanEnabled ? '已开启' : '已关闭'}</Tag>
                        </div>
                      </div>
                      <div className="maintenance-field">
                        <label>扫描时间</label>
                        <Form.Item name="auto_scan_times" noStyle>
                          <TimePicker format="HH:mm" allowClear={false} />
                        </Form.Item>
                      </div>
                      <div className="maintenance-field">
                        <label>日志源范围</label>
                        <Text className="maintenance-value">全部启用日志源</Text>
                      </div>
                      <div className="maintenance-field">
                        <label>入库方式</label>
                        <Text className="maintenance-value">按增量入库</Text>
                      </div>
                    </div>

                    <div className="maintenance-card-summary">
                      <Text>{autoScanSummary}</Text>
                    </div>
                  </div>

                  <div className="maintenance-run-card maintenance-run-card--manual">
                    <div className="maintenance-card-head">
                      <div>
                        <span className="maintenance-card-kicker"><SyncOutlined /> 手动维护</span>
                        <strong>入库操作</strong>
                      </div>
                    </div>

                    <div className="maintenance-run-grid">
                      <div className="maintenance-field">
                        <label>日志源</label>
                        <Select
                          value={importSourceID}
                          onChange={setImportSourceID}
                          options={[
                            { value: '', label: '全部启用日志源' },
                            ...enabledLogSources.map((source) => ({
                              value: source.source_id || 'default',
                              label: source.log_tag || source.source_id || 'default',
                            })),
                          ]}
                        />
                      </div>
                      <div className="maintenance-field">
                        <label>日期范围</label>
                        <Select
                          value={dateMode}
                          onChange={(value: IngestDateMode) => setDateMode(value)}
                          options={[
                            { value: 'all', label: '所有历史日期' },
                            { value: 'single', label: '单日' },
                            { value: 'range', label: '日期范围' },
                          ]}
                        />
                      </div>

                      <div className="maintenance-field">
                        <label>日期参数</label>
                        {dateMode === 'single' ? (
                          <DatePicker value={singleDate} onChange={setSingleDate} />
                        ) : dateMode === 'range' ? (
                          <DatePicker.RangePicker value={dateRange} onChange={(value) => setDateRange(value)} />
                        ) : (
                          <Text type="secondary">系统会扫描所选日志源下已有历史日志</Text>
                        )}
                      </div>

                      <div className="maintenance-field">
                        <label>操作类型</label>
                        <Select
                          value={ingestAction}
                          onChange={(value: IngestAction) => setIngestAction(value)}
                          options={[
                            { value: 'sync', label: '手动入库' },
                            { value: 'rebuild', label: '全量重建' },
                          ]}
                        />
                      </div>

                      <div className="maintenance-field maintenance-danger-field">
                        <label>执行操作</label>
                        {ingestAction === 'rebuild' ? (
                          <Popconfirm
                            title={`确认全量重建${importSourceID ? '当前日志源' : '全部日志源'}？`}
                            description={rebuildConfirmDescription}
                            okText="确认全量重建"
                            cancelText="取消"
                            onConfirm={() => void triggerIngestAction()}
                          >
                            <Button danger icon={<WarningOutlined />} loading={loading} disabled={ingestActionDisabled}>
                              {buttonLabel}
                            </Button>
                          </Popconfirm>
                        ) : (
                          <Button type="primary" icon={<SyncOutlined />} onClick={() => void triggerIngestAction()} loading={loading} disabled={ingestActionDisabled}>
                            {buttonLabel}
                          </Button>
                        )}
                      </div>
                    </div>

                    <div className="maintenance-card-summary maintenance-action-summary">
                      <Text>{actionSummary}</Text>
                    </div>
                  </div>

                  <div className="maintenance-run-card maintenance-run-card--upgrade">
                    <div className="maintenance-card-head">
                      <div>
                        <span className="maintenance-card-kicker"><CloudDownloadOutlined /> 手动升级</span>
                        <strong>版本升级</strong>
                      </div>
                      <Tag color={upgradeView.statusTone}>
                        {upgradeRestarting ? '服务重启中' : upgradeView.stateText}
                      </Tag>
                    </div>

                    {upgradeRestarting && (
                      <Text type="warning" style={{ display: 'block', marginTop: 4 }}>
                        服务正在重启，请稍候。重启完成后请手动刷新页面以加载新版本。
                      </Text>
                    )}

                    <div className="maintenance-upgrade-grid">
                      <div className="maintenance-field">
                        <label>当前版本</label>
                        <Text className="maintenance-value">{upgradeView.currentVersion}</Text>
                      </div>
                      <div className="maintenance-field">
                        <label>更新状态</label>
                        <Text className="maintenance-value">{upgradeView.stateText}</Text>
                      </div>
                      <div className="maintenance-field">
                        <label>在线检查</label>
                        <Button
                          icon={<ReloadOutlined />}
                          onClick={() => void checkUpgrade()}
                          loading={upgradeLoading}
                          disabled={!canCheckUpgrade}
                        >
                          检查更新
                        </Button>
                      </div>
                      <div className="maintenance-field">
                        <label>离线升级包</label>
                        <Upload
                          accept=".rpm,.deb"
                          maxCount={1}
                          fileList={upgradeFile ? [{ uid: upgradeFile.name, name: upgradeFile.name, status: 'done' }] : []}
                          beforeUpload={(file) => {
                            if (!/^fwlog-upgrade(?:-v.+\.x86_64\.rpm|_.+_amd64\.deb)$/i.test(file.name)) {
                              message.error('只允许 fwlog-upgrade RPM 或 DEB 包');
                              return Upload.LIST_IGNORE;
                            }
                            setUpgradeFile(file);
                            return false;
                          }}
                          onRemove={() => {
                            setUpgradeFile(null);
                            return true;
                          }}
                        >
                          <Button icon={<UploadOutlined />}>选择升级包</Button>
                        </Upload>
                      </div>
                      <div className="maintenance-field">
                        <label>安装操作</label>
                        <Button type="primary" disabled={!upgradeFile} loading={upgradeLoading} onClick={() => void uploadUpgrade()}>
                          上传并安装
                        </Button>
                      </div>
                    </div>

                    <div className="maintenance-card-summary">
                      <Text type={upgradeCheckError || upgradeView.state === 'failed' || upgradeView.state === 'asset_missing' ? 'danger' : upgradeView.state === 'available' ? 'warning' : upgradeView.state === 'latest' || upgradeView.state === 'succeeded' ? 'success' : 'secondary'}>
                        {upgradeSummary}
                      </Text>
                      <Text type="secondary">{upgradeView.lastCheckedText} · {upgradeView.sourceText}</Text>
                      {upgradeView.showUpgradeAction ? (
                        <Popconfirm
                          title="确认升级并重启服务？"
                          description={upgradeView.latestVersion}
                          okText="确认升级"
                          cancelText="取消"
                          onConfirm={() => void runUpgrade()}
                        >
                          <Button
                            type="primary"
                            icon={<CloudDownloadOutlined />}
                            loading={upgradeLoading || upgradeStatus?.state === 'running'}
                            disabled={!canRunUpgrade}
                          >
                            {upgradeView.upgradeButtonText}
                          </Button>
                        </Popconfirm>
                      ) : null}
                      {upgradeView.state === 'succeeded' && (
                        <Text type="warning" style={{ display: 'block', marginTop: 4 }}>
                          升级完成，请 <a onClick={() => window.location.reload()}>点击刷新</a> 加载新版本。
                        </Text>
                      )}
                    </div>
                  </div>
                </section>
              ),
            },
            {
              key: 'security',
              label: tabLabel(<SafetyCertificateOutlined />, '登录'),
              children: (
                <section className="ops-section maintenance-card">
                  <div className="setting-grid">
                    <div className="setting-fields">
                      <Form.Item name="current_password" label="当前密码" rules={[{ required: true, message: '请输入当前密码' }]}>
                        <Input.Password prefix={<KeyOutlined />} />
                      </Form.Item>
                      <Form.Item name="new_password" label="新密码" rules={[{ required: true, message: '请输入新密码' }, { min: 6, message: '密码至少需要 6 个字符' }]}>
                        <Input.Password prefix={<SafetyCertificateOutlined />} />
                      </Form.Item>
                      <Form.Item
                        name="confirm_new_password"
                        label="确认新密码"
                        dependencies={['new_password']}
                        rules={[
                          { required: true, message: '请再次输入新密码' },
                          ({ getFieldValue }) => ({
                            validator(_, value) {
                              if (!value || getFieldValue('new_password') === value) return Promise.resolve();
                              return Promise.reject(new Error('两次输入的新密码不一致'));
                            },
                          }),
                        ]}
                      >
                        <Input.Password prefix={<SafetyCertificateOutlined />} />
                      </Form.Item>
                      <Space>
                        <Button type="primary" icon={<KeyOutlined />} onClick={() => void changePassword()} loading={loading}>更新密码</Button>
                        <Button icon={<LogoutOutlined />} onClick={onRequireLogin}>退出登录</Button>
                      </Space>
                    </div>
                    <aside className="setting-summary setting-summary-amber">
                      <SafetyCertificateOutlined />
                      <Text type="secondary">当前会话</Text>
                      <strong>本机控制台</strong>
                    </aside>
                  </div>
                </section>
              ),
            },
          ]}
        />
      </Form>
    </div>
  );
}
