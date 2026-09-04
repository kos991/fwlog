import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import test from 'node:test';

const page = fs.readFileSync(path.resolve('src/pages/UpgradePreviewPage.tsx'), 'utf8');
const main = fs.readFileSync(path.resolve('src/main.tsx'), 'utf8');
const styles = fs.readFileSync(path.resolve('src/styles.css'), 'utf8');

test('upgrade preview follows the approved version management structure', () => {
  for (const required of [
    '系统版本',
    '发布时间',
    '数据引擎版本',
    'IP 地址库更新时间',
    '自动升级',
    '版本通道',
    '正式版',
    '体验版',
    '仅随完整安装包升级，不独立更新',
    '切换通道不会立即安装版本',
  ]) {
    assert.match(page, new RegExp(required), `升级预览缺少：${required}`);
  }

  assert.match(main, /preview'\) === 'upgrade'/);
  assert.match(main, /<UpgradePreviewPage \/>/);
  assert.match(styles, /\.upgrade-version-band\s*\{/);
  assert.match(styles, /\.upgrade-policy-grid\s*\{/);
  assert.match(styles, /@media \(max-width: 720px\)/);
  assert.doesNotMatch(page, /安全规则版本|安全规则自动升级/);
});
