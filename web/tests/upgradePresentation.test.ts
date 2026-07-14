import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import test from 'node:test';
import { buildUpgradeView, isSupportedUpgradeVersion } from '../src/upgradePresentation.ts';

test('accepts stable and test release versions for online upgrade', () => {
  assert.equal(isSupportedUpgradeVersion('v2.0.0'), true);
  assert.equal(isSupportedUpgradeVersion('v2.0.0.3'), true);
  assert.equal(isSupportedUpgradeVersion('v2.0.0.3;reboot'), false);
});

test('shows an upgrade action when a newer release has all assets', () => {
  const view = buildUpgradeView({
    status: { state: 'idle', current_version: 'v2.0.0' },
    check: {
      current_version: 'v2.0.0',
      latest_version: 'v2.0.2',
      update_available: true,
      assets_ready: true,
    },
    isChecking: false,
    lastCheckedAt: new Date('2026-07-08T01:00:00+08:00'),
  });

  assert.equal(view.state, 'available');
  assert.equal(view.showUpgradeAction, true);
  assert.equal(view.latestVersion, 'v2.0.2');
  assert.equal(view.statusTone, 'warning');
});

test('prefers a fresh available release over a previous failed upgrade status', () => {
  const view = buildUpgradeView({
    status: {
      state: 'failed',
      current_version: 'v2.0.0',
      target_version: 'v2.0.1',
      error: 'dpkg failed',
    },
    check: {
      current_version: 'v2.0.0',
      latest_version: 'v2.0.2',
      update_available: true,
      assets_ready: true,
    },
    isChecking: false,
    lastCheckedAt: new Date('2026-07-08T01:00:00+08:00'),
  });

  assert.equal(view.state, 'available');
  assert.equal(view.showUpgradeAction, true);
  assert.equal(view.latestVersion, 'v2.0.2');
  assert.equal(view.statusTone, 'warning');
});

test('requires the full bundle when the installed runtime is incompatible', () => {
  const view = buildUpgradeView({
    status: { state: 'idle', current_version: 'v2.0.0' },
    check: {
      current_version: 'v2.0.0',
      latest_version: 'v2.0.0',
      update_available: true,
      assets_ready: false,
      runtime_version: 'clickhouse-24.1',
      required_runtime_version: 'clickhouse-25.8',
      runtime_compatible: false,
    },
    isChecking: false,
    lastCheckedAt: null,
  });

  assert.equal(view.state, 'runtime_incompatible');
  assert.equal(view.showUpgradeAction, false);
  assert.match(view.message, /当前运行组件版本/);
  assert.match(view.message, /完整本地升级包/);
  assert.doesNotMatch(view.message, /runtime|full|离线包/i);
});

test('maintenance page uses the resolved upgrade view tone for the status tag', () => {
  const page = fs.readFileSync(path.resolve('src/pages/SystemMaintenancePage.tsx'), 'utf8');

  assert.match(page, /<Tag color=\{upgradeView\.statusTone\}>/);
});

test('upgrade panel copy no longer mentions automatic checking', () => {
  const view = buildUpgradeView({
    status: { state: 'idle', current_version: 'v2.0.3' },
    check: null,
    isChecking: false,
    lastCheckedAt: null,
  });

  assert.equal(view.sourceText, '从发布版本检查更新');
});

test('maintenance page removes auto check setting and startup auto check effect', () => {
  const page = fs.readFileSync(path.resolve('src/pages/SystemMaintenancePage.tsx'), 'utf8');

  assert.doesNotMatch(page, /自动检查更新/);
  assert.doesNotMatch(page, /upgrade_auto_check_enabled/);
  assert.doesNotMatch(page, /autoUpgradeCheckStartedRef/);
});
