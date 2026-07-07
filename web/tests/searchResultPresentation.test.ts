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
