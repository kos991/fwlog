import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import test from 'node:test';

test('访问趋势只展示目标地域节点，不展示线路', () => {
  const page = fs.readFileSync(path.resolve('src/pages/AccessTrendPreview.tsx'), 'utf8');
  const styles = fs.readFileSync(path.resolve('src/styles.css'), 'utf8');

  assert.match(page, /EffectScatterChart/);
  assert.match(page, /type Point =/);
  assert.match(page, /points: Point\[\]/);
  assert.match(page, /data\?\.points/);
  assert.match(page, /\/api\/access-trend/);
  assert.match(page, /useState\('7d'\)/);
  assert.doesNotMatch(page, /LinesChart/);
  assert.doesNotMatch(page, /type Flow =/);
  assert.doesNotMatch(page, /flows: Flow\[\]/);
  assert.doesNotMatch(page, /chart\.on\('click'/);
  assert.doesNotMatch(page, /NodeIndexOutlined/);
  assert.doesNotMatch(page, /formatIPPort/);
  assert.doesNotMatch(page, /access-route/);
  assert.doesNotMatch(styles, /\.access-route/);
});

test('数据概览保留模式、时间和统计，并使用紧凑地图布局', () => {
  const dashboard = fs.readFileSync(path.resolve('src/pages/HealthDashboard.tsx'), 'utf8');
  const page = fs.readFileSync(path.resolve('src/pages/AccessTrendPreview.tsx'), 'utf8');
  const styles = fs.readFileSync(path.resolve('src/styles.css'), 'utf8');

  assert.match(dashboard, /<AccessTrendPreview[\s\S]*?embedded[\s\S]*?sourceID=/);
  assert.match(page, /全部访问/);
  assert.match(page, /国内访问/);
  assert.match(page, /国外访问/);
  assert.match(page, /访问总量/);
  assert.match(page, /目标地域节点/);
  assert.match(page, /className="access-map-topbar"[\s\S]*className="access-stat-grid"[\s\S]*\{embedded && toolbar\}/);
  assert.doesNotMatch(page, /<aside/);
  assert.match(page, /value: '3d', label: '最近 3 天'/);
  assert.doesNotMatch(page, /value: '30d'/);
  assert.doesNotMatch(page, /当前线路/);
  assert.doesNotMatch(page, />线路</);
  assert.match(styles, /\.access-preview-grid\s*\{[^}]*height:\s*460px;/s);
  assert.match(styles, /\.access-map\s*\{[^}]*min-height:\s*0;/s);
  assert.doesNotMatch(styles, /\.access-detail-panel/);
});
