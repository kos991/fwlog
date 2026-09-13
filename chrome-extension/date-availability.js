(function initializeDateAvailability(root) {
  const statusPriority = { failed: 5, importing: 4, pending: 3, skipped: 2, ready: 1 };

  function pad(value) {
    return String(value).padStart(2, '0');
  }

  function validDateParts(year, month, day, hour = 0, minute = 0, second = 0) {
    const date = new Date(year, month - 1, day, hour, minute, second);
    return date.getFullYear() === year
      && date.getMonth() === month - 1
      && date.getDate() === day
      && date.getHours() === hour
      && date.getMinutes() === minute
      && date.getSeconds() === second;
  }

  function dateKey(value) {
    const match = String(value || '').match(/^(\d{4})-(\d{2})-(\d{2})/);
    if (!match) return '';
    const [year, month, day] = match.slice(1).map(Number);
    return validDateParts(year, month, day) ? `${year}-${pad(month)}-${pad(day)}` : '';
  }

  function parseDateTimeInput(value) {
    const text = String(value || '').trim();
    const normalized = text.replace('T', ' ');
    const separated = normalized.match(/^(\d{4})[-/](\d{1,2})[-/](\d{1,2})\s+(\d{1,2}):(\d{2})(?::(\d{2}))?$/);
    const compact = normalized.match(/^(\d{4})(\d{2})(\d{2})(\d{2})(\d{2})(\d{2})?$/);
    const match = separated || compact;
    if (!match) return '';
    const year = Number(match[1]);
    const month = Number(match[2]);
    const day = Number(match[3]);
    const hour = Number(match[4]);
    const minute = Number(match[5]);
    const second = Number(match[6] || 0);
    if (!validDateParts(year, month, day, hour, minute, second)) return '';
    return `${year}-${pad(month)}-${pad(day)} ${pad(hour)}:${pad(minute)}:${pad(second)}`;
  }

  function toInputDateTime(value) {
    const parsed = parseDateTimeInput(value);
    return parsed ? parsed.slice(0, 16) : '';
  }

  function stateKind(status) {
    if (status === 'ready') return 'ready';
    if (status === 'importing') return 'importing';
    if (status === 'failed') return 'failed';
    if (status === 'no_data') return 'skipped';
    return 'pending';
  }

  function stateTitle(item, kind) {
    if (kind === 'ready') return `已完成，可查询 ${Number(item.rows_imported || 0).toLocaleString('zh-CN')} 行`;
    if (kind === 'importing') return `正在入库，${Math.round(Number(item.progress_pct || 0))}%`;
    if (kind === 'failed') return item.error || '处理失败';
    if (kind === 'skipped') return '无有效日志';
    return '等待处理';
  }

  function buildDateAvailability(payload) {
    const dates = {};
    const items = Array.isArray(payload?.dates) ? payload.dates : [];
    const readyDateSet = new Set();
    for (const item of items) {
      const key = dateKey(item?.log_date);
      if (!key) continue;
      const kind = stateKind(item?.status);
      if (kind === 'ready') readyDateSet.add(key);
      const previous = dates[key];
      if (!previous || statusPriority[kind] > statusPriority[previous.kind]) {
        dates[key] = { kind, title: stateTitle(item, kind) };
      }
    }
    const readyDates = Array.from(readyDateSet).sort();
    return {
      dates,
      readyDates,
      firstReady: readyDates[0] || '',
      latestReady: readyDates[readyDates.length - 1] || '',
    };
  }

  function monthCells(year, month) {
    const first = new Date(year, month - 1, 1);
    const start = new Date(year, month - 1, 1 - first.getDay());
    return Array.from({ length: 42 }, (_, index) => {
      const date = new Date(start.getFullYear(), start.getMonth(), start.getDate() + index);
      const key = `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`;
      return { key, day: date.getDate(), inMonth: date.getMonth() === month - 1 };
    });
  }

  const api = { buildDateAvailability, dateKey, monthCells, parseDateTimeInput, toInputDateTime };
  root.FWLOGDateAvailability = api;
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
})(globalThis);
