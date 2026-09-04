import React from 'react';
import {
  ClockCircleOutlined,
  CloudDownloadOutlined,
  DatabaseOutlined,
  DeleteOutlined,
  EditOutlined,
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
import { Button, DatePicker, Empty, Form, Input, InputNumber, Modal, Popconfirm, Select, Space, Switch, Tabs, Tag, TimePicker, Tooltip, Typography, Upload, message } from 'antd';
import dayjs, { type Dayjs } from 'dayjs';
import { apiGet, apiPost, apiUpload, type UpgradeCheckResponse, type UpgradeStatus } from '../api';
import { ThreatIntelligenceSettingsPanel } from '../components/ThreatIntelligenceSettingsPanel';
import { buildUpgradeView, isSupportedUpgradeVersion } from '../upgradePresentation';

const { Text } = Typography;

type SystemMaintenancePageProps = {
  onRequireLogin: () => void;
};

type LogSourceSetting = {
  source_id?: string;
  serial_number?: string;
  log_tag?: string;
  log_dir?: string;
  source_type?: 'file' | 'rsyslog' | string;
  listen_protocol?: 'udp' | 'tcp' | string;
  listen_host?: string;
  listen_port?: number | string;
  spool_dir?: string;
  client_ip?: string;
  archive_dir?: string;
  archive_retention_days?: number | string;
  enabled?: boolean;
};

type ReceiverStatus = {
  source_id?: string;
  protocol?: string;
  address?: string;
  port?: number;
  spool_dir?: string;
  client_ip?: string;
  running?: boolean;
  error?: string;
  last_client_ip?: string;
  last_received_at?: string;
  received_messages?: number;
  archive_error?: string;
  last_archive_at?: string;
};

type ReceiverStatusMap = Record<string, ReceiverStatus>;

type SourceEditorState = {
  type: 'file' | 'rsyslog';
  index: number | null;
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
  const serialNumber = String(source.serial_number || '').trim();
  const sourceType = source.source_type === 'rsyslog' ? 'rsyslog' : 'file';
  if (sourceType === 'rsyslog') {
    const spoolDir = source.spool_dir || source.log_dir || defaultSpoolDir(sourceID);
    const archiveDir = String(source.archive_dir || '').trim();
    return {
      ...source,
      source_id: sourceID,
      serial_number: serialNumber,
      source_type: 'rsyslog',
      listen_protocol: source.listen_protocol?.toLowerCase() === 'tcp' ? 'tcp' : 'udp',
      listen_host: source.listen_host || '0.0.0.0',
      listen_port: Number(source.listen_port || 5514),
      spool_dir: spoolDir,
      client_ip: String(source.client_ip || '').trim(),
      archive_dir: archiveDir,
      archive_retention_days: Math.max(0, Number(source.archive_retention_days || 0)),
      log_dir: archiveDir || spoolDir,
      enabled: source.enabled !== false,
    };
  }
  return {
    ...source,
    source_id: sourceID,
    serial_number: serialNumber,
    source_type: 'file',
    enabled: source.enabled !== false,
  };
}

function normalizeLogSourcesForForm(sources: LogSourceSetting[]) {
  return sources.map((source, index) => normalizeLogSourceSetting(source, index));
}

function nextSourceID(prefix: string, sources: LogSourceSetting[]) {
  const used = new Set(sources.map((source) => source.source_id));
  let suffix = sources.length + 1;
  while (used.has(`${prefix}-${suffix}`)) suffix += 1;
  return `${prefix}-${suffix}`;
}

function formatReceiverTime(value?: string) {
  if (!value || value.startsWith('0001-')) return '';
  const parsed = dayjs(value);
  return parsed.isValid() ? parsed.format('YYYY-MM-DD HH:mm:ss') : '';
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
  const [sourceForm] = Form.useForm<LogSourceSetting>();
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
  const [receiverStatuses, setReceiverStatuses] = React.useState<ReceiverStatusMap>({});
  const [sourceEditor, setSourceEditor] = React.useState<SourceEditorState | null>(null);
  const [sourceSaving, setSourceSaving] = React.useState(false);
  const [upgradeRestarting, setUpgradeRestarting] = React.useState(false);
  const [upgradeFile, setUpgradeFile] = React.useState<File | null>(null);
  const autoScanEnabled = Form.useWatch('auto_scan_enabled', form);
  const autoScanTimes = Form.useWatch('auto_scan_times', form);

  const loadReceiverStatus = React.useCallback(async () => {
    try {
      const status = await apiGet<ReceiverStatusMap>('/api/receiver/status');
      setReceiverStatuses(status || {});
    } catch {
      setReceiverStatuses({});
    }
  }, []);

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
      const { log_sources: _ignoredLogSources, ...formSettings } = settings;
      form.setFieldsValue({
        ...formSettings,
        cidr_aliases: cidrAliases,
        auto_scan_enabled: settings.auto_scan_enabled === true || settings.auto_scan_enabled === 'true',
        auto_scan_mode: settings.auto_scan_mode || 'daily',
        auto_scan_times: autoScanTimeValue(settings.auto_scan_times),
        auto_scan_timezone: settings.auto_scan_timezone || 'Asia/Shanghai',
        auto_scan_interval_sec: Number(settings.auto_scan_interval_sec || 3600),
      });
      await loadReceiverStatus();
    } catch (error) {
      message.error(error instanceof Error ? error.message : '加载设置失败');
    } finally {
      setLoading(false);
    }
  }, [form, loadReceiverStatus]);

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
  const autoScanSummary = `自动扫描：${autoScanEnabled ? '已开启' : '已关闭'}；每天 ${autoScanDisplay} 扫描全部已启用日志来源，只导入尚未完成的日期。`;
  const upgradeSummary = `版本状态：当前版本 ${upgradeView.currentVersion}；${upgradeCheckError || upgradeView.message}`;

  async function persistLogSources(next: LogSourceSetting[]) {
    const normalized = normalizeLogSourcesForForm(next);
    const response = await apiPost<Settings>('/api/settings', {
      log_sources: JSON.stringify(normalized),
    });
    const persisted = normalizeLogSourcesForForm(parseLogSources(response.log_sources));
    const applied = persisted.length || normalized.length === 0 ? persisted : normalized;
    setSavedLogSources(applied);
    if (importSourceID && !applied.some((source) => source.enabled !== false && source.source_id === importSourceID)) {
      setImportSourceID('');
    }
    await loadReceiverStatus();
    return applied;
  }

  function openSourceEditor(type: 'file' | 'rsyslog', index: number | null = null) {
    const source = index === null ? undefined : savedLogSources[index];
    const sourceID = source?.source_id || nextSourceID(type === 'rsyslog' ? 'rsyslog' : 'source', savedLogSources);
    sourceForm.resetFields();
    sourceForm.setFieldsValue(source || (type === 'rsyslog' ? {
      source_id: sourceID,
      serial_number: '',
      log_tag: '',
      source_type: 'rsyslog',
      listen_protocol: 'udp',
      listen_host: '0.0.0.0',
      listen_port: 5514,
      client_ip: '',
      spool_dir: defaultSpoolDir(sourceID),
      archive_dir: '',
      archive_retention_days: 0,
      enabled: true,
    } : {
      source_id: sourceID,
      serial_number: '',
      log_tag: '',
      log_dir: '',
      source_type: 'file',
      enabled: true,
    }));
    setSourceEditor({ type, index });
  }

  async function saveSourceEditor() {
    if (!sourceEditor) return;
    try {
      const values = await sourceForm.validateFields();
      const source = normalizeLogSourceSetting({
        ...values,
        source_type: sourceEditor.type,
        listen_host: sourceEditor.type === 'rsyslog' ? '0.0.0.0' : undefined,
      }, sourceEditor.index ?? savedLogSources.length);
      const next = sourceEditor.index === null
        ? [...savedLogSources, source]
        : savedLogSources.map((item, index) => (index === sourceEditor.index ? source : item));
      setSourceSaving(true);
      await persistLogSources(next);
      setSourceEditor(null);
      message.success(sourceEditor.index === null ? '日志来源已添加并应用' : '日志来源已更新并应用');
    } catch (error) {
      if (error && typeof error === 'object' && 'errorFields' in error) return;
      message.error(error instanceof Error ? error.message : '保存日志来源失败');
    } finally {
      setSourceSaving(false);
    }
  }

  async function toggleLogSource(index: number, enabled: boolean) {
    const previous = savedLogSources;
    const next = previous.map((source, sourceIndex) => (sourceIndex === index ? { ...source, enabled } : source));
    setSavedLogSources(next);
    setSourceSaving(true);
    try {
      await persistLogSources(next);
      message.success(enabled ? '日志来源已启用' : '日志来源已停用');
    } catch (error) {
      setSavedLogSources(previous);
      message.error(error instanceof Error ? error.message : '更新日志来源状态失败');
    } finally {
      setSourceSaving(false);
    }
  }

  async function deleteLogSource(index: number) {
    const next = savedLogSources.filter((_, sourceIndex) => sourceIndex !== index);
    setSourceSaving(true);
    try {
      await persistLogSources(next);
      message.success('日志来源配置已删除');
    } catch (error) {
      message.error(error instanceof Error ? error.message : '删除日志来源失败');
    } finally {
      setSourceSaving(false);
    }
  }

  const save = async () => {
    try {
      setLoading(true);
      const values = form.getFieldsValue();
      const { log_sources: _ignoredLogSources, ...settingsValues } = values;
      const firstSource = savedLogSources[0];
      await apiPost('/api/settings', {
        ...settingsValues,
        cidr_aliases: JSON.stringify(values.cidr_aliases || []),
        log_dir: firstSource?.log_dir || values.log_dir,
        log_tag: firstSource?.log_tag || values.log_tag,
        auto_scan_enabled: String(Boolean(values.auto_scan_enabled)),
        auto_scan_mode: values.auto_scan_mode || 'daily',
        auto_scan_times: formatAutoScanTime(values.auto_scan_times),
        auto_scan_timezone: values.auto_scan_timezone || 'Asia/Shanghai',
        auto_scan_interval_sec: String(Number(values.auto_scan_interval_sec) || 3600),
      });
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
    : '全部已启用日志来源';
  const actionLabel = ingestAction === 'sync' ? '导入新增日志' : '重新导入所选日期';
  const dateScopeLabel = dateMode === 'all'
    ? '所有历史日期'
    : dateMode === 'single'
      ? singleDate?.format('YYYY-MM-DD') || '未选择日期'
      : dateRange?.[0] && dateRange?.[1]
        ? `${dateRange[0].format('YYYY-MM-DD')} 至 ${dateRange[1].format('YYYY-MM-DD')}`
        : '未选择日期范围';
  const buttonLabel = ingestAction === 'sync' ? '开始导入' : '开始重新导入';
  const actionSummary = `本次处理：日志来源 = ${selectedSourceLabel}；日期 = ${dateScopeLabel}；处理方式 = ${actionLabel}。`;
  const rebuildConfirmDescription = `将先清除「${selectedSourceLabel}」在「${dateScopeLabel}」已有的入库结果，再重新处理所选日期。该操作耗时可能较长。`;
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
      message.success(ingestAction === 'sync' ? '新增日志导入任务已开始' : '重新导入任务已开始');
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
        message.warning('发现新版本，但发布文件不完整，暂不能升级');
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
      message.error('请先检查更新并确认发布文件完整');
      return;
    }
    if (!isSupportedUpgradeVersion(version)) {
      message.error('检查到的升级版本必须使用 vX.Y.Z 或 vX.Y.Z.N 格式');
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
      message.success('本地升级包已上传，安装任务已开始');
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
          <span className="eyebrow">系统管理</span>
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
              label: tabLabel(<FolderOpenOutlined />, '日志来源'),
              children: (
                <section className="ops-section maintenance-card maintenance-panel">
                  <div className="source-list-editor">
                    <div className="maintenance-panel-toolbar">
                      <Text className="maintenance-panel-note" type="secondary">添加、启停或修改后立即应用</Text>
                      <Space wrap>
                        <Button icon={<PlusOutlined />} onClick={() => openSourceEditor('file')}>
                          添加文件目录源
                        </Button>
                        <Button type="primary" icon={<PlusOutlined />} onClick={() => openSourceEditor('rsyslog')}>
                          添加 RSyslog 接收源
                        </Button>
                      </Space>
                    </div>

                    <div className="source-management-list">
                      <div className="source-management-row source-management-row--header">
                        <span>设备序列号</span>
                        <span>显示名称</span>
                        <span>类型</span>
                        <span>发送端 / 目录</span>
                        <span>接收与压缩</span>
                        <span>状态</span>
                        <span>操作</span>
                      </div>
                      {savedLogSources.length === 0 ? (
                        <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无日志来源" />
                      ) : savedLogSources.map((source, index) => {
                        const sourceID = source.source_id || `source-${index + 1}`;
                        const isRSyslog = source.source_type === 'rsyslog';
                        const status = receiverStatuses[sourceID];
                        const lastReceivedAt = formatReceiverTime(status?.last_received_at);
                        const lastArchiveAt = formatReceiverTime(status?.last_archive_at);
                        const retentionDays = Number(source.archive_retention_days || 0);
                        const sourceError = status?.error || status?.archive_error;
                        return (
                          <div className="source-management-row" key={sourceID}>
                            <div className="source-management-cell" data-label="设备序列号">
                              <span className="source-management-mobile-label">设备序列号</span>
                              <strong title={source.serial_number || ''}>{source.serial_number || '-'}</strong>
                              <Text type="secondary" title={sourceID}>来源标识：{sourceID}</Text>
                            </div>
                            <div className="source-management-cell" data-label="显示名称">
                              <span className="source-management-mobile-label">显示名称</span>
                              <strong title={source.log_tag || ''}>{source.log_tag || '-'}</strong>
                            </div>
                            <div className="source-management-cell" data-label="类型">
                              <span className="source-management-mobile-label">类型</span>
                              <Tag color={isRSyslog ? 'processing' : 'default'}>{isRSyslog ? 'RSyslog 接收源' : '文件目录源'}</Tag>
                            </div>
                            <div className="source-management-cell source-management-detail" data-label="发送端 / 目录">
                              <span className="source-management-mobile-label">发送端 / 目录</span>
                              <strong title={isRSyslog ? source.client_ip : source.log_dir}>
                                {isRSyslog ? source.client_ip || '接受任意发送端' : source.log_dir || '-'}
                              </strong>
                              {isRSyslog && <Text type="secondary" title={source.spool_dir}>接收文件：{source.spool_dir || '-'}</Text>}
                            </div>
                            <div className="source-management-cell source-management-detail" data-label="接收与压缩">
                              <span className="source-management-mobile-label">接收与压缩</span>
                              {isRSyslog ? (
                                <>
                                  <strong>{String(source.listen_protocol || 'udp').toUpperCase()} · {Number(source.listen_port || 5514)}</strong>
                                  <Text type="secondary">最近发送端：{status?.last_client_ip || '尚未收到'}</Text>
                                  <Text type="secondary">接收：{status?.received_messages || 0} 条{lastReceivedAt ? ` · ${lastReceivedAt}` : ''}</Text>
                                  <Text type="secondary">
                                    压缩文件：{source.archive_dir ? '保存到指定目录' : '保留在接收目录'} · {retentionDays === 0 ? '永久保留' : `${retentionDays} 天`}{lastArchiveAt ? ` · ${lastArchiveAt}` : ''}
                                  </Text>
                                </>
                              ) : <Text type="secondary">文件扫描</Text>}
                            </div>
                            <div className="source-management-cell" data-label="状态">
                              <span className="source-management-mobile-label">状态</span>
                              {source.enabled === false ? (
                                <Text type="secondary">-</Text>
                              ) : sourceError ? (
                                <Tooltip title={sourceError}><Tag color="error">异常</Tag></Tooltip>
                              ) : isRSyslog ? (
                                <Tag color={status?.running ? 'success' : 'warning'}>{status?.running ? '运行中' : '等待启动'}</Tag>
                              ) : <Tag color="success">已启用</Tag>}
                            </div>
                            <div className="source-management-actions" data-label="操作">
                              <Switch
                                size="small"
                                checked={source.enabled !== false}
                                loading={sourceSaving}
                                aria-label={`${sourceID} 启用日志接收`}
                                onChange={(checked) => void toggleLogSource(index, checked)}
                              />
                              <Tooltip title="编辑">
                                <Button
                                  type="text"
                                  icon={<EditOutlined />}
                                  aria-label="编辑日志来源"
                                  disabled={sourceSaving}
                                  onClick={() => openSourceEditor(isRSyslog ? 'rsyslog' : 'file', index)}
                                />
                              </Tooltip>
                              <Popconfirm
                                title="删除这个日志来源？"
                                description="只删除配置，不会删除已接收或已压缩的文件。"
                                okText="删除"
                                cancelText="取消"
                                onConfirm={() => void deleteLogSource(index)}
                              >
                                <Tooltip title="删除">
                                  <Button type="text" danger icon={<DeleteOutlined />} aria-label="删除日志来源" disabled={sourceSaving} />
                                </Tooltip>
                              </Popconfirm>
                            </div>
                          </div>
                        );
                      })}
                    </div>
                  </div>
                </section>
              ),
            },
            {
              key: 'ip',
              label: tabLabel(<GlobalOutlined />, 'CIDR 别名'),
              children: (
                <section className="ops-section maintenance-card maintenance-panel">
                  <div className="maintenance-panel-toolbar">
                    <Text className="maintenance-panel-note" type="secondary">管理地理位置数据、IP 映射和网段显示名称</Text>
                    <Button icon={<ReloadOutlined />} onClick={() => void trigger('/api/ip-data/reload', 'IP 映射数据已重新加载')} loading={loading}>
                      重新加载
                    </Button>
                  </div>
                  <div className="setting-grid setting-grid--single">
                    <div className="setting-fields">
                      <Form.Item name="custom_ip_map_path" label="IP 映射文件">
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
                              <span>显示名称</span>
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
                                  title="删除这个 CIDR 别名？"
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
                  </div>
                </section>
              ),
            },
            {
              key: 'ingest',
              label: tabLabel(<WarningOutlined />, '日志入库'),
              children: (
                <section className="ops-section maintenance-card maintenance-panel">
                  <Text className="maintenance-panel-note" type="secondary">自动扫描和手动处理历史日志</Text>
                  <div className="maintenance-panel-stack">
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
                        <label>日志来源范围</label>
                        <Text className="maintenance-value">全部已启用日志来源</Text>
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
                        <span className="maintenance-card-kicker"><SyncOutlined /> 日志导入</span>
                        <strong>导入操作</strong>
                      </div>
                    </div>

                    <div className="maintenance-run-grid">
                      <div className="maintenance-field">
                        <label>日志来源</label>
                        <Select
                          value={importSourceID}
                          onChange={setImportSourceID}
                          options={[
                            { value: '', label: '全部已启用日志来源' },
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
                          <Text type="secondary">系统会扫描所选日志来源下已有的历史日志</Text>
                        )}
                      </div>

                      <div className="maintenance-field">
                        <label>操作类型</label>
                        <Select
                          value={ingestAction}
                          onChange={(value: IngestAction) => setIngestAction(value)}
                          options={[
                            { value: 'sync', label: '导入新增日志' },
                            { value: 'rebuild', label: '重新导入所选日期' },
                          ]}
                        />
                      </div>

                      <div className="maintenance-field maintenance-danger-field">
                        <label>开始处理</label>
                        {ingestAction === 'rebuild' ? (
                          <Popconfirm
                            title={`确认重新导入${importSourceID ? '当前日志来源' : '全部日志来源'}？`}
                            description={rebuildConfirmDescription}
                            okText="确认重新导入"
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

                  </div>
                </section>
              ),
            },
            {
              key: 'upgrade',
              label: tabLabel(<CloudDownloadOutlined />, '程序升级'),
              children: (
                <section className="ops-section maintenance-card maintenance-panel">
                  <Text className="maintenance-panel-note" type="secondary">检查版本并安装本地升级包</Text>
                  <div className="maintenance-panel-stack">
                  <div className="maintenance-run-card maintenance-run-card--upgrade">
                    <div className="maintenance-card-head">
                      <div>
                        <span className="maintenance-card-kicker"><CloudDownloadOutlined /> 程序更新</span>
                        <strong>升级操作</strong>
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
                        <label>本地升级包</label>
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
                  </div>
                </section>
              ),
            },
            {
              key: 'threat-intelligence',
              label: tabLabel(<SafetyCertificateOutlined />, '威胁情报'),
              children: (
                <section className="ops-section maintenance-card maintenance-panel">
                  <Text className="maintenance-panel-note" type="secondary">配置第三方平台 API Key 或 Token；没有平台账号请保持停用，不要填写 FWLOG 登录密码</Text>
                  <ThreatIntelligenceSettingsPanel />
                </section>
              ),
            },
            {
              key: 'security',
              label: tabLabel(<SafetyCertificateOutlined />, '账号安全'),
              children: (
                <section className="ops-section maintenance-card maintenance-panel">
                  <Text className="maintenance-panel-note" type="secondary">修改管理员密码并管理当前登录会话</Text>
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
      <Modal
        open={sourceEditor !== null}
        title={sourceEditor?.index === null
          ? sourceEditor.type === 'rsyslog' ? '添加 RSyslog 接收源' : '添加文件目录源'
          : sourceEditor?.type === 'rsyslog' ? '编辑 RSyslog 接收源' : '编辑文件目录源'}
        width={760}
        okText="保存并应用"
        cancelText="取消"
        confirmLoading={sourceSaving}
        maskClosable={!sourceSaving}
        onOk={() => void saveSourceEditor()}
        onCancel={() => {
          if (!sourceSaving) setSourceEditor(null);
        }}
      >
        <Form form={sourceForm} layout="vertical" preserve={false} className="source-editor-form">
          <div className="source-editor-grid">
            <Form.Item
              name="serial_number"
              label="设备序列号"
            >
              <Input prefix={<TagsOutlined />} placeholder="例如：SN-FW-A-001" />
            </Form.Item>
            <Form.Item
              name="source_id"
              label="来源标识"
              rules={[
                { required: true, message: '请输入来源标识' },
                { pattern: /^[A-Za-z0-9._-]+$/, message: '只允许字母、数字、点、下划线和连字符' },
              ]}
            >
              <Input prefix={<DatabaseOutlined />} placeholder="device-id" />
            </Form.Item>
            <Form.Item name="log_tag" label="显示名称" rules={[{ required: true, message: '请输入显示名称' }]}>
              <Input prefix={<TagsOutlined />} placeholder="例如：出口防火墙" />
            </Form.Item>

            {sourceEditor?.type === 'rsyslog' ? (
              <>
                <Form.Item
                  className="source-editor-field-wide"
                  name="client_ip"
                  label="允许的发送端地址（可选）"
                  rules={[
                    { pattern: /^(?:[0-9]{1,3}(?:\.[0-9]{1,3}){3}(?:\/(?:[0-9]|[12][0-9]|3[0-2]))?|[0-9A-Fa-f:.]+(?:\/(?:[0-9]|[1-9][0-9]|1[01][0-9]|12[0-8]))?)$/, message: '请输入有效的 IPv4、IPv6 或 CIDR' },
                  ]}
                >
                  <Input prefix={<GlobalOutlined />} placeholder="192.168.10.20 或 192.168.10.0/24" />
                </Form.Item>
                <Form.Item name="listen_protocol" label="接收协议" rules={[{ required: true }]}>
                  <Select options={[
                    { value: 'udp', label: 'UDP' },
                    { value: 'tcp', label: 'TCP' },
                  ]} />
                </Form.Item>
                <Form.Item name="listen_port" label="监听端口" rules={[{ required: true, message: '请输入监听端口' }]}>
                  <InputNumber min={1} max={65535} prefix={<GlobalOutlined />} placeholder="默认 5514" />
                </Form.Item>
                <Form.Item
                  className="source-editor-field-wide"
                  name="spool_dir"
                  label="接收文件保存目录"
                  rules={[
                    { required: true, whitespace: true, message: '请输入接收文件保存目录' },
                    { pattern: /^\//, message: '请输入绝对路径' },
                  ]}
                >
                  <Input prefix={<FolderOpenOutlined />} placeholder="/data/fwlog/received/device-id" />
                </Form.Item>
                <Form.Item
                  className="source-editor-field-wide"
                  name="archive_dir"
                  label="压缩文件保存目录（可选）"
                  extra="留空时压缩文件保留在接收文件保存目录"
                  rules={[{ pattern: /^(?:\/.*)?$/, message: '请输入绝对路径或留空' }]}
                >
                  <Input prefix={<FolderOpenOutlined />} placeholder="可选，例如 /data/fwlog/archive/device-id" />
                </Form.Item>
                <Form.Item
                  name="archive_retention_days"
                  label="压缩文件保留天数"
                  extra="0 表示永久保留"
                  rules={[{ required: true, message: '请输入压缩文件保留天数' }]}
                >
                  <InputNumber min={0} max={3650} precision={0} />
                </Form.Item>
                <Form.Item name="listen_host" hidden>
                  <Input />
                </Form.Item>
              </>
            ) : (
              <Form.Item
                className="source-editor-field-wide"
                name="log_dir"
                label="文件目录"
                rules={[
                  { required: true, whitespace: true, message: '请输入文件目录' },
                  { pattern: /^\//, message: '请输入绝对路径' },
                ]}
              >
                <Input prefix={<FolderOpenOutlined />} placeholder="/data/device_fw_log" />
              </Form.Item>
            )}

            <Form.Item className="source-editor-field-wide" name="enabled" label="启用日志接收" valuePropName="checked">
              <Switch checkedChildren="启用" unCheckedChildren="停用" />
            </Form.Item>
          </div>
        </Form>
      </Modal>
    </div>
  );
}
