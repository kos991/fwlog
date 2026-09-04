import React from 'react';
import { Radio, Switch, Tag, Typography } from 'antd';

const { Text } = Typography;

type UpgradeChannel = 'stable' | 'beta';

const versionItems = [
  { label: 'FWLog 系统版本', value: 'v2.1.0', strong: true, position: 'system' },
  { label: '发布时间', value: '2026-07-17 09:40' },
  { label: '数据引擎版本', value: '25.8.27.1' },
  { label: 'IP 地址库更新时间', value: '2026-07-15' },
  { label: '运行架构', value: 'Linux · amd64' },
];

const autoUpgradeItems = [
  {
    key: 'application',
    title: '主程序自动升级',
    description: '仅安装经过完整性校验的应用升级包',
  },
  {
    key: 'ipDatabase',
    title: 'IP 地址库自动更新',
    description: '更新地址归属和地理位置数据文件',
  },
];

export function UpgradePreviewPage() {
  const [channel, setChannel] = React.useState<UpgradeChannel>('stable');
  const [autoUpgrade, setAutoUpgrade] = React.useState<Record<string, boolean>>({
    application: true,
    ipDatabase: true,
  });
  return (
    <div className="upgrade-preview-page">
      <section className="upgrade-version-band" aria-label="版本信息">
        {versionItems.map((item, index) => (
          <div className={`upgrade-version-item upgrade-version-item--${item.position || `slot-${index + 1}`}`} key={item.label}>
            <span>{item.label}</span>
            <strong className={item.strong ? 'upgrade-version-primary' : undefined}>{item.value}</strong>
          </div>
        ))}
      </section>

      <div className="upgrade-policy-grid">
        <section className="upgrade-policy-panel">
          <div className="upgrade-panel-heading">
            <h2>自动升级</h2>
          </div>

          <div className="upgrade-policy-list">
            {autoUpgradeItems.map((item) => (
              <div className="upgrade-policy-row" key={item.key}>
                <div className="upgrade-policy-copy">
                  <strong>{item.title}</strong>
                  <Text type="secondary">{item.description}</Text>
                </div>
                <Switch
                  checked={autoUpgrade[item.key]}
                  aria-label={`${item.title}自动升级`}
                  onChange={(checked) => setAutoUpgrade((current) => ({ ...current, [item.key]: checked }))}
                />
              </div>
            ))}

            <div className="upgrade-policy-row">
              <div className="upgrade-policy-copy">
                <strong>数据引擎更新</strong>
                <Text type="secondary">仅随完整安装包升级，不独立更新</Text>
              </div>
              <Tag>随完整包更新</Tag>
            </div>
          </div>
        </section>

        <section className="upgrade-policy-panel upgrade-channel-panel">
          <div className="upgrade-panel-heading">
            <h2>版本通道</h2>
          </div>

          <Radio.Group
            className="upgrade-channel-list"
            value={channel}
            onChange={(event) => setChannel(event.target.value as UpgradeChannel)}
          >
            <div className={`upgrade-channel-option ${channel === 'stable' ? 'is-selected' : ''}`} onClick={() => setChannel('stable')}>
              <Radio value="stable" />
              <span>
                <strong>正式版 <Tag color="success">推荐</Tag></strong>
                <Text type="secondary">版本稳定，适合生产环境</Text>
              </span>
            </div>
            <div className={`upgrade-channel-option ${channel === 'beta' ? 'is-selected' : ''}`} onClick={() => setChannel('beta')}>
              <Radio value="beta" />
              <span>
                <strong>体验版 <Tag color="blue">Beta</Tag></strong>
                <Text type="secondary">提前获取新功能和候选版本</Text>
              </span>
            </div>
          </Radio.Group>

          <Text className="upgrade-channel-note" type="secondary">
            切换通道不会立即安装版本，仅影响后续更新检查。
          </Text>

        </section>
      </div>
    </div>
  );
}
