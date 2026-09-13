const DEFAULT_API = 'https://192.168.0.130:8080';
let mode = 'password';
let token = '';
let submitting = false;
let queryState = { page: 1, cursor: '', cursorStack: [''], values: null };
let settingsReturnView = 'login';
let pluginEnabled = true;
let workspacePage = 'search';
let dateInputsEdited = false;
let calendarTarget = 'start';
let calendarMonth = new Date(new Date().getFullYear(), new Date().getMonth(), 1);
let dateAvailability = { dates: {}, readyDates: [], firstReady: '', latestReady: '' };

const ENABLED_ICONS = { 16: 'icons/icon-16.png', 32: 'icons/icon-32.png', 48: 'icons/icon-48.png', 128: 'icons/icon-128.png' };
const DISABLED_ICONS = { 16: 'icons/icon-disabled-16.png', 32: 'icons/icon-disabled-32.png', 48: 'icons/icon-disabled-48.png', 128: 'icons/icon-disabled-128.png' };
const { availablePercent, summarizeReceiverStatus } = globalThis.FWLOGStatusMetrics;
const { buildDateAvailability, monthCells, parseDateTimeInput, toInputDateTime } = globalThis.FWLOGDateAvailability;
const reducedMotion = matchMedia('(prefers-reduced-motion: reduce)');

const $ = (id) => document.getElementById(id);
const show = (id, visible) => { $(id).hidden = !visible; };
const errorText = (target, error) => { $(target).textContent = error instanceof Error ? error.message : String(error); };

async function apiBase() {
  if (globalThis.chrome?.storage?.local) {
    return (await chrome.storage.local.get({ apiBase: DEFAULT_API })).apiBase.replace(/\/$/, '');
  }
  return (localStorage.getItem('fwlog_api_base') || DEFAULT_API).replace(/\/$/, '');
}

async function saveApiBase(value) {
  if (globalThis.chrome?.storage?.local) await chrome.storage.local.set({ apiBase: value });
  else localStorage.setItem('fwlog_api_base', value);
}

async function readStoredToken() {
  if (globalThis.chrome?.storage?.session) return (await chrome.storage.session.get({ accessToken: '' })).accessToken;
  return sessionStorage.getItem('fwlog_access_token') || '';
}

async function storeToken(value) {
  if (globalThis.chrome?.storage?.session) await chrome.storage.session.set({ accessToken: value });
  else sessionStorage.setItem('fwlog_access_token', value);
}

async function clearStoredToken() {
  if (globalThis.chrome?.storage?.session) await chrome.storage.session.remove('accessToken');
  else sessionStorage.removeItem('fwlog_access_token');
}

async function readPluginEnabled() {
  if (globalThis.chrome?.storage?.local) return (await chrome.storage.local.get({ pluginEnabled: true })).pluginEnabled !== false;
  return localStorage.getItem('fwlog_plugin_enabled') !== 'false';
}

async function savePluginEnabled(value) {
  if (globalThis.chrome?.storage?.local) await chrome.storage.local.set({ pluginEnabled: value });
  else localStorage.setItem('fwlog_plugin_enabled', String(value));
}

async function syncActionIcon(enabled) {
  if (!globalThis.chrome?.action?.setIcon) return;
  await chrome.action.setIcon({ path: enabled ? ENABLED_ICONS : DISABLED_ICONS });
  await chrome.action.setTitle({ title: enabled ? 'FWLOG 日志查询' : 'FWLOG 已停用' });
}

function setConnectionStatus(state, text) {
  $('connection-status').dataset.state = state;
  $('connection-status').querySelector('span').textContent = text;
}

function applyPluginEnabled(enabled) {
  pluginEnabled = enabled;
  $('plugin-toggle').setAttribute('aria-checked', String(enabled));
  $('plugin-toggle').setAttribute('aria-label', enabled ? '停用插件' : '启用插件');
  $('plugin-toggle').title = enabled ? '停用插件' : '启用插件';
  $('settings').disabled = !enabled;
  void syncActionIcon(enabled);
  show('disabled-panel', !enabled);
  if (!enabled) {
    show('settings-panel', false);
    show('login', false);
    show('query', false);
    setConnectionStatus('idle', '已停用');
    return;
  }
  show('disabled-panel', false);
  show('login', token === '');
  show('query', token !== '');
}

function setWorkspacePage(nextPage, refresh = true) {
  const changed = workspacePage !== nextPage;
  workspacePage = nextPage;
  if (nextPage !== 'search' && !$('date-picker').hidden) setDatePickerOpen(false);
  document.querySelector('.workspace-tabs').dataset.active = nextPage;
  document.querySelectorAll('.workspace-tabs button').forEach((button) => {
    const selected = button.dataset.page === nextPage;
    button.classList.toggle('active', selected);
    button.setAttribute('aria-selected', String(selected));
  });
  show('search-page', nextPage === 'search');
  show('status-page', nextPage === 'status');
  const page = $(nextPage === 'search' ? 'search-page' : 'status-page');
  if (changed && !reducedMotion.matches) {
    const direction = nextPage === 'status' ? 1 : -1;
    page.animate(
      [{ opacity: 0, transform: `translateX(${direction * 5}px)` }, { opacity: 1, transform: 'translateX(0)' }],
      { duration: 150, easing: 'cubic-bezier(.2, 0, 0, 1)' },
    );
  }
  if (nextPage === 'status' && refresh) void loadSystemMetrics();
}

async function probeConnection() {
  if (!pluginEnabled) return;
  if (!globalThis.chrome?.runtime?.id) {
    setConnectionStatus('connected', '已连接');
    return;
  }
  setConnectionStatus('checking', '检测中');
  try {
    const response = await fetch(`${await apiBase()}/api/setup/status`);
    setConnectionStatus(response.ok ? 'connected' : 'failed', response.ok ? '已连接' : '连接异常');
  } catch {
    setConnectionStatus('failed', '连接失败');
  }
}

async function activateQuery(nextToken, persist = true) {
  token = nextToken;
  if (persist) await storeToken(nextToken);
  show('settings-panel', false);
  show('login', false);
  show('query', true);
  setDefaultDates();
  setWorkspacePage('search', false);
  if (pluginEnabled) setConnectionStatus('connected', '已连接');
  void loadDateAvailability();
}

function setLoginMode(nextMode, focus = true) {
  mode = nextMode;
  const isChallenge = mode === 'challenge';
  document.querySelector('.switch').hidden = isChallenge;
  document.querySelectorAll('.switch button').forEach((item) => {
    const selected = item.dataset.mode === mode;
    item.classList.toggle('active', selected);
    item.setAttribute('aria-selected', String(selected));
  });
  show('password-row', mode === 'password');
  show('code-row', mode === 'mfa' || isChallenge);
  show('back-to-password', isChallenge);
  $('login-button').textContent = isChallenge ? '验证并登录' : '登录';
  $('login-error').textContent = '';
  if (mode === 'password') $('code').value = '';
  if (mode === 'mfa') $('password').value = '';
  if (focus) (mode === 'password' ? $('password') : $('code')).focus();
}

function resetLoginCard() {
  delete $('login').dataset.challenge;
  $('password').value = '';
  $('code').value = '';
  setLoginMode('password', false);
}

document.querySelectorAll('.switch button').forEach((button) => button.addEventListener('click', () => setLoginMode(button.dataset.mode)));
$('back-to-password').addEventListener('click', () => setLoginMode('password'));
$('password').addEventListener('input', () => { $('login-error').textContent = ''; });
$('code').addEventListener('input', () => { $('login-error').textContent = ''; });

async function login() {
  const payload = { mode, client: 'chrome_extension' };
  if (mode === 'mfa') payload.mfa_code = $('code').value.trim(); else payload.password = $('password').value;
  const response = await fetch(`${await apiBase()}/api/login`, { method: 'POST', headers: {'Content-Type':'application/json'}, body: JSON.stringify(payload) });
  const data = await response.json().catch(() => ({}));
  if (!response.ok) throw new Error(data.message || '登录失败');
  if (data.mfa_required) {
    $('login').dataset.challenge = data.challenge || '';
    $('code').value = '';
    setLoginMode('challenge');
    return;
  }
  if (!data.access_token) throw new Error('服务端未返回访问令牌');
  await activateQuery(data.access_token);
}

$('login-form').addEventListener('submit', async (event) => {
  event.preventDefault();
  if (!pluginEnabled) return;
  if (submitting) return;
  const value = mode === 'password' ? $('password').value : $('code').value.trim();
  if (!value) {
    $('login-error').textContent = mode === 'password' ? '请输入管理员密码' : '请输入 6 位动态验证码';
    (mode === 'password' ? $('password') : $('code')).focus();
    return;
  }
  if (mode !== 'password' && !/^\d{6}$/.test(value)) {
    $('login-error').textContent = '请输入 6 位数字验证码';
    $('code').focus();
    return;
  }
  submitting = true;
  $('login-button').disabled = true;
  $('login-button').setAttribute('aria-busy', 'true');
  $('login-button').textContent = '登录中…';
  try {
    $('login-error').textContent = '';
    if (mode === 'challenge') {
      const response = await fetch(`${await apiBase()}/api/login`, { method:'POST', headers:{'Content-Type':'application/json'}, body:JSON.stringify({ challenge:$('login').dataset.challenge, mfa_code:$('code').value.trim(), client:'chrome_extension' }) });
      const data = await response.json().catch(() => ({})); if (!response.ok) throw new Error(data.message || 'MFA 验证失败'); if (!data.access_token) throw new Error('服务端未返回访问令牌'); await activateQuery(data.access_token);
    } else await login();
  } catch (error) {
    errorText('login-error', error);
    (mode === 'password' ? $('password') : $('code')).focus();
  } finally {
    submitting = false;
    $('login-button').disabled = false;
    $('login-button').removeAttribute('aria-busy');
    $('login-button').textContent = mode === 'challenge' ? '验证并登录' : '登录';
  }
});

function setDefaultDates() {
  const end = new Date(); const start = new Date(end.getTime() - 24 * 3600 * 1000);
  const local = (date) => new Date(date.getTime() - date.getTimezoneOffset() * 60000).toISOString().slice(0,16).replace('T', ' ');
  $('start').value ||= local(start); $('end').value ||= local(end);
}

function setDateSyncState(state, text) {
  document.querySelector('.date-sync-toggle i').dataset.state = state;
  $('date-sync-text').textContent = text;
}

function setCalendarMonth(value) {
  const key = String(value || '').slice(0, 10);
  const match = key.match(/^(\d{4})-(\d{2})-(\d{2})$/);
  if (!match) return;
  calendarMonth = new Date(Number(match[1]), Number(match[2]) - 1, 1);
}

function selectedDate(id) {
  return parseDateTimeInput($(id).value).slice(0, 10);
}

function renderCalendar() {
  const year = calendarMonth.getFullYear();
  const month = calendarMonth.getMonth() + 1;
  const startDate = selectedDate('start');
  const endDate = selectedDate('end');
  $('calendar-title').textContent = `${year}年 ${month}月`;
  $('calendar-hint').textContent = calendarTarget === 'start' ? '选择开始日期' : '选择结束日期';
  $('calendar-grid').innerHTML = monthCells(year, month).map((cell) => {
    const state = dateAvailability.dates[cell.key];
    const selected = cell.key === startDate || cell.key === endDate;
    const inRange = startDate && endDate && cell.key > startDate && cell.key < endDate;
    const classes = ['calendar-day', cell.inMonth ? '' : 'outside', selected ? 'selected' : '', inRange ? 'in-range' : ''].filter(Boolean).join(' ');
    const title = state?.title || '暂无处理状态';
    return `<button class="${classes}" type="button" role="gridcell" data-date="${cell.key}" data-state="${state?.kind || ''}" title="${escapeHTML(title)}" aria-label="${cell.key}，${escapeHTML(title)}">${cell.day}</button>`;
  }).join('');
}

function setDatePickerOpen(open, target = calendarTarget) {
  calendarTarget = target;
  $('date-picker').hidden = !open;
  $('date-sync-toggle').setAttribute('aria-expanded', String(open));
  if (!open) return;
  setCalendarMonth(selectedDate(target) || dateAvailability.latestReady || new Date().toISOString().slice(0, 10));
  renderCalendar();
}

async function loadDateAvailability() {
  if (!pluginEnabled || !token) return;
  setDateSyncState('loading', '正在同步可查询日期');
  try {
    const response = await fetch(`${await apiBase()}/api/ingest-progress?range=all&include_ready=true`, { headers: { Authorization: `Bearer ${token}` } });
    if (!response.ok) throw new Error(`HTTP ${response.status}`);
    dateAvailability = buildDateAvailability(await response.json());
    if (!dateAvailability.latestReady) {
      setDateSyncState('ready', '暂无已完成的可查询日期');
      renderCalendar();
      return;
    }
    setDateSyncState('ready', `可查询 ${dateAvailability.firstReady} 至 ${dateAvailability.latestReady}`);
    if (!dateInputsEdited && !queryState.values) {
      $('start').value = `${dateAvailability.latestReady} 00:00`;
      $('end').value = `${dateAvailability.latestReady} 23:59`;
    }
    if ($('date-picker').hidden) setCalendarMonth(dateAvailability.latestReady);
    renderCalendar();
  } catch {
    setDateSyncState('failed', '可查询日期同步失败，仍可手动输入');
  }
}

function formatPercent(value) {
  const number = Number(value);
  return Number.isFinite(number) ? `${number.toFixed(1)}%` : '--';
}

function setPercentMetric(valueId, barId, value) {
  const number = Number(value);
  $(valueId).textContent = formatPercent(value);
  $(barId).style.width = Number.isFinite(number) ? `${Math.min(100, Math.max(0, number))}%` : '0';
}

function formatBytes(value) {
  let number = Number(value);
  if (!Number.isFinite(number) || number < 0) return '--';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let unit = 0;
  while (number >= 1024 && unit < units.length - 1) {
    number /= 1024;
    unit += 1;
  }
  return `${number >= 10 || unit === 0 ? number.toFixed(0) : number.toFixed(1)} ${units[unit]}`;
}

function formatCount(value) {
  const number = Number(value);
  return Number.isFinite(number) ? number.toLocaleString('zh-CN') : '0';
}

function formatReceivedAt(value) {
  const date = new Date(value);
  if (!value || Number.isNaN(date.getTime())) return '暂无日志';
  const now = new Date();
  const sameDay = date.getFullYear() === now.getFullYear() && date.getMonth() === now.getMonth() && date.getDate() === now.getDate();
  const time = date.toLocaleTimeString('zh-CN', { hour12: false });
  return sameDay ? `最近 ${time}` : `最近 ${date.getMonth() + 1}-${date.getDate()} ${time}`;
}

function setReceiverHealth(state, title, services, detail, last = '--') {
  $('receiver-health').dataset.state = state;
  $('receiver-title').textContent = title;
  $('receiver-services').textContent = services;
  $('receiver-detail').textContent = detail;
  $('receiver-last').textContent = last;
}

function renderReceiverHealth(payload) {
  const status = summarizeReceiverStatus(payload);
  const count = formatCount(status.receivedMessages);
  const last = formatReceivedAt(status.lastReceivedAt);
  if (status.state === 'unconfigured') {
    setReceiverHealth('unconfigured', '未配置实时日志接收', '0/0 服务运行', '没有启用的日志接收来源', '未配置');
    return;
  }
  if (status.state === 'failed') {
    setReceiverHealth('failed', `${status.failed} 个接收服务异常`, `${status.running}/${status.configured} 服务运行`, `本次接收 ${count} 条`, last);
    return;
  }
  if (status.state === 'waiting') {
    setReceiverHealth('waiting', '接收服务已启动', `${status.running}/${status.configured} 服务运行`, '暂未收到日志', last);
    return;
  }
  setReceiverHealth('active', '日志接收正常', `${status.running}/${status.configured} 服务运行`, `本次接收 ${count} 条`, last);
}

async function loadSystemMetrics() {
  $('metric-cpu').textContent = '--';
  $('metric-memory').textContent = '--';
  $('metric-database').textContent = '--';
  $('metric-disk-available').textContent = '--';
  $('metric-cpu-bar').style.width = '0';
  $('metric-memory-bar').style.width = '0';
  $('metric-disk-bar').style.width = '0';
  setReceiverHealth('checking', '正在读取接收状态', '--', '--');
  const base = await apiBase();
  $('current-api').textContent = base;
  if (!pluginEnabled || !token) return;
  try {
    const headers = { Authorization: `Bearer ${token}` };
    const [healthResponse, receiverResponse] = await Promise.all([
      fetch(`${base}/api/health-dashboard/summary?range=all`, { headers }),
      fetch(`${base}/api/receiver/status`, { headers }),
    ]);
    if (healthResponse.status === 401 || receiverResponse.status === 401) {
      token = '';
      await clearStoredToken();
      show('query', false);
      show('login', true);
      resetLoginCard();
      $('login-error').textContent = '登录已过期，请重新登录';
      return;
    }
    if (!healthResponse.ok) throw new Error(`主机状态读取失败（HTTP ${healthResponse.status}）`);
    const data = await healthResponse.json();
    setPercentMetric('metric-cpu', 'metric-cpu-bar', data.system_health?.cpu?.load_percent);
    setPercentMetric('metric-memory', 'metric-memory-bar', data.system_health?.memory?.used_percent);
    const database = data.system_health?.database || {};
    $('metric-database').textContent = formatBytes(database.disk_used_bytes ?? data.data_health?.clickhouse_disk_used_bytes);
    $('metric-disk-available').textContent = database.disk_available_bytes == null ? '--' : formatBytes(database.disk_available_bytes);
    const diskPercent = Number.isFinite(Number(database.disk_available_percent))
      ? Number(database.disk_available_percent)
      : availablePercent(database.disk_total_bytes, database.disk_available_bytes);
    $('metric-disk-bar').style.width = diskPercent == null ? '0' : `${Math.min(100, Math.max(0, diskPercent))}%`;
    if (receiverResponse.ok) renderReceiverHealth(await receiverResponse.json());
    else setReceiverHealth('failed', '接收状态读取失败', '--', `HTTP ${receiverResponse.status}`, '--');
  } catch {
    setConnectionStatus('failed', '连接失败');
    setReceiverHealth('failed', '状态读取失败', '--', '无法连接 FWLOG 查询服务', '--');
  }
}
function queryParams(values, page, cursor) {
  const params = new URLSearchParams(); Object.entries(values).forEach(([key, value]) => { if (value !== '') params.set(key, value); }); params.set('page', page); params.set('page_size', values.page_size); if (cursor) params.set('cursor', cursor); return params;
}
function collectValues() {
  const start = parseDateTimeInput($('start').value);
  const end = parseDateTimeInput($('end').value);
  if (!start) throw new Error('开始时间格式应为 YYYY-MM-DD HH:mm');
  if (!end) throw new Error('结束时间格式应为 YYYY-MM-DD HH:mm');
  if (start > end) throw new Error('结束时间不能早于开始时间');
  $('start').value = toInputDateTime(start);
  $('end').value = toInputDateTime(end);
  return { start, end, ip: $('ip').value.trim(), page_size: '50' };
}
async function runQuery(page = 1, cursor = '') {
  if (!pluginEnabled) throw new Error('插件已停用');
  const values = queryState.values || collectValues(); $('query-error').textContent = ''; $('summary').textContent = '查询中…';
  let response;
  try {
    response = await fetch(`${await apiBase()}/api/query?${queryParams(values, page, cursor)}`, { headers: { Authorization: `Bearer ${token}` } });
  } catch (error) {
    setConnectionStatus('failed', '连接失败');
    throw error;
  }
  if (response.status === 401) {
    token = '';
    await clearStoredToken();
    show('query', false);
    show('login', true);
    resetLoginCard();
    $('login-error').textContent = '登录已过期，请重新登录';
  }
  const data = await response.json().catch(() => ({})); if (!response.ok) throw new Error(data.message || '查询失败');
  setConnectionStatus('connected', '已连接');
  queryState = { values, page, cursor, cursorStack: queryState.cursorStack, }; render(data); if (data.next_cursor) queryState.cursorStack[page] = data.next_cursor;
}
function render(data) {
  $('summary').textContent = `共 ${Number(data.total || 0).toLocaleString('zh-CN')} 条`;
  $('query-time').textContent = `${data.query_time_ms || 0} ms`;
  $('page-number').textContent = `第 ${data.page || queryState.page || 1} 页`;
  $('body').innerHTML = (data.records || []).map(renderLogRecord).join('') || '<div class="empty-result">没有匹配结果</div>';
  $('previous').disabled = queryState.page <= 1; $('next').disabled = !data.has_more;
}
function escapeHTML(value) { return String(value ?? '-').replace(/[&<>"']/g, (char) => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[char])); }

function compactTimestamp(value) {
  const text = String(value || '');
  const match = text.match(/\d{4}-(\d{2})-(\d{2})[ T](\d{2}):(\d{2})/);
  return match ? `${match[1]}-${match[2]} ${match[3]}:${match[4]}` : text.slice(0, 16) || '-';
}

function protocolLabel(value) {
  const protocol = String(value ?? '-').toUpperCase();
  if (protocol === '6') return 'TCP';
  if (protocol === '17') return 'UDP';
  return protocol;
}

function endpoint(ip, port, highlighted = false) {
  const value = String(ip || '-');
  const address = highlighted ? `<mark>${escapeHTML(value)}</mark>` : escapeHTML(value);
  return `${address}${port === '' || port == null ? '' : `<span class="port">:${escapeHTML(port)}</span>`}`;
}

function renderLogRecord(row) {
  const matchIP = queryState.values?.ip || '';
  const active = /^(create|accept|allow)$/i.test(String(row.action || ''));
  return `<article class="log-item">
    <button class="log-summary" type="button" aria-expanded="false">
      <span class="flow-line"><i${active ? '' : ' class="muted-dot"'}></i><b>${endpoint(row.src_ip, row.src_port, row.src_ip === matchIP)}<span class="route-arrow">→</span>${endpoint(row.dst_ip, row.dst_port, row.dst_ip === matchIP)}</b><em>${escapeHTML(protocolLabel(row.protocol))}</em></span>
      <span class="meta-line"><span><time>${escapeHTML(compactTimestamp(row.timestamp))}</time><span class="separator">·</span>NAT <code>${endpoint(row.nat_ip, row.nat_port, row.nat_ip === matchIP)}</code><span class="separator">·</span>${escapeHTML(row.source_id || '未标识来源')}</span><svg viewBox="0 0 16 16" aria-hidden="true"><path d="m4 6 4 4 4-4"/></svg></span>
    </button>
    <div class="log-details"><div><span>完整时间</span><b>${escapeHTML(row.timestamp)}</b><span>动作</span><b>${escapeHTML(row.action)}</b><span>日志标识</span><b>${escapeHTML(row.log_tag)}</b><span>来源 ID</span><b>${escapeHTML(row.source_id)}</b></div></div>
  </article>`;
}

$('body').addEventListener('click', (event) => {
  const summary = event.target.closest('.log-summary');
  if (!summary) return;
  summary.setAttribute('aria-expanded', String(summary.getAttribute('aria-expanded') !== 'true'));
});
document.querySelectorAll('.date-trigger').forEach((button) => {
  button.addEventListener('click', () => {
    setDatePickerOpen(true, button.dataset.target);
    void loadDateAvailability();
  });
});
$('date-sync-toggle').addEventListener('click', () => {
  const open = $('date-picker').hidden;
  setDatePickerOpen(open, calendarTarget);
  if (open) void loadDateAvailability();
});
$('calendar-close').addEventListener('click', () => setDatePickerOpen(false));
$('calendar-previous').addEventListener('click', () => {
  calendarMonth = new Date(calendarMonth.getFullYear(), calendarMonth.getMonth() - 1, 1);
  renderCalendar();
});
$('calendar-next').addEventListener('click', () => {
  calendarMonth = new Date(calendarMonth.getFullYear(), calendarMonth.getMonth() + 1, 1);
  renderCalendar();
});
$('calendar-grid').addEventListener('click', (event) => {
  const button = event.target.closest('[data-date]');
  if (!button) return;
  const input = $(calendarTarget);
  const current = parseDateTimeInput(input.value);
  const time = current ? current.slice(11, 16) : calendarTarget === 'start' ? '00:00' : '23:59';
  input.value = `${button.dataset.date} ${time}`;
  dateInputsEdited = true;
  if (calendarTarget === 'start') {
    calendarTarget = 'end';
    renderCalendar();
    return;
  }
  setDatePickerOpen(false);
});
['start', 'end'].forEach((id) => {
  $(id).addEventListener('input', () => {
    dateInputsEdited = true;
    $('query-error').textContent = '';
  });
  $(id).addEventListener('change', () => {
    const normalized = toInputDateTime($(id).value);
    if (normalized) $(id).value = normalized;
  });
});
document.addEventListener('keydown', (event) => {
  if (event.key === 'Escape' && !$('date-picker').hidden) setDatePickerOpen(false);
});
$('query-form').addEventListener('submit', async (event) => { event.preventDefault(); try { queryState = { page:1, cursor:'', cursorStack:[''], values:collectValues() }; await runQuery(); } catch (error) { errorText('query-error', error); $('summary').textContent = '查询失败'; } });
$('previous').addEventListener('click', async () => { try { await runQuery(queryState.page - 1, queryState.cursorStack[queryState.page - 1] || ''); } catch (error) { errorText('query-error', error); } });
$('next').addEventListener('click', async () => { try { await runQuery(queryState.page + 1, queryState.cursorStack[queryState.page] || ''); } catch (error) { errorText('query-error', error); } });
$('toggle-password').addEventListener('click', () => {
  const visible = $('password').type === 'text';
  $('password').type = visible ? 'password' : 'text';
  $('toggle-password').querySelector('.eye-open').hidden = !visible;
  $('toggle-password').querySelector('.eye-closed').hidden = visible;
  $('toggle-password').setAttribute('aria-label', visible ? '显示密码' : '隐藏密码');
  $('toggle-password').title = visible ? '显示密码' : '隐藏密码';
  $('password').focus();
});

$('logout').addEventListener('click', async () => {
  token = '';
  await clearStoredToken();
  show('query', false);
  show('login', true);
  resetLoginCard();
  $('password').focus();
});

document.querySelectorAll('.workspace-tabs button').forEach((button) => {
  button.addEventListener('click', () => setWorkspacePage(button.dataset.page));
});

function closeSettings() {
  show('settings-panel', false);
  show('query', settingsReturnView === 'query');
  show('login', settingsReturnView === 'login');
}

async function openSettings() {
  if (!pluginEnabled) return;
  if (!$('date-picker').hidden) setDatePickerOpen(false);
  settingsReturnView = $('query').hidden ? 'login' : 'query';
  $('api-url').value = await apiBase();
  $('settings-status').textContent = '';
  show('login', false);
  show('query', false);
  show('settings-panel', true);
  $('api-url').focus();
}

$('settings').addEventListener('click', openSettings);
$('edit-api').addEventListener('click', openSettings);
$('cancel-settings').addEventListener('click', closeSettings);
$('save-settings').addEventListener('click', async () => {
  const value = $('api-url').value.trim().replace(/\/$/, '');
  try {
    const parsed = new URL(value);
    if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') throw new Error();
  } catch {
    $('settings-status').textContent = '请输入有效的 HTTP 或 HTTPS 地址';
    return;
  }
  await saveApiBase(value);
  closeSettings();
  void probeConnection();
  void loadSystemMetrics();
  void loadDateAvailability();
});

$('plugin-toggle').addEventListener('click', async () => {
  const enabled = !pluginEnabled;
  applyPluginEnabled(enabled);
  await savePluginEnabled(enabled);
  if (enabled) {
    void probeConnection();
    void loadSystemMetrics();
    void loadDateAvailability();
  }
});

async function restoreSession() {
  resetLoginCard();
  const savedToken = await readStoredToken();
  token = savedToken;
  applyPluginEnabled(await readPluginEnabled());
  if (pluginEnabled && savedToken) await activateQuery(savedToken, false);
  void probeConnection();
}

void restoreSession();
