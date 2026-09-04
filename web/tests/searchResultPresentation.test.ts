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

test('search date picker accepts manually typed compact date times', () => {
  const page = fs.readFileSync(pagePath, 'utf8');

  assert.match(page, /const queryDateTimeFormat = 'YYYY-MM-DD HH:mm:ss';/);
  assert.match(page, /'YYYY-MM-DD HH:mm'/);
  assert.match(page, /'YYYYMMDDHHmmss'/);
  assert.match(page, /'YYYYMMDDHHmm'/);
  assert.match(page, /format=\{queryDateTimeInputFormats\}/);
  assert.match(page, /label="日期（支持手动输入）"/);
  assert.match(page, /placeholder=\{\['开始时间（支持手动输入）', '结束时间（支持手动输入）'\]\}/);
  assert.match(page, /start: range\?\.\[0\]\?\.format\(queryDateTimeFormat\)/);
  assert.match(page, /end: range\?\.\[1\]\?\.format\(queryDateTimeFormat\)/);
});

test('search page exposes log source filter and device identity', () => {
  const page = fs.readFileSync(pagePath, 'utf8');

  assert.match(page, /source_id\?: string;/);
  assert.match(page, /logSourceOptions/);
  assert.match(page, /source_id: values\.source_id/);
  assert.match(page, /name="source_id" label="日志来源"/);
  assert.match(page, /title: '来源标识'/);
  assert.match(page, /dataIndex: 'source_id'/);
});

test('search date picker uses compact status markers instead of clipped labels', () => {
  const page = fs.readFileSync(pagePath, 'utf8');

  assert.match(page, /visible-date-marker/);
  assert.doesNotMatch(page, /visible-date-label/);
  assert.doesNotMatch(page, /label: '可查'/);
});

test('search results include threat intelligence actions for destination IPs', () => {
  const page = fs.readFileSync(pagePath, 'utf8');

  assert.match(page, /ThreatIntelligenceActions/);
  assert.match(page, /row\.dst_ip/);
  assert.doesNotMatch(page, /ThreatIntelligenceActions[^\n]*dst_port/);
});
