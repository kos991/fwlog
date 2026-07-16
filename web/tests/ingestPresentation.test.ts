import assert from 'node:assert/strict';
import { buildIngestProgressView } from '../src/ingestPresentation.ts';

const view = buildIngestProgressView({
  status: 'importing',
  current_date: '2026-05-23',
  current_file: '',
  files_total: 65,
  files_done: 0,
  rows_imported: 0,
  bytes_total: 0,
  bytes_done: 0,
  progress_pct: 0,
  last_updated_at: '2026-07-03T10:16:30+08:00',
});

assert.equal(view.percent, 0);
assert.equal(view.displayPercent, 2);
assert.equal(view.percentText, '处理中');
assert.equal(view.currentFileText, '等待当前文件');
assert.equal(view.fileProgressText, '0 / 65 文件');
assert.equal(view.rowsText, '0 行');
assert.equal(view.bytesText, '0 B / 0 B');
assert.equal(view.updatedText, '2026-07-03 10:16:30');
assert.equal(view.detailText, '已发现 65 个文件，等待解析或写入');

const subOnePercentView = buildIngestProgressView({
  status: 'importing',
  files_total: 200,
  files_done: 0,
  progress_pct: 0.4,
});

assert.equal(subOnePercentView.percent, 0);
assert.equal(subOnePercentView.displayPercent, 1);
assert.equal(subOnePercentView.percentText, '<1%');

const finalizingView = buildIngestProgressView({
  status: 'importing',
  files_total: 165,
  files_done: 164,
  progress_pct: 99.39,
});

assert.equal(finalizingView.displayPercent, 99);
assert.equal(finalizingView.percentText, '收尾中');
assert.equal(finalizingView.detailText, '正在处理最后 1 个文件');

const fullyReadButFinalizingView = buildIngestProgressView({
  status: 'importing',
  files_total: 2,
  files_done: 2,
  rows_imported: 3197207,
  progress_pct: 100,
});

assert.equal(fullyReadButFinalizingView.percentText, '收尾中');
assert.equal(fullyReadButFinalizingView.detailText, '文件已读取完成，正在确认入库结果');

const completedView = buildIngestProgressView({
  status: 'ready',
  files_total: 2,
  files_done: 2,
  rows_imported: 3197207,
  progress_pct: 100,
});

assert.equal(completedView.percentText, '100%');
assert.equal(completedView.detailText, '已完成 2 个文件，共入库 3,197,207 行');

const noDataView = buildIngestProgressView({
  status: 'no_data',
  files_total: 1,
  files_done: 1,
  rows_imported: 0,
  progress_pct: 100,
});

assert.equal(noDataView.percentText, '无数据');
assert.equal(noDataView.detailText, '文件中没有可入库的日志');
