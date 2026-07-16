import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import test from 'node:test';
import { ingestStatusText } from '../src/uiCopy.ts';

test('统一入库状态使用清晰一致的中文', () => {
  assert.equal(ingestStatusText(), '暂无任务');
  assert.equal(ingestStatusText('idle'), '暂无任务');
  assert.equal(ingestStatusText('pending'), '等待处理');
  assert.equal(ingestStatusText('scanning'), '正在扫描');
  assert.equal(ingestStatusText('importing'), '正在入库');
  assert.equal(ingestStatusText('ready'), '已完成');
  assert.equal(ingestStatusText('succeeded'), '已完成');
  assert.equal(ingestStatusText('failed'), '处理失败');
  assert.equal(ingestStatusText('no_data'), '无有效日志');
  assert.equal(ingestStatusText('custom'), 'custom');
});

test('后台概览与进度页使用明确的统计和任务文案', () => {
  const dashboard = fs.readFileSync(path.resolve('src/pages/HealthDashboard.tsx'), 'utf8');
  const progress = fs.readFileSync(path.resolve('src/pages/IncrementalProgressPage.tsx'), 'utf8');

  for (const text of ['已入库日志', '可查询日期范围', '今日新增日志', '日志存储占用', '当前入库任务']) {
    assert.match(dashboard, new RegExp(text));
  }
  for (const text of ['显示已完成', '仅未完成', '当前任务', '按日期查看进度', '来源标识']) {
    assert.match(progress, new RegExp(text));
  }
  assert.doesNotMatch(progress, />Source</);
});

test('入库进度轮询不因响应数组引用变化而重复重启', () => {
  const progress = fs.readFileSync(path.resolve('src/pages/IncrementalProgressPage.tsx'), 'utf8');

  assert.match(progress, /const anyImporting = Boolean\(data\?\.sources\?\.some/);
  assert.match(progress, /\}, \[load, anyImporting\]\);/);
  assert.doesNotMatch(progress, /\}, \[load, data\?\.status, data\?\.sources\]\);/);
});

test('日志查询页使用统一的来源和结果文案', () => {
  const search = fs.readFileSync(path.resolve('src/pages/LogSearchPage.tsx'), 'utf8');

  for (const text of ['日志来源', '任意 IP', '访问结果', '来源名称', '已完成，可查询', '等待处理']) {
    assert.match(search, new RegExp(text));
  }
});
