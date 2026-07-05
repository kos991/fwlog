import assert from 'node:assert/strict';
import test from 'node:test';
import { buildUpgradeView } from '../src/upgradePresentation.ts';

test('shows unchecked state before a release check runs', () => {
  const view = buildUpgradeView({
    status: { state: 'idle', current_version: 'dev-20260705' },
    check: null,
    isChecking: false,
    lastCheckedAt: null,
    autoCheckEnabled: false,
  });

  assert.equal(view.state, 'unchecked');
  assert.equal(view.currentVersion, 'dev-20260705');
  assert.equal(view.primaryText, '检查更新');
  assert.equal(view.showUpgradeAction, false);
  assert.equal(view.message, '升级只会在确认后手动执行；自动检查不会自动安装。');
});

test('shows an upgrade action only when a newer release has all assets', () => {
  const view = buildUpgradeView({
    status: { state: 'idle', current_version: 'dev-20260705' },
    check: {
      current_version: 'dev-20260705',
      latest_version: 'v1.1.1',
      update_available: true,
      assets_ready: true,
    },
    isChecking: false,
    lastCheckedAt: new Date('2026-07-05T12:00:00+08:00'),
    autoCheckEnabled: true,
  });

  assert.equal(view.state, 'available');
  assert.equal(view.latestVersion, 'v1.1.1');
  assert.equal(view.showUpgradeAction, true);
  assert.equal(view.upgradeButtonText, '升级到 v1.1.1');
  assert.equal(view.lastCheckedText, '上次检查：2026-07-05 12:00:00');
});

test('hides upgrade action when current release is latest', () => {
  const view = buildUpgradeView({
    status: { state: 'idle', current_version: 'v1.1.1' },
    check: {
      current_version: 'v1.1.1',
      latest_version: 'v1.1.1',
      update_available: false,
      assets_ready: true,
    },
    isChecking: false,
    lastCheckedAt: new Date('2026-07-05T12:00:00+08:00'),
    autoCheckEnabled: false,
  });

  assert.equal(view.state, 'latest');
  assert.equal(view.showUpgradeAction, false);
  assert.equal(view.message, '当前已是最新版本。');
});

test('blocks upgrade when release assets are missing', () => {
  const view = buildUpgradeView({
    status: { state: 'idle', current_version: 'v1.1.0' },
    check: {
      current_version: 'v1.1.0',
      latest_version: 'v1.1.1',
      update_available: true,
      assets_ready: false,
      missing_assets: ['nat-query-service_linux_amd64'],
    },
    isChecking: false,
    lastCheckedAt: null,
    autoCheckEnabled: false,
  });

  assert.equal(view.state, 'asset_missing');
  assert.equal(view.showUpgradeAction, false);
  assert.equal(view.message, '缺少资产：nat-query-service_linux_amd64');
});

test('prioritizes running and failed backend status messages', () => {
  const running = buildUpgradeView({
    status: { state: 'running', current_version: 'v1.1.0', target_version: 'v1.1.1' },
    check: null,
    isChecking: false,
    lastCheckedAt: null,
    autoCheckEnabled: false,
  });

  assert.equal(running.state, 'running');
  assert.equal(running.latestVersion, 'v1.1.1');
  assert.equal(running.showUpgradeAction, false);
  assert.equal(running.message, '正在升级到 v1.1.1，服务会自动重启。');

  const failed = buildUpgradeView({
    status: { state: 'failed', current_version: 'v1.1.0', error: 'download failed' },
    check: null,
    isChecking: false,
    lastCheckedAt: null,
    autoCheckEnabled: false,
  });

  assert.equal(failed.state, 'failed');
  assert.equal(failed.message, 'download failed');
});

test('shows succeeded status without offering another upgrade action', () => {
  const view = buildUpgradeView({
    status: {
      state: 'succeeded',
      current_version: 'v1.1.1',
      target_version: 'v1.1.1',
      backup_path: '/opt/nat-query/nat-query-service.bak.20260705114302',
    },
    check: null,
    isChecking: false,
    lastCheckedAt: null,
    autoCheckEnabled: false,
  });

  assert.equal(view.state, 'succeeded');
  assert.equal(view.showUpgradeAction, false);
  assert.equal(view.message, '升级完成，备份：/opt/nat-query/nat-query-service.bak.20260705114302');
});
