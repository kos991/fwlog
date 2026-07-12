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
