import React from 'react';
import {
  ApiOutlined,
  DeleteOutlined,
  LockOutlined,
  SaveOutlined,
  SafetyCertificateOutlined,
} from '@ant-design/icons';
import { Button, Input, Popconfirm, Space, Switch, Tag, Tooltip, Typography, message } from 'antd';
import { apiGet, apiPost } from '../api';
import {
  THREAT_PROVIDER_META,
  type ThreatProvider,
  type ThreatProviderListResponse,
  type ThreatProviderStatus,
} from '../threatIntelligence';

const { Text } = Typography;

const PROVIDERS: ThreatProvider[] = ['threatbook', 'nsfocus', 'qianxin', 'tencent'];
const PROVIDER_NAMES: Record<ThreatProvider, string> = {
  threatbook: '微步',
  nsfocus: '绿盟',
  qianxin: '奇安信',
  tencent: '腾讯',
};

type ProviderUpdate = {
  enabled: boolean;
  credential: string | null;
  clear_credential: boolean;
};

function statusTag(status?: ThreatProviderStatus) {
  if (status?.credential_error) return <Tag color="error">凭据异常</Tag>;
  if (status?.last_test_status === 'success') return <Tag color="success">连接正常</Tag>;
  if (status?.last_test_status === 'failed') return <Tag color="error">连接失败</Tag>;
  if (status?.configured) return <Tag color="processing">已配置</Tag>;
  return <Tag>未配置</Tag>;
}

export function ThreatIntelligenceSettingsPanel() {
  const [statuses, setStatuses] = React.useState<ThreatProviderStatus[]>([]);
  const [credentials, setCredentials] = React.useState<Record<ThreatProvider, string>>({
    threatbook: '',
    nsfocus: '',
    qianxin: '',
    tencent: '',
  });
  const [enabled, setEnabled] = React.useState<Record<ThreatProvider, boolean>>({
    threatbook: false,
    nsfocus: false,
    qianxin: false,
    tencent: false,
  });
  const [loading, setLoading] = React.useState(true);
  const [saving, setSaving] = React.useState<ThreatProvider | null>(null);
  const [testing, setTesting] = React.useState<ThreatProvider | null>(null);

  const load = React.useCallback(async () => {
    try {
      setLoading(true);
      const response = await apiGet<ThreatProviderListResponse>('/api/threat-intelligence/providers');
      const nextStatuses = response.providers || [];
      setStatuses(nextStatuses);
      setEnabled((current) => {
        const next = { ...current };
        nextStatuses.forEach((item) => {
          next[item.provider] = item.enabled;
        });
        return next;
      });
    } catch (error) {
      setStatuses([]);
      message.error(error instanceof Error ? error.message : '加载威胁情报平台失败');
    } finally {
      setLoading(false);
    }
  }, []);

  React.useEffect(() => {
    void load();
  }, [load]);

  const saveProvider = async (provider: ThreatProvider) => {
    const credential = credentials[provider].trim();
    const body: ProviderUpdate = {
      enabled: enabled[provider],
      credential: credential ? credential : null,
      clear_credential: false,
    };
    try {
      setSaving(provider);
      const updated = await apiPost<ThreatProviderStatus>(`/api/threat-intelligence/providers/${provider}`, body);
      setStatuses((current) => current.map((item) => item.provider === provider ? updated : item));
      setCredentials((current) => ({ ...current, [provider]: '' }));
      message.success(`${THREAT_PROVIDER_META[provider].name} 配置已保存`);
    } catch (error) {
      message.error(error instanceof Error ? error.message : '保存平台配置失败');
    } finally {
      setSaving(null);
    }
  };

  const clearCredential = async (provider: ThreatProvider) => {
    try {
      setSaving(provider);
      const updated = await apiPost<ThreatProviderStatus>(`/api/threat-intelligence/providers/${provider}`, {
        enabled: enabled[provider],
        credential: null,
        clear_credential: true,
      });
      setStatuses((current) => current.map((item) => item.provider === provider ? updated : item));
      setCredentials((current) => ({ ...current, [provider]: '' }));
      message.success(`${THREAT_PROVIDER_META[provider].name} 凭据已清除`);
    } catch (error) {
      message.error(error instanceof Error ? error.message : '清除凭据失败');
    } finally {
      setSaving(null);
    }
  };

  const testProvider = async (provider: ThreatProvider) => {
    try {
      setTesting(provider);
      await apiPost(`/api/threat-intelligence/providers/${provider}/test`);
      await load();
      message.success(`${THREAT_PROVIDER_META[provider].name} 连接测试完成`);
    } catch (error) {
      message.error(error instanceof Error ? error.message : '连接测试失败');
    } finally {
      setTesting(null);
    }
  };

  return (
    <section className="threat-intelligence-settings">
      <div className="maintenance-panel-toolbar">
        <Text className="maintenance-panel-note" type="secondary">
          凭据仅提交至 FWLOG 后端保存，输入框不会回显已保存内容
        </Text>
        <Button icon={<SafetyCertificateOutlined />} onClick={() => void load()} loading={loading}>
          刷新状态
        </Button>
      </div>
      <div className="threat-provider-list" aria-busy={loading}>
        {PROVIDERS.map((provider) => {
          const meta = THREAT_PROVIDER_META[provider];
          const providerName = PROVIDER_NAMES[provider] || meta.name;
          const status = statuses.find((item) => item.provider === provider);
          const isBusy = saving === provider || testing === provider;
          return (
            <div className="threat-provider-row" key={provider}>
              <div className="threat-provider-identity">
                <strong>{providerName}</strong>
                <Text type="secondary">{provider}</Text>
                {provider === 'nsfocus' && <Text type="warning">连接测试需要公网 IP 授权</Text>}
              </div>
              <div className="threat-provider-enabled">
                <Switch
                  checked={enabled[provider]}
                  onChange={(checked) => setEnabled((current) => ({ ...current, [provider]: checked }))}
                  disabled={isBusy}
                  aria-label={`${providerName} 启用`}
                />
                <Text>{enabled[provider] ? '已启用' : '已停用'}</Text>
              </div>
              <div className="threat-provider-credential">
                <Input.Password
                  prefix={<LockOutlined />}
                  value={credentials[provider]}
                  onChange={(event) => setCredentials((current) => ({ ...current, [provider]: event.target.value }))}
                  placeholder="输入新凭据（留空保持原凭据）"
                  autoComplete="new-password"
                  aria-label={`${providerName} 凭据`}
                />
              </div>
              <div className="threat-provider-status">
                {statusTag(status)}
                {status?.last_test_message && <Text type="secondary">{status.last_test_message}</Text>}
              </div>
              <Space className="threat-provider-actions" wrap>
                <Button
                  type="primary"
                  icon={<SaveOutlined />}
                  loading={saving === provider}
                  disabled={testing !== null}
                  onClick={() => void saveProvider(provider)}
                >
                  保存
                </Button>
                <Popconfirm
                  title="确认测试连接？"
                  description="连接测试可能消耗 1 次接口额度"
                  okText="开始测试"
                  cancelText="取消"
                  onConfirm={() => void testProvider(provider)}
                >
                  <Button icon={<ApiOutlined />} loading={testing === provider} disabled={saving !== null}>
                    连接测试
                  </Button>
                </Popconfirm>
                <Tooltip title="清除凭据">
                  <Button
                    danger
                    type="text"
                    icon={<DeleteOutlined />}
                    aria-label={`清除${providerName}凭据`}
                    disabled={!status?.configured || isBusy}
                    onClick={() => void clearCredential(provider)}
                  />
                </Tooltip>
              </Space>
            </div>
          );
        })}
      </div>
    </section>
  );
}

export default ThreatIntelligenceSettingsPanel;
