import assert from 'node:assert/strict';
import test from 'node:test';
import {
  formatConfidence,
  riskColor,
  riskText,
  verdictText,
  visibleThreatProviders,
  type ThreatProviderStatus,
} from '../src/threatIntelligence.ts';

test('shows only enabled providers with usable credentials', () => {
  const statuses = [
    { provider: 'threatbook', name: '微步', enabled: true, configured: true },
    { provider: 'nsfocus', name: '绿盟', enabled: false, configured: true },
    { provider: 'qianxin', name: '奇安信', enabled: true, configured: false },
    { provider: 'tencent', name: '腾讯', enabled: true, configured: true, last_test_status: 'failed' },
  ] satisfies ThreatProviderStatus[];

  assert.deepEqual(visibleThreatProviders(statuses).map((item) => item.provider), ['threatbook']);
});

test('renders stable Chinese verdict, risk, color, and confidence copy', () => {
  assert.equal(verdictText('malicious'), '恶意');
  assert.equal(verdictText('unknown'), '未知');
  assert.equal(riskText('critical'), '严重');
  assert.equal(riskColor('critical'), 'red');
  assert.equal(formatConfidence(96, 'unknown'), '96');
  assert.equal(formatConfidence(null, 'high'), '高');
});
