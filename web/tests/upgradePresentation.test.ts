import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import test from 'node:test';
import { buildUpgradeView } from '../src/upgradePresentation.ts';

test('shows an upgrade action when a newer release has all assets', () => {
  const view = buildUpgradeView({
    status: { state: 'idle', current_version: 'v1.0.10' },
    check: {
      current_version: 'v1.0.10',
      latest_version: 'v1.0.12',
      update_available: true,
      assets_ready: true,
    },
    isChecking: false,
    lastCheckedAt: new Date('2026-07-08T01:00:00+08:00'),
    autoCheckEnabled: true,
  });

  assert.equal(view.state, 'available');
  assert.equal(view.showUpgradeAction, true);
  assert.equal(view.latestVersion, 'v1.0.12');
  assert.equal(view.statusTone, 'warning');
});

test('prefers a fresh available release over a previous failed upgrade status', () => {
  const view = buildUpgradeView({
    status: {
      state: 'failed',
      current_version: 'v1.0.10',
      target_version: 'v1.0.11',
      error: 'dpkg failed',
    },
    check: {
      current_version: 'v1.0.10',
      latest_version: 'v1.0.12',
      update_available: true,
      assets_ready: true,
    },
    isChecking: false,
    lastCheckedAt: new Date('2026-07-08T01:00:00+08:00'),
    autoCheckEnabled: true,
  });

  assert.equal(view.state, 'available');
  assert.equal(view.showUpgradeAction, true);
  assert.equal(view.latestVersion, 'v1.0.12');
  assert.equal(view.statusTone, 'warning');
});

test('maintenance page uses the resolved upgrade view tone for the status tag', () => {
  const page = fs.readFileSync(path.resolve('src/pages/SystemMaintenancePage.tsx'), 'utf8');

  assert.match(page, /<Tag color=\{upgradeView\.statusTone\}>/);
});
