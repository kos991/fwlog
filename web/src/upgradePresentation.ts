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
    currentVersion,
    latestVersion,
    primaryText: input.isChecking ? '检查中' : '检查更新',
    showUpgradeAction: state === 'available',
    upgradeButtonText,
    message: buildMessage(input, state, latestVersion),
    lastCheckedText: input.lastCheckedAt ? `上次检查：${formatLocalDateTime(input.lastCheckedAt)}` : '上次检查：尚未检查',
    sourceText: input.autoCheckEnabled
      ? '已开启自动检查；从 GitHub Releases 检查更新'
      : '从 GitHub Releases 检查更新',
    statusTone: resolveStatusTone(state),
  };
}

function resolvePanelState(input: BuildUpgradeViewInput): UpgradePanelState {
  if (input.isChecking) return 'checking';
  if (input.status?.state === 'running') return 'running';
  if (input.status?.state === 'failed') return 'failed';
  if (input.status?.state === 'succeeded') return 'succeeded';
  if (!input.check) return 'unchecked';
  if (input.check.update_available && !input.check.assets_ready) return 'asset_missing';
  if (input.check.update_available && input.check.assets_ready) return 'available';
  return 'latest';
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
    return missing.length ? `缺少资产：${missing.join(', ')}` : input.check?.message || 'Release 资产不齐，暂不能升级。';
  }
  if (state === 'available') {
    return latestVersion ? `发现可升级版本 ${latestVersion}。` : '发现可升级版本。';
  }
  if (state === 'latest') return '当前已是最新版本。';
  if (state === 'checking') return '正在从 GitHub Releases 检查更新。';
  return '升级只会在确认后手动执行；自动检查不会自动安装。';
}

function resolveStatusTone(state: UpgradePanelState): UpgradeView['statusTone'] {
  if (state === 'running' || state === 'checking') return 'processing';
  if (state === 'succeeded' || state === 'latest') return 'success';
  if (state === 'available' || state === 'asset_missing') return 'warning';
  if (state === 'failed') return 'error';
  return 'default';
}

function formatLocalDateTime(value: Date) {
  const pad = (item: number) => String(item).padStart(2, '0');
  return [
    value.getFullYear(),
    pad(value.getMonth() + 1),
    pad(value.getDate()),
  ].join('-') + ` ${pad(value.getHours())}:${pad(value.getMinutes())}:${pad(value.getSeconds())}`;
}
