import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import test from 'node:test';

const pagePath = path.resolve('src/pages/SystemMaintenancePage.tsx');
const stylesPath = path.resolve('src/styles.css');

test('maintenance page pins schedule, manual actions, and upgrade cards into explicit grid areas', () => {
  const page = fs.readFileSync(pagePath, 'utf8');
  const styles = fs.readFileSync(stylesPath, 'utf8');

  assert.match(page, /maintenance-plan-card maintenance-plan-card--schedule/);
  assert.match(page, /maintenance-run-card maintenance-run-card--manual/);
  assert.match(page, /maintenance-run-card maintenance-run-card--upgrade/);

  assert.match(styles, /grid-template-areas:\s*"schedule manual"\s*"upgrade manual"/);
  assert.match(styles, /\.maintenance-plan-card--schedule\s*\{[^}]*grid-area:\s*schedule;/s);
  assert.match(styles, /\.maintenance-run-card--manual\s*\{[^}]*grid-area:\s*manual;/s);
  assert.match(styles, /\.maintenance-run-card--upgrade\s*\{[^}]*grid-area:\s*upgrade;/s);
});

test('auto scan time picker is owned by form field instead of a string controlled value', () => {
  const page = fs.readFileSync(pagePath, 'utf8');

  assert.match(page, /<Form\.Item name="auto_scan_times" noStyle>/);
  assert.doesNotMatch(page, /value=\{autoScanTimeValue\(autoScanTime\)\}/);
  assert.doesNotMatch(page, /setFieldValue\('auto_scan_times'/);
  assert.match(page, /auto_scan_times: autoScanTimeValue\(settings\.auto_scan_times\)/);
  assert.match(page, /auto_scan_times: formatAutoScanTime\(values\.auto_scan_times\)/);
});
