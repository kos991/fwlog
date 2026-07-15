import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import test from 'node:test';
import { formatIPPort, geoFallback, matchCidrAlias } from '../src/ipAddress.ts';

test('日志查询页使用统一的 IPv4 和 IPv6 地址工具', () => {
  const page = fs.readFileSync(path.resolve('src/pages/LogSearchPage.tsx'), 'utf8');
  const packageJson = fs.readFileSync(path.resolve('package.json'), 'utf8');

  assert.match(page, /from '\.\.\/ipAddress'/);
  assert.doesNotMatch(page, /function ipv4ToNumber/);
  assert.match(packageJson, /"ipaddr\.js"/);
});

test('IPv6 地址与端口使用方括号避免歧义', () => {
  assert.equal(formatIPPort('2001:db8::10', 443), '[2001:db8::10]:443');
  assert.equal(formatIPPort('192.0.2.10', 443), '192.0.2.10:443');
});

test('CIDR 别名同时支持双栈并选择最长前缀', () => {
  const aliases = [
    { cidr: '2001:db8::/32', alias: 'IPv6 网络' },
    { cidr: '2001:db8:1::/48', alias: 'IPv6 业务区' },
    { cidr: '192.0.2.0/24', alias: 'IPv4 网络' },
  ];

  assert.equal(matchCidrAlias('2001:db8:1::20', aliases), 'IPv6 业务区');
  assert.equal(matchCidrAlias('192.0.2.20', aliases), 'IPv4 网络');
});

test('IPv6 内网与公网地址使用一致的兜底文案', () => {
  assert.equal(geoFallback('fd00::1'), '内网');
  assert.equal(geoFallback('fe80::1'), '内网');
  assert.equal(geoFallback('2001:4860:4860::8888'), '未知公网');
});
