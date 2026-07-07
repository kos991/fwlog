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
  WarningOutlined,
} from '@ant-design/icons';
import { Button, DatePicker, Form, Input, Popconfirm, Space, Switch, Tabs, Tag, TimePicker, Typography, message } from 'antd';
import dayjs, { type Dayjs } from 'dayjs';
import { apiGet, apiPost, type UpgradeCheckResponse, type UpgradeStatus } from '../api';
import { buildUpgradeView } from '../upgradePresentation';

const { Text } = Typography;

type SystemMaintenancePageProps = {
  onRequireLogin: () => void;
};

type LogSourceSetting = {
  source_id?: string;
  log_tag?: string;
  log_dir?: string;
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
  upgrade_auto_check_enabled?: boolean | string;
  current_password?: string;
  new_password?: string;
  confirm_new_password?: string;
};

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
  const [upgradeAutoCheckSaving, setUpgradeAutoCheckSaving] = React.useState(false);
  const [upgradeStatus, setUpgradeStatus] = React.useState<UpgradeStatus | null>(null);
  const [upgradeCheck, setUpgradeCheck] = React.useState<UpgradeCheckResponse | null>(null);
  const [upgradeCheckError, setUpgradeCheckError] = React.useState('');
  const [upgradeLastCheckedAt, setUpgradeLastCheckedAt] = React.useState<Date | null>(null);
  const [rebuildDate, setRebuildDate] = React.useState<Dayjs | null>(dayjs());
  const [fullRebuild, setFullRebuild] = React.useState(false);
  const autoUpgradeCheckStartedRef = React.useRef(false);
  const geoipPath = Form.useWatch('geoip_db_path', form);
  const customIpPath = Form.useWatch('custom_ip_map_path', form);
  const autoScanEnabled = Form.useWatch('auto_scan_enabled', form);
  const upgradeAutoCheckEnabled = Form.useWatch('upgrade_auto_check_enabled', form);

  const load = React.useCallback(async () => {
    try {
      setLoading(true);
      const settings = await apiGet<Settings>('/api/settings');
      const parsedLogSources = parseLogSources(settings.log_sources);
      const logSources = parsedLogSources.length ? parsedLogSources : [
        {
          source_id: 'default',
          log_tag: settings.log_tag || '深信服 NAT',
          log_dir: settings.log_dir || '/data/sangfor_fw_log',
          enabled: true,
        },
      ];
      const cidrAliases = parseCidrAliases(settings.cidr_aliases);
      form.setFieldsValue({
        ...settings,
        log_sources: logSources,
        cidr_aliases: cidrAliases,
        auto_scan_enabled: settings.auto_scan_enabled === true || settings.auto_scan_enabled === 'true',
        auto_scan_mode: 'daily',
        auto_scan_times: autoScanTimeValue(settings.auto_scan_times),
        auto_scan_timezone: settings.auto_scan_timezone || 'Asia/Shanghai',
        auto_scan_interval_sec: Number(settings.auto_scan_interval_sec || 3600),
        upgrade_auto_check_enabled: settings.upgrade_auto_check_enabled === true || settings.upgrade_auto_check_enabled === 'true',
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
    const timer = window.setInterval(() => {
      void loadUpgradeStatus().catch(() => undefined);
    }, 3000);
    return () => window.clearInterval(timer);
  }, [loadUpgradeStatus, upgradeStatus?.state]);

  const upgradeView = buildUpgradeView({
    status: upgradeStatus,
    check: upgradeCheck,
    isChecking: upgradeLoading,
    lastCheckedAt: upgradeLastCheckedAt,
    autoCheckEnabled: upgradeAutoCheckEnabled === true,
  });
  const canCheckUpgrade = upgradeStatus?.state !== 'running' && !upgradeLoading;
  const canRunUpgrade = upgradeView.showUpgradeAction && upgradeStatus?.state !== 'running' && !upgradeLoading;

  const save = async () => {
    try {
      setLoading(true);
      const values = form.getFieldsValue();
      const logSources = parseLogSources(values.log_sources);
      const firstSource = logSources[0];
      await apiPost('/api/settings', {
        ...values,
        cidr_aliases: JSON.stringify(values.cidr_aliases || []),
        log_sources: JSON.stringify(logSources),
        log_dir: firstSource?.log_dir || values.log_dir,
        log_tag: firstSource?.log_tag || values.log_tag,
        auto_scan_enabled: String(Boolean(values.auto_scan_enabled)),
        auto_scan_mode: 'daily',
        auto_scan_times: formatAutoScanTime(values.auto_scan_times),
        auto_scan_timezone: values.auto_scan_timezone || 'Asia/Shanghai',
        auto_scan_interval_sec: '86400',
        upgrade_auto_check_enabled: String(Boolean(values.upgrade_auto_check_enabled)),
      });
      message.success('设置已保存');
    } catch (error) {
      message.error(error instanceof Error ? error.message : '保存设置失败');
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

  const triggerRebuild = async () => {
    if (fullRebuild) {
      await trigger('/api/rebuild', '已触发全量重建');
      return;
    }
    if (!rebuildDate) {
      message.error('请先选择重建日期');
      return;
    }
    await trigger(`/api/rebuild?date=${rebuildDate.format('YYYY-MM-DD')}`, '已触发指定日期重建');
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

  React.useEffect(() => {
    if (upgradeAutoCheckEnabled !== true) return;
    if (autoUpgradeCheckStartedRef.current) return;
    if (upgradeStatus?.state === 'running') return;
    autoUpgradeCheckStartedRef.current = true;
    void checkUpgrade();
  }, [upgradeAutoCheckEnabled, upgradeStatus?.state]);

  const saveUpgradeAutoCheckEnabled = async (checked: boolean) => {
    const previous = upgradeAutoCheckEnabled === true;
    if (checked) autoUpgradeCheckStartedRef.current = true;
    form.setFieldsValue({ upgrade_auto_check_enabled: checked });
    try {
      setUpgradeAutoCheckSaving(true);
      await apiPost('/api/settings', { upgrade_auto_check_enabled: String(checked) });
      message.success(checked ? '已开启自动检查更新' : '已关闭自动检查更新');
      if (checked) {
        void checkUpgrade();
      }
    } catch (error) {
      autoUpgradeCheckStartedRef.current = previous;
      form.setFieldsValue({ upgrade_auto_check_enabled: previous });
      message.error(error instanceof Error ? error.message : '保存自动检查设置失败');
    } finally {
      setUpgradeAutoCheckSaving(false);
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
                            <strong>日志目录</strong>
                          </div>
                          <Button
                            icon={<PlusOutlined />}
                            onClick={() => add({ source_id: `source-${fields.length + 1}`, log_tag: '', log_dir: '', enabled: true })}
                          >
                            添加
                          </Button>
                        </div>
                        <div className="source-row source-row-header">
                          <span>设备 ID</span>
                          <span>日志名称</span>
                          <span>日志目录</span>
                          <span>启用</span>
                          <span>操作</span>
                        </div>
                        {fields.map((field) => (
                          <div className="source-row" key={field.key}>
                            <Form.Item name={[field.name, 'source_id']} rules={[{ required: true, message: '必填' }]}>
                              <Input prefix={<DatabaseOutlined />} placeholder="device-id" />
                            </Form.Item>
                            <Form.Item name={[field.name, 'log_tag']} rules={[{ required: true, message: '必填' }]}>
                              <Input prefix={<TagsOutlined />} placeholder="日志名称" />
                            </Form.Item>
                            <Form.Item name={[field.name, 'log_dir']} rules={[{ required: true, message: '必填' }]}>
                              <Input prefix={<FolderOpenOutlined />} placeholder="/data/device_fw_log" />
                            </Form.Item>
                            <Form.Item name={[field.name, 'enabled']} valuePropName="checked">
                              <Switch />
                            </Form.Item>
                            <Popconfirm
                              title="删除这个设备目录？"
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
                      <div className="maintenance-switch-line">
                        <Form.Item name="auto_scan_enabled" valuePropName="checked" noStyle>
                          <Switch />
                        </Form.Item>
                        <Tag color={autoScanEnabled ? 'processing' : 'default'}>{autoScanEnabled ? '已开启' : '已关闭'}</Tag>
                      </div>
                    </div>

                    <div className="maintenance-plan-grid">
                      <div className="maintenance-field">
                        <label>扫描时间</label>
                        <Form.Item name="auto_scan_times" noStyle>
                          <TimePicker format="HH:mm" allowClear={false} />
                        </Form.Item>
                      </div>
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
                        <label>手动入库</label>
                        <Button type="primary" icon={<SyncOutlined />} onClick={() => void trigger('/api/sync', '已开始入库')} loading={loading}>
                          执行
                        </Button>
                      </div>

                      <div className="maintenance-field">
                        <label>重建日期</label>
                        <DatePicker value={rebuildDate} onChange={setRebuildDate} disabled={fullRebuild} />
                      </div>

                      <div className="maintenance-field maintenance-danger-field">
                        <label>重建入库</label>
                        <Popconfirm
                          title={fullRebuild ? '确认全量重建？' : '确认重建该日期？'}
                          description={fullRebuild ? '将重建所有历史日志，耗时较长。' : rebuildDate ? rebuildDate.format('YYYY-MM-DD') : '未选择日期'}
                          okText={fullRebuild ? '确认全量重建' : '确认重建'}
                          cancelText="取消"
                          onConfirm={() => void triggerRebuild()}
                        >
                          <Button danger icon={<WarningOutlined />} loading={loading} disabled={!fullRebuild && !rebuildDate}>
                            {fullRebuild ? '全量重建' : '重建'}
                          </Button>
                        </Popconfirm>
                      </div>
                    </div>

                    <div className="maintenance-rebuild-mode">
                      <div>
                        <strong>全量重建</strong>
                        <Text type="secondary">开启后会重建所有历史日志，不再使用上方日期。</Text>
                      </div>
                      <Switch checked={fullRebuild} onChange={setFullRebuild} />
                    </div>
                  </div>

                  <div className="maintenance-run-card maintenance-run-card--upgrade">
                    <div className="maintenance-card-head">
                      <div>
                        <span className="maintenance-card-kicker"><CloudDownloadOutlined /> 自动升级</span>
                        <strong>版本升级</strong>
                      </div>
                      <Tag color={upgradeStatus?.state === 'running' ? 'processing' : upgradeStatus?.state === 'failed' ? 'error' : upgradeStatus?.state === 'succeeded' ? 'success' : 'default'}>
                        {upgradeView.stateText}
                      </Tag>
                    </div>

                    <div className="maintenance-upgrade-panel">
                      <div className="maintenance-upgrade-summary">
                        <div className="maintenance-upgrade-info">
                          <span>当前版本</span>
                          <strong>{upgradeView.currentVersion}</strong>
                        </div>
                        <div className="maintenance-upgrade-info">
                          <span>更新状态</span>
                          <strong>{upgradeView.stateText}</strong>
                        </div>
                      </div>

                      <div className="maintenance-upgrade-actions">
                        <Button
                          icon={<ReloadOutlined />}
                          onClick={() => void checkUpgrade()}
                          loading={upgradeLoading}
                          disabled={!canCheckUpgrade}
                        >
                          {upgradeView.primaryText}
                        </Button>
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
                      </div>

                      <div className="maintenance-upgrade-auto-row">
                        <div>
                          <strong>自动检查更新</strong>
                          <Text type="secondary">进入设置页时检查新版本，不会自动安装。</Text>
                        </div>
                        <Switch
                          checked={upgradeAutoCheckEnabled === true}
                          loading={upgradeAutoCheckSaving}
                          onChange={(checked) => void saveUpgradeAutoCheckEnabled(checked)}
                        />
                      </div>
                    </div>

                    <div className="maintenance-upgrade-note">
                      <Text type={upgradeCheckError || upgradeView.state === 'failed' || upgradeView.state === 'asset_missing' ? 'danger' : upgradeView.state === 'available' ? 'warning' : upgradeView.state === 'latest' || upgradeView.state === 'succeeded' ? 'success' : 'secondary'}>
                        {upgradeCheckError || upgradeView.message}
                      </Text>
                      <Text type="secondary">{upgradeView.lastCheckedText} · {upgradeView.sourceText}</Text>
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
                      <Form.Item
                        name="new_password"
                        label="新密码"
                        rules={[
                          { required: true, message: '请输入新密码' },
                          { min: 6, message: '密码至少需要 6 个字符' },
                        ]}
                      >
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
                              if (!value || getFieldValue('new_password') === value) {
                                return Promise.resolve();
                              }
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
