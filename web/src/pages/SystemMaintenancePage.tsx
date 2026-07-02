import React from 'react';
import {
  DatabaseOutlined,
  FolderOpenOutlined,
  ReloadOutlined,
  SafetyOutlined,
  SettingOutlined,
  SyncOutlined
} from '@ant-design/icons';
import { Button, Form, Input, Select, Space, Switch, Tabs, Typography, message } from 'antd';
import type { TabsProps } from 'antd';
import { apiGet, apiPost } from '../api';

const { Text } = Typography;

type SystemMaintenancePageProps = {
  onRequireLogin: () => void;
};

type SettingsResponse = Record<string, string>;

type PasswordResponse = {
  authenticated: boolean;
};

type SourceFormValues = {
  source_id: string;
  log_dir: string;
  log_tag: string;
};

type IPDataFormValues = {
  custom_ip_map_path: string;
  geoip_db_path: string;
  ip_map_enabled: boolean;
  geoip_enabled: boolean;
};

type AutoSyncFormValues = {
  auto_scan_enabled: boolean;
  auto_scan_mode: string;
  auto_scan_times: string;
  auto_scan_timezone: string;
  auto_scan_jitter_sec: string;
};

type PasswordFormValues = {
  current_password?: string;
  new_password: string;
};

type IPDataStatus = {
  loaded: boolean;
  custom_ip_map_path: string;
  geoip_db_path: string;
  ip_map_enabled: boolean;
  geoip_enabled: boolean;
  updated_at: string;
  error: string;
};

function MaintenanceSection(props: { title: string; description: string; children: React.ReactNode }) {
  return (
    <section className="maintenance-pane">
      <div className="section-head">
        <h3>{props.title}</h3>
      </div>
      <Text type="secondary" className="maintenance-copy">
        {props.description}
      </Text>
      <div className="maintenance-body">{props.children}</div>
    </section>
  );
}

export function SystemMaintenancePage({ onRequireLogin }: SystemMaintenancePageProps) {
  const [settings, setSettings] = React.useState<SettingsResponse>({});
  const [loading, setLoading] = React.useState(false);
  const [reloadingIP, setReloadingIP] = React.useState(false);
  const [passwordSubmitting, setPasswordSubmitting] = React.useState(false);
  const [sourceForm] = Form.useForm<SourceFormValues>();
  const [ipDataForm] = Form.useForm<IPDataFormValues>();
  const [autoSyncForm] = Form.useForm<AutoSyncFormValues>();
  const [passwordForm] = Form.useForm<PasswordFormValues>();

  const loadSettings = React.useCallback(async () => {
    setLoading(true);
    try {
      const response = await apiGet<SettingsResponse>('/api/settings');
      setSettings(response);
    } catch (error) {
      message.error(error instanceof Error ? error.message : '系统维护数据加载失败');
    } finally {
      setLoading(false);
    }
  }, []);

  React.useEffect(() => {
    void loadSettings();
  }, [loadSettings]);

  React.useEffect(() => {
    sourceForm.setFieldsValue({
      source_id: settings.source_id || 'default',
      log_dir: settings.log_dir || '',
      log_tag: settings.log_tag || ''
    });
    ipDataForm.setFieldsValue({
      custom_ip_map_path: settings.custom_ip_map_path || '',
      geoip_db_path: settings.geoip_db_path || '',
      ip_map_enabled: settings.ip_map_enabled === 'true',
      geoip_enabled: settings.geoip_enabled === 'true'
    });
    autoSyncForm.setFieldsValue({
      auto_scan_enabled: settings.auto_scan_enabled === 'true',
      auto_scan_mode: settings.auto_scan_mode || 'hourly',
      auto_scan_times: settings.auto_scan_times || '01:00',
      auto_scan_timezone: settings.auto_scan_timezone || 'Asia/Shanghai',
      auto_scan_jitter_sec: settings.auto_scan_jitter_sec || '60'
    });
  }, [autoSyncForm, ipDataForm, settings, sourceForm]);

  const saveSettings = async (next: Record<string, unknown>) => {
    try {
      await apiPost('/api/settings', next);
      message.success('设置已保存');
      await loadSettings();
    } catch (error) {
      message.error(error instanceof Error ? error.message : '设置保存失败');
    }
  };

  const reloadIPData = async () => {
    setReloadingIP(true);
    try {
      const response = await apiPost<IPDataStatus>('/api/ip-data/reload');
      if (!response.loaded && response.error) {
        throw new Error(response.error);
      }
      message.success('IP 库已重新加载');
      await loadSettings();
    } catch (error) {
      message.error(error instanceof Error ? error.message : 'IP 库重载失败');
    } finally {
      setReloadingIP(false);
    }
  };

  const updatePassword = async () => {
    try {
      const values = await passwordForm.validateFields();
      setPasswordSubmitting(true);
      const response = await apiPost<PasswordResponse>('/api/password', {
        current_password: values.current_password,
        new_password: values.new_password
      });
      if (!response.authenticated) {
        onRequireLogin();
      }
      message.success('管理员密码已更新');
      passwordForm.resetFields();
    } catch (error) {
      if (error && typeof error === 'object' && 'errorFields' in error) {
        return;
      }
      message.error(error instanceof Error ? error.message : '管理员密码更新失败');
    } finally {
      setPasswordSubmitting(false);
    }
  };

  const items: TabsProps['items'] = [
    {
      key: 'sources',
      label: '日志源',
      icon: <FolderOpenOutlined />,
      children: (
        <MaintenanceSection title="日志源" description="日志目录和日志标识统一在系统维护内管理。">
          <Form
            form={sourceForm}
            layout="vertical"
            className="dense-form"
            onFinish={(values) => void saveSettings(values)}
          >
            <div className="maintenance-grid">
              <Form.Item name="source_id" label="日志源标识">
                <Input placeholder="source_id" />
              </Form.Item>
              <Form.Item name="log_tag" label="日志标识">
                <Input placeholder="日志标识" />
              </Form.Item>
            </div>
            <Form.Item name="log_dir" label="日志目录">
              <Input placeholder="/data/sangfor_fw_log" />
            </Form.Item>
            <Space>
              <Button type="primary" htmlType="submit" icon={<SettingOutlined />} loading={loading}>
                保存配置
              </Button>
            </Space>
          </Form>
        </MaintenanceSection>
      )
    },
    {
      key: 'ip-data',
      label: 'IP 库',
      icon: <DatabaseOutlined />,
      children: (
        <MaintenanceSection title="IP 库" description="支持自定义 IP 映射和 GeoIP 路径管理，重载失败时保留旧库。">
          <Form
            form={ipDataForm}
            layout="vertical"
            className="dense-form"
            onFinish={(values) => void saveSettings(values)}
          >
            <Form.Item name="custom_ip_map_path" label="自定义 IP 映射 CSV 路径">
              <Input placeholder="/opt/nat-query/custom_ip_map.csv" />
            </Form.Item>
            <Form.Item name="geoip_db_path" label="GeoIP mmdb 路径">
              <Input placeholder="/data/index/GeoLite2-City.mmdb" />
            </Form.Item>
            <div className="maintenance-grid">
              <Form.Item name="ip_map_enabled" label="启用自定义 IP 映射" valuePropName="checked">
                <Switch />
              </Form.Item>
              <Form.Item name="geoip_enabled" label="启用 GeoIP" valuePropName="checked">
                <Switch />
              </Form.Item>
            </div>
            <Space>
              <Button type="primary" htmlType="submit" icon={<SettingOutlined />} loading={loading}>
                保存配置
              </Button>
              <Button icon={<ReloadOutlined />} onClick={() => void reloadIPData()} loading={reloadingIP}>
                重新加载 IP 库
              </Button>
            </Space>
          </Form>
        </MaintenanceSection>
      )
    },
    {
      key: 'auto-sync',
      label: '自动增量',
      icon: <SyncOutlined />,
      children: (
        <MaintenanceSection title="自动增量" description="自动增量开关和调度模式在这里维护。">
          <Form
            form={autoSyncForm}
            layout="vertical"
            className="dense-form"
            onFinish={(values) => void saveSettings(values)}
          >
            <div className="maintenance-grid">
              <Form.Item name="auto_scan_enabled" label="开启自动增量" valuePropName="checked">
                <Switch />
              </Form.Item>
              <Form.Item name="auto_scan_mode" label="调度模式">
                <Select
                  options={[
                    { label: 'hourly', value: 'hourly' },
                    { label: 'daily', value: 'daily' },
                    { label: 'custom', value: 'custom' }
                  ]}
                />
              </Form.Item>
              <Form.Item name="auto_scan_times" label="执行时间">
                <Input placeholder="01:00" />
              </Form.Item>
              <Form.Item name="auto_scan_timezone" label="时区">
                <Input placeholder="Asia/Shanghai" />
              </Form.Item>
              <Form.Item name="auto_scan_jitter_sec" label="抖动秒数">
                <Input placeholder="60" />
              </Form.Item>
            </div>
            <Button type="primary" htmlType="submit" icon={<SettingOutlined />} loading={loading}>
              保存配置
            </Button>
          </Form>
        </MaintenanceSection>
      )
    },
    {
      key: 'ops',
      label: '维护操作',
      icon: <ReloadOutlined />,
      children: (
        <MaintenanceSection title="维护操作" description="低频维护动作集中在这里，避免进入日常主流程。">
          <Space wrap>
            <Button
              icon={<SyncOutlined />}
              onClick={() => void apiPost('/api/sync')
                .then(() => message.success('已触发立即增量'))
                .catch((error: unknown) => message.error(error instanceof Error ? error.message : '操作失败'))}
            >
              立即增量一次
            </Button>
            <Button
              onClick={() => void apiPost('/api/rebuild', { mode: 'retry_failed' })
                .then(() => message.success('已触发失败日期重试'))
                .catch((error: unknown) => message.error(error instanceof Error ? error.message : '操作失败'))}
            >
              重试失败日期
            </Button>
            <Button
              danger
              onClick={() => void apiPost('/api/rebuild', { mode: 'full' })
                .then(() => message.success('已触发全量重建'))
                .catch((error: unknown) => message.error(error instanceof Error ? error.message : '操作失败'))}
            >
              全量重建
            </Button>
          </Space>
        </MaintenanceSection>
      )
    },
    {
      key: 'security',
      label: '登录安全',
      icon: <SafetyOutlined />,
      children: (
        <MaintenanceSection title="登录安全" description="仅保留单一管理员密码。">
          <Form form={passwordForm} layout="vertical" className="dense-form">
            <Form.Item name="current_password" label="当前管理员密码">
              <Input.Password />
            </Form.Item>
            <Form.Item
              name="new_password"
              label="新管理员密码"
              rules={[{ required: true, message: '请输入新管理员密码' }]}
            >
              <Input.Password />
            </Form.Item>
            <Button type="primary" icon={<SafetyOutlined />} loading={passwordSubmitting} onClick={() => void updatePassword()}>
              更新密码
            </Button>
          </Form>
        </MaintenanceSection>
      )
    }
  ];

  return <Tabs className="maintenance-tabs" items={items} />;
}
