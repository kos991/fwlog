import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import test from 'node:test';

const pagePath = path.resolve('src/pages/LogSearchPage.tsx');

test('search page shows actual total count instead of current page count only', () => {
  const page = fs.readFileSync(pagePath, 'utf8');

  assert.match(page, /const currentTotal = response\?\.total \|\| 0;/);
  assert.match(page, /showTotal: \(\) => response \? `第 \$\{currentPage\} 页，共 \$\{currentTotal\} 条` : ''/);
  assert.match(page, /第 <span className="mono-number">\{currentPage\}<\/span> 页，共 <span className="mono-number">\{currentTotal\}<\/span> 条/);
});

test('search page exposes log source filter and device identity', () => {
  const page = fs.readFileSync(pagePath, 'utf8');

  assert.match(page, /source_id\?: string;/);
  assert.match(page, /logSourceOptions/);
  assert.match(page, /source_id: values\.source_id/);
  assert.match(page, /name="source_id" label="日志源"/);
  assert.match(page, /title: '设备 ID'/);
  assert.match(page, /dataIndex: 'source_id'/);
});

test('search date picker uses compact status markers instead of clipped labels', () => {
  const page = fs.readFileSync(pagePath, 'utf8');

  assert.match(page, /visible-date-marker/);
  assert.doesNotMatch(page, /visible-date-label/);
  assert.doesNotMatch(page, /label: '可查'/);
});
