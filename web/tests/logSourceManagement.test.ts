import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import test from 'node:test';

const pagePath = path.resolve('src/pages/SystemMaintenancePage.tsx');
const stylesPath = path.resolve('src/styles.css');

test('log source management uses compact rows and a modal editor', () => {
  const page = fs.readFileSync(pagePath, 'utf8');

  assert.match(page, /className="source-management-list"/);
  assert.match(page, /className="source-management-row source-management-row--header"/);
  assert.match(page, /<Modal/);
  assert.match(page, /编辑 RSyslog 接收源/);
  assert.match(page, /客户端 IP \/ 网段/);
  assert.match(page, /归档目录/);
  assert.match(page, /归档保留天数/);
  assert.match(page, /留空时压缩文件保留在落盘目录，不移动/);
  assert.match(page, /0 表示永久保留/);
  assert.doesNotMatch(page, /<Form\.List name="log_sources">/);
  assert.doesNotMatch(page, /className=\{`source-item-card/);
});

test('source CRUD persists the complete list immediately without stale form ownership', () => {
  const page = fs.readFileSync(pagePath, 'utf8');

  assert.match(page, /async function persistLogSources\(next: LogSourceSetting\[\]\)/);
  assert.match(page, /apiPost<Settings>\('\/api\/settings', \{\s*log_sources: JSON\.stringify\(normalized\)/s);
  assert.match(page, /await persistLogSources\(next\)/);
  assert.match(page, /const \{ log_sources: _ignoredLogSources, \.\.\.settingsValues \} = values;/);
  assert.match(page, /description="只删除配置，不会删除已落盘或已归档文件。"/);
  assert.match(page, /aria-label="编辑日志源"/);
  assert.match(page, /aria-label="删除日志源"/);
});

test('receiver status and responsive source rows expose client and archive state', () => {
  const page = fs.readFileSync(pagePath, 'utf8');
  const styles = fs.readFileSync(stylesPath, 'utf8');

  assert.match(page, /apiGet<ReceiverStatusMap>\('\/api\/receiver\/status'\)/);
  assert.match(page, /last_client_ip/);
  assert.match(page, /received_messages/);
  assert.match(page, /archive_error/);
  assert.match(page, /archive_retention_days/);
  assert.match(styles, /\.source-management-row\s*\{[^}]*grid-template-columns:/s);
  assert.match(styles, /@media \(max-width: 900px\)[\s\S]*\.source-management-row\s*\{[^}]*grid-template-columns:\s*1fr;/);
  assert.doesNotMatch(styles, /\.source-management-list\s*\{[^}]*min-width:\s*\d+px/s);
});
