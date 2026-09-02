import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import test from 'node:test';

const componentPath = path.resolve('src/components/ThreatIntelligenceSettingsPanel.tsx');
const pagePath = path.resolve('src/pages/SystemMaintenancePage.tsx');

test('maintenance page exposes an independent threat intelligence settings panel', () => {
  const page = fs.readFileSync(pagePath, 'utf8');
  const panel = fs.readFileSync(componentPath, 'utf8');

  assert.match(page, /key: 'threat-intelligence'/);
  assert.match(page, /威胁情报/);
  for (const name of ['微步', '绿盟', '奇安信', '腾讯']) {
    assert.match(panel, new RegExp(name));
  }
  assert.match(panel, /连接测试可能消耗 1 次接口额度/);
  assert.match(panel, /clear_credential: true/);
  assert.doesNotMatch(panel, /value=\{provider\.credential\}/);
  assert.match(panel, /apiGet<ThreatProviderListResponse>\('\/api\/threat-intelligence\/providers'\)/);
  assert.match(panel, /apiPost<ThreatProviderStatus>\(`\/api\/threat-intelligence\/providers\/\$\{provider\}`/);
});

test('maintenance layout includes the threat intelligence tab and panel note', () => {
  const page = fs.readFileSync(pagePath, 'utf8');
  assert.equal((page.match(/<section className="ops-section maintenance-card maintenance-panel">/g) || []).length, 6);
  assert.equal((page.match(/className="maintenance-panel-note"/g) || []).length, 6);
  assert.match(page, /威胁情报平台配置/);
  assert.match(page, /ThreatIntelligenceSettingsPanel/);
});
