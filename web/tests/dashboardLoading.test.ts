import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import test from 'node:test';

const source = fs.readFileSync(path.resolve('src/pages/HealthDashboard.tsx'), 'utf8');

test('仪表盘按顺序加载概览和排行并取消旧请求', () => {
  assert.match(source, /health-dashboard\/summary/);
  assert.match(source, /health-dashboard\/rankings/);
  assert.match(source, /AbortController/);
  assert.match(source, /await loadSummary\(\)/);
  assert.match(source, /await loadRankings\(\)/);
  assert.doesNotMatch(source, /include_distributions/);
});

