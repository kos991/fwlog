import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import test from 'node:test';

const source = fs.readFileSync(path.resolve('src/pages/SystemMaintenancePage.tsx'), 'utf8');

test('source selection is limited to saved import and rebuild operations', () => {
  assert.match(source, /const \[savedLogSources, setSavedLogSources\]/);
  assert.match(source, /sourceScoped && importSourceID/);
  assert.match(source, /trigger\('\/api\/sync',[^\n]+true\)/);
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
