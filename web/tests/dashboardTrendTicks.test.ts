import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import test from 'node:test';

const source = fs.readFileSync(path.resolve('src/pages/HealthDashboard.tsx'), 'utf8');

test('dashboard trend chart uses unique integer count ticks', () => {
  assert.match(source, /function buildCountTicks\(maxValue: number\): number\[\]/);
  assert.match(source, /function formatTrendAxisValue\(value: number\)/);
  assert.match(source, /const padding = \{ top: 22, right: 24, bottom: 34, left: 72 \};/);
  assert.match(source, /formatTrendAxisValue\(tick\)/);
  assert.match(source, /if \(!Number\.isFinite\(maxValue\) \|\| maxValue <= 0\)/);
  assert.match(source, /return \[0\];/);
  assert.doesNotMatch(source, /Math\.round\(max \* ratio\)/);
  assert.doesNotMatch(source, /const gridValues = \[0, 0\.25, 0\.5, 0\.75, 1\]/);
});

test('dashboard trend consumes daily source scoped points', () => {
  assert.match(source, /type LogTrendPoint = \{/);
  assert.match(source, /date: string;/);
  assert.match(source, /source_id: string;/);
  assert.match(source, /log_tag: string;/);
  assert.match(source, /log_trend\?: LogTrendPoint\[\];/);
  assert.doesNotMatch(source, /log_trend\?: DistributionItem\[\];/);
});

test('dashboard trend renders date labels instead of fixed hour labels', () => {
  assert.match(source, /buildTrendSeries/);
  assert.match(source, /formatTrendDateLabel/);
  assert.doesNotMatch(source, /const xLabels = \['00:00', '04:00', '08:00', '12:00', '16:00', '20:00'\]/);
});

test('dashboard trend exposes all devices and per-device filter options', () => {
  assert.match(source, /全部设备/);
  assert.match(source, /selectedTrendSource/);
  assert.match(source, /trendSourceOptions/);
  assert.match(source, /source_id: selectedTrendSource === allTrendSourcesValue \? undefined : selectedTrendSource/);
});
