import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import test from 'node:test';

const source = fs.readFileSync(path.resolve('src/api.ts'), 'utf8');

test('api mock uses readable Chinese log source copy', () => {
  for (const forbidden of ['娣变俊', '鏈?', '鍐呯綉', '鍏綉', '涓浗', '骞夸笢', '浠嶅湪鍏ュ簱', '璇锋眰澶辫触']) {
    assert.equal(source.includes(forbidden), false, `mock 中不应保留乱码：${forbidden}`);
  }

  for (const required of ['深信服 NAT', '内网', '公网', '中国', '广东', '办公终端', '仍在入库', '请求失败']) {
    assert.equal(source.includes(required), true, `mock 中缺少中文文案：${required}`);
  }
});

test('api mock includes rsyslog log source defaults', () => {
  assert.match(source, /source_type:\s*'rsyslog'/);
  assert.match(source, /listen_protocol:\s*'udp'/);
  assert.match(source, /listen_port:\s*5514/);
  assert.match(source, /spool_dir:\s*'\/data\/fwlog\/received\/rsyslog-main'/);
});
