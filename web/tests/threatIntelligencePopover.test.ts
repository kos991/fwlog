import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import test from 'node:test';

const componentPath = path.resolve('src/components/ThreatIntelligencePopover.tsx');
const pagePath = path.resolve('src/pages/LogSearchPage.tsx');
const stylesPath = path.resolve('src/styles.css');

test('popover loads local history before explicit analysis', () => {
  const component = fs.readFileSync(componentPath, 'utf8');

  assert.match(component, /\/results\?ip=/);
  assert.match(component, /开始分析/);
  assert.match(component, /重新分析/);
  assert.match(component, /onClick=\{\(\) => void analyze\(\)\}/);
  assert.doesNotMatch(component, /onOpenChange=\{[^}]*analyze/s);
  assert.match(component, /本次分析失败/);
  assert.match(component, /原始详情/);
  assert.match(component, /raw_response/);
});

test('popover uses local provider icons and a bounded mobile width', () => {
  const component = fs.readFileSync(componentPath, 'utf8');
  const styles = fs.readFileSync(stylesPath, 'utf8');

  for (const icon of ['threatbook.svg', 'nsfocus.svg', 'qianxin.svg', 'tencent.svg']) {
    assert.match(component, new RegExp(icon.replace('.', '\\.')));
    assert.ok(fs.statSync(path.resolve(`src/assets/threat-intelligence/${icon}`)).size > 0);
  }
  assert.match(component, /aria-label=/);
  assert.match(styles, /\.threat-intelligence-popover[^}]*width:\s*380px/s);
  assert.match(styles, /max-width:\s*calc\(100vw - 24px\)/);
});

test('search page loads provider status independently and passes only destination IP', () => {
  const page = fs.readFileSync(pagePath, 'utf8');

  assert.match(page, /ThreatIntelligenceActions/);
  assert.match(page, /apiGet<[^>]*ThreatProviderListResponse[^>]*>\('\/api\/threat-intelligence\/providers'\)/);
  assert.match(page, /setThreatProviders\(\[\]\)/);
  assert.match(page, /<ThreatIntelligenceActions[\s\S]*ip=\{row\.dst_ip \|\| ''\}/);
  assert.doesNotMatch(page, /<ThreatIntelligenceActions[^\n]*dst_port/);
});
