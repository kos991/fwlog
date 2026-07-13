import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import test from 'node:test';

const source = fs.readFileSync(path.resolve('src/pages/SystemMaintenancePage.tsx'), 'utf8');

test('source selection applies to sync and rebuild ingest operations', () => {
  assert.match(source, /const \[savedLogSources, setSavedLogSources\]/);
  assert.match(source, /function buildIngestPath/);
  assert.match(source, /sourceID/);
  assert.match(source, /date_from/);
  assert.match(source, /date_to/);
  assert.match(source, /apiPost\(buildIngestPath\(\)\)/);
  assert.match(source, /trigger\('\/api\/ip-data\/reload',[^\n]+\)/);
});

test('log source form supports file and rsyslog source types', () => {
  assert.match(source, /source_type/);
  assert.match(source, /文件目录/);
  assert.match(source, /RSyslog 接收/);
  assert.match(source, /listen_port/);
  assert.match(source, /spool_dir/);
  assert.match(source, /默认 5514/);
});
