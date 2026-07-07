import type { UpgradeCheckResponse, UpgradeStatus } from './api';

export type UpgradePanelState =
  | 'unchecked'
  | 'checking'
  | 'available'
  | 'latest'
  | 'asset_missing'
  | 'running'
  | 'failed'
  | 'succeeded';

type BuildUpgradeViewInput = {
  status: UpgradeStatus | null;
  check: UpgradeCheckResponse | null;
  isChecking: boolean;
  lastCheckedAt: Date | null;
  autoCheckEnabled: boolean;
};

export type UpgradeView = {
  state: UpgradePanelState;
  stateText: string;
  currentVersion: string;
  latestVersion: string;
  primaryText: string;
  showUpgradeAction: boolean;
  upgradeButtonText: string;
  message: string;
  lastCheckedText: string;
  sourceText: string;
  statusTone: 'default' | 'processing' | 'success' | 'warning' | 'error';
};

export function buildUpgradeView(input: BuildUpgradeViewInput): UpgradeView {
  const state = resolvePanelState(input);
  const currentVersion = input.check?.current_version || input.status?.current_version || '-';
  const latestVersion = resolveLatestVersion(input);
  const upgradeButtonText = latestVersion ? `升级到 ${latestVersion}` : '升级';

  return {
    state,
    stateText: resolveStateText(state),
    currentVersion,
    latestVersion,
    primaryText: input.isChecking ? '检查中' : '检查更新',
    showUpgradeAction: state === 'available',
    upgradeButtonText,
    message: buildMessage(input, state, latestVersion),
    lastCheckedText: input.lastCheckedAt ? `上次检查：${formatLocalDateTime(input.lastCheckedAt)}` : '上次检查：尚未检查',
    sourceText: input.autoCheckEnabled
      ? '已开启自动检查；从发布版本检查更新'
      : '从发布版本检查更新',
    statusTone: resolveStatusTone(state),
  };
}

function resolvePanelState(input: BuildUpgradeViewInput): UpgradePanelState {
  if (input.isChecking) return 'checking';
  if (input.status?.state === 'running') return 'running';
  if (input.check) {
    if (input.check.update_available && !input.check.assets_ready) return 'asset_missing';
    if (input.check.update_available && input.check.assets_ready) return 'available';
    return 'latest';
  }
  if (input.status?.state === 'failed') return 'failed';
  if (input.status?.state === 'succeeded') return 'succeeded';
  return 'unchecked';
}

function resolveLatestVersion(input: BuildUpgradeViewInput) {
  if (input.status?.state === 'running' && input.status.target_version) return input.status.target_version;
  if (input.status?.state === 'succeeded' && input.status.target_version) return input.status.target_version;
  return input.check?.latest_version || '';
}

function buildMessage(input: BuildUpgradeViewInput, state: UpgradePanelState, latestVersion: string) {
  if (state === 'running') {
    return latestVersion ? `正在升级到 ${latestVersion}，服务会自动重启。` : '正在升级，服务会自动重启。';
  }
  if (state === 'failed') {
    return input.status?.error || input.status?.message || '升级失败，请查看服务日志。';
  }
  if (state === 'succeeded') {
    return input.status?.backup_path
      ? `升级完成，备份：${input.status.backup_path}`
      : input.status?.message || '升级完成，服务已重启。';
  }
  if (state === 'asset_missing') {
    const missing = input.check?.missing_assets || [];
    return missing.length ? `缺少发布资产：${missing.join(', ')}` : input.check?.message || '发布资产不完整，暂不能升级。';
  }
  if (state === 'available') {
    return latestVersion ? `发现新版本：${latestVersion}，可手动升级。` : '发现新版本，可手动升级。';
  }
  if (state === 'latest') return '当前已是最新版本。';
  if (state === 'checking') return '正在检查发布版本。';
  return '点击检查更新后，如发现新版本会显示升级按钮。';
}

function resolveStatusTone(state: UpgradePanelState): UpgradeView['statusTone'] {
  if (state === 'running' || state === 'checking') return 'processing';
  if (state === 'succeeded' || state === 'latest') return 'success';
  if (state === 'available' || state === 'asset_missing') return 'warning';
  if (state === 'failed') return 'error';
  return 'default';
}

function resolveStateText(state: UpgradePanelState) {
  switch (state) {
    case 'checking':
      return '检查中';
    case 'available':
      return '可升级';
    case 'latest':
      return '已是最新';
    case 'asset_missing':
      return '资产缺失';
    case 'running':
      return '升级中';
    case 'failed':
      return '升级失败';
    case 'succeeded':
      return '升级完成';
    default:
      return '未检查';
  }
}

function formatLocalDateTime(value: Date) {
  const pad = (item: number) => String(item).padStart(2, '0');
  return [
    value.getFullYear(),
    pad(value.getMonth() + 1),
    pad(value.getDate()),
  ].join('-') + ` ${pad(value.getHours())}:${pad(value.getMinutes())}:${pad(value.getSeconds())}`;
}
