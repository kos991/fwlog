import assert from 'node:assert/strict';
import test from 'node:test';
import {
  formatConfidence,
  riskColor,
  riskText,
  verdictText,
} from '../src/threatIntelligence.ts';

test('renders stable Chinese verdict, risk, color, and confidence copy', () => {
  assert.equal(verdictText('malicious'), '恶意');
  assert.equal(verdictText('unknown'), '未知');
  assert.equal(riskText('critical'), '严重');
  assert.equal(riskColor('critical'), 'red');
  assert.equal(formatConfidence(96, 'unknown'), '96');
  assert.equal(formatConfidence(null, 'high'), '高');
});
