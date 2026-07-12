import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import test from 'node:test';

const source = fs.readFileSync(path.resolve('src/pages/LogSearchPage.tsx'), 'utf8');

test('column filters restart the server query instead of filtering the current page', () => {
  assert.match(source, /applyColumnFilter/);
  assert.match(source, /runSearch\(values, 1, queryPageSize, undefined, true\)/);
  assert.doesNotMatch(source, /onFilter:/);
});
