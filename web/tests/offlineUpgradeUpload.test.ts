import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import test from 'node:test';

const page = fs.readFileSync(path.resolve('src/pages/SystemMaintenancePage.tsx'), 'utf8');
const api = fs.readFileSync(path.resolve('src/api.ts'), 'utf8');

test('maintenance page uploads only fwlog-upgrade rpm or deb packages', () => {
  assert.match(page, /accept="\.rpm,\.deb"/);
  assert.match(page, /x86_64\\\.rpm\|_\.\+_amd64\\\.deb/);
  assert.match(page, /apiUpload<UpgradeStatus>\('\/api\/upgrade\/upload', upgradeFile\)/);
  assert.match(api, /body\.append\('package', file\)/);
});
