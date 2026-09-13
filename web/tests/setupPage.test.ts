import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import test from 'node:test';

const root = path.resolve(process.cwd());
test('首次初始化页面和启动分支已接入', () => {
  const page = fs.readFileSync(path.join(root, 'src/pages/SetupPage.tsx'), 'utf8');
  const main = fs.readFileSync(path.join(root, 'src/main.tsx'), 'utf8');
  const api = fs.readFileSync(path.join(root, 'src/api.ts'), 'utf8');
  assert.match(page, /初始化管理员/);
  assert.match(page, /admin/);
  assert.match(page, /confirm_password/);
  assert.match(page, /visibilityToggle/);
  assert.match(page, /onComplete/);
  assert.match(main, /\/api\/setup\/status/);
  assert.match(main, /<SetupPage/);
  assert.match(api, /mockSetupRequired/);
});
