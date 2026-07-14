const INGEST_STATUS_TEXT: Record<string, string> = {
  idle: '暂无任务',
  pending: '等待处理',
  scanning: '正在扫描',
  importing: '正在入库',
  ready: '已完成',
  succeeded: '已完成',
  failed: '处理失败',
  skipped: '未处理',
};

export function ingestStatusText(status?: string) {
  return INGEST_STATUS_TEXT[status || 'idle'] || status || '暂无任务';
}
