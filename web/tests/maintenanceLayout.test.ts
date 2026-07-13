import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import test from 'node:test';

const pagePath = path.resolve('src/pages/SystemMaintenancePage.tsx');
const stylesPath = path.resolve('src/styles.css');

test('maintenance page stacks schedule, manual actions, and upgrade cards vertically', () => {
  const page = fs.readFileSync(pagePath, 'utf8');
  const styles = fs.readFileSync(stylesPath, 'utf8');

  assert.match(page, /maintenance-plan-card maintenance-plan-card--schedule/);
  assert.match(page, /maintenance-run-card maintenance-run-card--manual/);
  assert.match(page, /maintenance-run-card maintenance-run-card--upgrade/);

  assert.match(styles, /grid-template-areas:\s*"schedule"\s*"manual"\s*"upgrade"/);
  assert.doesNotMatch(styles, /grid-template-areas:\s*"schedule manual"/);
  assert.match(styles, /\.maintenance-plan-card--schedule\s*\{[^}]*grid-area:\s*schedule;/s);
  assert.match(styles, /\.maintenance-run-card--manual\s*\{[^}]*grid-area:\s*manual;/s);
  assert.match(styles, /\.maintenance-run-card--upgrade\s*\{[^}]*grid-area:\s*upgrade;/s);
});

test('maintenance page uses unified Chinese copy only', () => {
  const page = fs.readFileSync(pagePath, 'utf8');

  assert.doesNotMatch(page, />Source</);
  assert.doesNotMatch(page, /All enabled sources/);
  assert.doesNotMatch(page, />Manual</);
  assert.doesNotMatch(page, /Auto upgrade/);
  assert.doesNotMatch(page, /自动升级/);

  for (const required of ['日志源', '全部启用日志源', '手动入库', '执行入库', '版本升级', '手动升级', '选择离线升级包', '上传并安装']) {
    assert.match(page, new RegExp(required), `维护页缺少统一文案：${required}`);
  }
});

test('auto scan time picker is owned by form field instead of a string controlled value', () => {
  const page = fs.readFileSync(pagePath, 'utf8');

  assert.match(page, /<Form\.Item name="auto_scan_times" noStyle>/);
  assert.doesNotMatch(page, /value=\{autoScanTimeValue\(autoScanTime\)\}/);
  assert.doesNotMatch(page, /setFieldValue\('auto_scan_times'/);
  assert.match(page, /auto_scan_times: autoScanTimeValue\(settings\.auto_scan_times\)/);
  assert.match(page, /auto_scan_times: formatAutoScanTime\(values\.auto_scan_times\)/);
});

test('full rebuild switch makes all-date rebuild explicit', () => {
  const page = fs.readFileSync(pagePath, 'utf8');
  const styles = fs.readFileSync(stylesPath, 'utf8');

  assert.match(page, /const \[fullRebuild, setFullRebuild\]/);
  assert.match(page, /<Switch checked=\{fullRebuild\} onChange=\{setFullRebuild\} \/>/);
  assert.match(page, /disabled=\{fullRebuild\}/);
  assert.match(page, /await trigger\('\/api\/rebuild', '已触发全量重建'\)/);
  assert.match(page, /await trigger\(`\/api\/rebuild\?date=\$\{rebuildDate\.format\('YYYY-MM-DD'\)\}`/);
  assert.match(styles, /\.maintenance-rebuild-mode\s*\{/);
});
