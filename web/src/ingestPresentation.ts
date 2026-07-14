export type IngestProgressInput = {
  status?: string;
  current_date?: string;
  current_file?: string;
  files_total?: number;
  files_done?: number;
  rows_imported?: number;
  bytes_total?: number;
  bytes_done?: number;
  progress_pct?: number;
  last_updated_at?: string;
};

export type IngestProgressView = {
  percent: number;
  displayPercent: number;
  percentText: string;
  currentFileText: string;
  fileProgressText: string;
  rowsText: string;
  bytesText: string;
  updatedText: string;
  detailText: string;
};

function formatCount(value?: number) {
  return new Intl.NumberFormat('zh-CN').format(value ?? 0);
}

function formatBytes(bytes?: number) {
  if (!bytes) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let size = bytes;
  let unit = 0;
  while (size >= 1024 && unit < units.length - 1) {
    size /= 1024;
    unit += 1;
  }
  return `${size.toFixed(size >= 100 ? 0 : 1)} ${units[unit]}`;
}

function formatTimestamp(value?: string) {
  if (!value) return '-';
  const normalized = value.replace('T', ' ').replace(/([+-]\d{2}:\d{2}|Z)$/, '');
  return normalized.slice(0, 19) || '-';
}

export function buildIngestProgressView(ingest?: IngestProgressInput | null): IngestProgressView {
  const filesTotal = ingest?.files_total ?? 0;
  const filesDone = ingest?.files_done ?? 0;
  const rowsImported = ingest?.rows_imported ?? 0;
  const bytesDone = ingest?.bytes_done ?? 0;
  const bytesTotal = ingest?.bytes_total ?? 0;
  const rawPercent = Math.max(0, Math.min(100, ingest?.progress_pct ?? 0));
  const percent = Math.round(rawPercent);
  const isStarting = ingest?.status === 'importing' && percent === 0 && filesTotal > 0;
  const isFinalizing = ingest?.status === 'importing' && rawPercent >= 99 && filesTotal > 0;
  const isComplete = ingest?.status === 'ready' || ingest?.status === 'succeeded';
  const displayPercent = isStarting ? (rawPercent > 0 ? 1 : 2) : percent;
  const percentText = isFinalizing ? '收尾中' : rawPercent > 0 && rawPercent < 1 ? '<1%' : isStarting ? '处理中' : `${percent}%`;
  const currentFileText = ingest?.current_file || '等待当前文件';

  let detailText = '暂无入库任务';
  if (ingest?.status === 'failed') {
    detailText = '入库失败，请查看维护操作';
  } else if (isComplete) {
    detailText = `已完成 ${formatCount(filesDone)} 个文件，共入库 ${formatCount(rowsImported)} 行`;
  } else if (percent === 0 && filesTotal > 0) {
    detailText = `已发现 ${formatCount(filesTotal)} 个文件，等待解析或写入`;
  } else if (isFinalizing) {
    detailText = filesDone >= filesTotal
      ? '文件已读取完成，正在确认入库结果'
      : `正在处理最后 ${formatCount(filesTotal - filesDone)} 个文件`;
  } else if (filesTotal > 0) {
    detailText = `正在处理 ${formatCount(filesDone)} / ${formatCount(filesTotal)} 个文件`;
  } else if (ingest?.status === 'importing') {
    detailText = '正在扫描日志文件';
  }

  return {
    percent,
    displayPercent,
    percentText,
    currentFileText,
    fileProgressText: `${formatCount(filesDone)} / ${formatCount(filesTotal)} 文件`,
    rowsText: `${formatCount(rowsImported)} 行`,
    bytesText: `${formatBytes(bytesDone)} / ${formatBytes(bytesTotal)}`,
    updatedText: formatTimestamp(ingest?.last_updated_at),
    detailText,
  };
}
