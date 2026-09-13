import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import test from 'node:test';

const pagePath = path.resolve('src/pages/SystemMaintenancePage.tsx');
const stylesPath = path.resolve('src/styles.css');

test('system settings tabs expose clear task names', () => {
  const page = fs.readFileSync(pagePath, 'utf8');
  const styles = fs.readFileSync(stylesPath, 'utf8');
  const api = fs.readFileSync(path.resolve('src/api.ts'), 'utf8');

  for (const required of [
    "key: 'source'",
    "日志来源",
    "key: 'ip'",
    "网段名称",
    "key: 'ingest'",
    "扫描计划",
    "数据处理",
    "key: 'upgrade'",
    "程序升级",
    "key: 'threat-intelligence'",
    "威胁情报",
    "key: 'interfaces'",
    "接口服务",
    "key: 'security'",
    "账号安全",
  ]) {
    assert.match(page, new RegExp(required), `系统设置缺少页签文案：${required}`);
  }
  assert.doesNotMatch(page, /<span className="eyebrow">/);
  assert.match(page, /IP 映射文件/);
  assert.match(page, /custom_ip_map_path/);
  assert.match(page, /geoip_db_path/);
  assert.match(api, /custom_ip_map_path: '\/opt\/fwlog\/custom_ip_map\.csv'/);
  assert.match(api, /geoip_db_path: '\/opt\/fwlog\/GeoLite2-City\.mmdb'/);
  assert.match(page, /显示名称/);
  assert.match(page, /serial_number\?: string;/);
  assert.match(page, /label="设备序列号"/);
  assert.match(page, /source\.serial_number \|\| '-'/);
  assert.match(page, /maintenance-plan-card maintenance-plan-card--schedule/);
  assert.match(page, /maintenance-run-card maintenance-run-card--manual/);
  assert.match(page, /maintenance-run-card maintenance-run-card--upgrade/);
  assert.match(page, /API Key 或 Token/);
  assert.match(page, /ThreatIntelligenceSettingsPanel/);
  assert.equal((page.match(/<section className="ops-section maintenance-card maintenance-panel(?: account-security-panel)?">/g) || []).length, 8);
  assert.equal((page.match(/className="maintenance-panel-note"/g) || []).length, 8);
  assert.equal((page.match(/className="maintenance-panel-stack"/g) || []).length, 1);
  assert.doesNotMatch(page, /maintenance-panel-head/);
  assert.match(page, /className="setting-grid setting-grid--single"/);
  assert.doesNotMatch(page, /当前映射数据|地理位置数据 \+ 自定义映射/);
  assert.match(styles, /\.maintenance-panel\s*\{[^}]*display:\s*grid;[^}]*gap:\s*16px;/s);
  assert.match(styles, /\.maintenance-panel-stack\s*\{[^}]*display:\s*grid;[^}]*gap:\s*14px;/s);
  assert.match(styles, /\.setting-grid--single\s*\{[^}]*grid-template-columns:\s*minmax\(0,\s*1fr\);/s);
  assert.match(styles, /\.maintenance-panel\s*>\s*\.source-list-editor\s*\{[^}]*padding:\s*14px;[^}]*border:/s);
  assert.match(styles, /\.maintenance-panel\s*>\s*\.setting-grid\s*>\s*\.setting-fields\s*\{[^}]*padding:\s*14px;[^}]*border:/s);
  assert.match(styles, /\.page-stack\s*>\s*\.ant-form\s*\{[^}]*max-width:\s*100%;[^}]*overflow:\s*hidden;/s);
  assert.match(styles, /\.maintenance-card-summary,\s*\.maintenance-action-summary\s*\{/);
  assert.match(styles, /@media \(max-width: 1200px\) and \(min-width: 901px\)[\s\S]*\.maintenance-plan-grid,[\s\S]*grid-template-columns: repeat\(2, minmax\(0, 1fr\)\)/);
  assert.match(styles, /\.ant-pro-layout-container,[\s\S]*min-width: 0;[\s\S]*max-width: 100%/);
});

test('日志接收和日志整理归入日志管理大类', () => {
  const page = fs.readFileSync(pagePath, 'utf8');
  const layout = fs.readFileSync(path.resolve('src/layout/AppLayout.tsx'), 'utf8');
  const main = fs.readFileSync(path.resolve('src/main.tsx'), 'utf8');
  const styles = fs.readFileSync(stylesPath, 'utf8');
  assert.match(layout, /path: '\/progress-center',\s*name: '日志管理'[\s\S]*routes:/);
  assert.match(layout, /path: '\/progress-reception',\s*name: '日志接收'/);
  assert.match(layout, /path: '\/progress\/organize', name: '日志整理'/);
  assert.doesNotMatch(layout, /path: '\/progress-reception'[\s\S]*routes:/);
  assert.doesNotMatch(layout, /path: '\/progress\/organize', name: '日志整理'[\s\S]*routes:/);
  assert.match(layout, /location=\{\{ pathname: routePathMap\[active\] \}\}/);
  assert.match(main, /active === 'reception'[\s\S]*<LogReceptionPage/);
  assert.match(main, /active === 'organize'[\s\S]*<SystemMaintenancePage scope="organize"/);
  assert.doesNotMatch(main, /receptionProgress|receptionSources|organizeRebuild|organizeSchedule|organizeAliases/);
  assert.doesNotMatch(main, /active === '(?:progress|schedule|sources|cidr|ingest)'/);
  const progressPage = fs.readFileSync(path.resolve('src/pages/IncrementalProgressPage.tsx'), 'utf8');
  assert.doesNotMatch(progressPage, /<span className="eyebrow">/);
  assert.match(progressPage, /type IncrementalProgressPageProps = \{[\s\S]*embedded\?: boolean/);
  const receptionPage = fs.readFileSync(path.resolve('src/pages/LogReceptionPage.tsx'), 'utf8');
  assert.match(receptionPage, /<IncrementalProgressPage embedded \/>/);
  assert.match(main, /scope="system"/);
  assert.match(page, /maintenance-tabs--standalone/);
  assert.match(page, /tabBarStyle=\{scope === 'schedule' \|\| scope === 'sources' \? \{ display: 'none' \} : undefined\}/);
  assert.match(page, /scope === 'schedule' \? '扫描计划' : scope === 'sources' \? '日志来源' : scope === 'organize' \? '日志整理' : '系统设置'/);
  assert.match(page, /scope === 'schedule'\) return item\.key === 'schedule'/);
  assert.match(page, /scope === 'sources'\) return item\.key === 'source'/);
  assert.match(page, /scope === 'organize'[\s\S]*item\.key === 'ingest'[\s\S]*item\.key === 'schedule'[\s\S]*item\.key === 'ip'/);
  assert.match(page, /tabLabel\(<ClockCircleOutlined \/>, '扫描计划'\)/);
  assert.match(styles, /\.progress-page-toolbar\s*\{[^}]*margin-bottom:\s*0;/s);
  assert.match(styles, /\.workbench\s*\{[\s\S]*min-width:\s*0;[\s\S]*overflow-x:\s*hidden;/);
  assert.match(styles, /\.ant-menu-submenu-popup,\s*\.ant-menu-submenu-popup \.ant-menu\s*\{[\s\S]*width:\s*max-content !important;[\s\S]*min-width:\s*0 !important;/);
  assert.match(styles, /\.ant-menu-submenu-popup \.ant-menu-submenu-title,[\s\S]*width:\s*auto !important;[\s\S]*white-space:\s*nowrap/);
  assert.match(layout, /collapsedButtonRender=\{false\}/);
  assert.match(layout, /siderWidth=\{64\}/);
  assert.match(layout, /collapsed\n/);
});

test('API 与 MCP 合并到系统设置并提供持久化开关', () => {
  const page = fs.readFileSync(pagePath, 'utf8');
  const layout = fs.readFileSync(path.resolve('src/layout/AppLayout.tsx'), 'utf8');
  const main = fs.readFileSync(path.resolve('src/main.tsx'), 'utf8');

  assert.match(page, /name="external_api_enabled"[\s\S]*valuePropName="checked"/);
  assert.match(page, /name="mcp_enabled"[\s\S]*valuePropName="checked"/);
  assert.match(page, /日志查询 API/);
  assert.match(page, /MCP 接口/);
  assert.match(page, /\/api\/query/);
  assert.match(page, /\/api\/mcp/);
  assert.match(page, /interface-download-button/);
  assert.match(page, /fwlog-query-extension\.zip/);
  assert.match(page, /fwlog-mcp-config\.json/);
  assert.match(page, /item\.key === 'interfaces'/);
  assert.doesNotMatch(layout, /path: '\/api'/);
  assert.doesNotMatch(layout, /\| 'api'/);
  assert.doesNotMatch(main, /ApiInterfacePage/);
  assert.doesNotMatch(main, /active === 'api'/);
});

test('接口开关在本地预览中保存后保持当前状态', () => {
  const api = fs.readFileSync(path.resolve('src/api.ts'), 'utf8');

  assert.match(api, /let mockExternalAPIEnabled = true;/);
  assert.match(api, /let mockMCPEnabled = false;/);
  assert.match(api, /mockExternalAPIEnabled = Boolean\(payload\.external_api_enabled\)/);
  assert.match(api, /mockMCPEnabled = Boolean\(payload\.mcp_enabled\)/);
  assert.match(api, /external_api_enabled: String\(mockExternalAPIEnabled\)/);
  assert.match(api, /mcp_enabled: String\(mockMCPEnabled\)/);
});

test('主要业务页面不再显示标题上方的重复描述', () => {
  for (const relativePath of [
    'src/pages/HealthDashboard.tsx',
    'src/pages/LogSearchPage.tsx',
    'src/pages/IncrementalProgressPage.tsx',
    'src/pages/SystemMaintenancePage.tsx',
  ]) {
    const source = fs.readFileSync(path.resolve(relativePath), 'utf8');
    assert.doesNotMatch(source, /className="eyebrow"/, `${relativePath} 仍包含重复标题描述`);
  }
});

test('账号安全使用紧凑双栏并将会话操作独立到底部', () => {
  const page = fs.readFileSync(pagePath, 'utf8');
  const styles = fs.readFileSync(stylesPath, 'utf8');

  assert.match(page, /className="account-security-layout"/);
  assert.match(page, /className="account-security-section account-password-section"/);
  assert.match(page, /className="account-security-section account-mfa-section"/);
  assert.match(page, /className="account-session-bar"/);
  assert.match(page, /修改登录密码/);
  assert.match(page, /当前登录状态将失效/);
  assert.match(page, /systemSection !== 'security'/);
  assert.doesNotMatch(page, /setting-summary setting-summary-amber/);
  assert.match(styles, /\.account-security-panel\s*\{[^}]*width:\s*100%;[^}]*max-width:\s*none;/s);
  assert.match(styles, /\.account-security-layout\s*\{[^}]*grid-template-columns:\s*minmax\(0,\s*1fr\) minmax\(360px,\s*0\.9fr\);/s);
  assert.match(styles, /@media \(max-width: 900px\)[\s\S]*?\.account-security-layout\s*\{[^}]*grid-template-columns:\s*1fr;/s);
});

test('maintenance page uses unified Chinese copy only', () => {
  const page = fs.readFileSync(pagePath, 'utf8');

  assert.doesNotMatch(page, />Source</);
  assert.doesNotMatch(page, /All enabled sources/);
  assert.doesNotMatch(page, />Manual</);
  assert.doesNotMatch(page, /Auto upgrade/);
  assert.doesNotMatch(page, /自动升级/);

  for (const required of ['日志来源', '全部已启用日志来源', '网段名称', 'IP 网段', '显示名称', '日期范围', '数据处理', '新增日志处理', '历史数据重建', '开始处理新增日志', '开始重建历史数据', '程序升级', '程序更新', '本地升级包', '选择升级包', '上传并安装']) {
    assert.match(page, new RegExp(required), `维护页缺少统一文案：${required}`);
  }
});

test('an explicitly empty log source list stays empty', () => {
  const page = fs.readFileSync(pagePath, 'utf8');
  assert.match(page, /const legacyLogSources = settings\.log_sources === undefined/);
  assert.match(page, /parsedLogSources\.length \? parsedLogSources : legacyLogSources/);
  assert.doesNotMatch(page, /normalizeLogSourcesForForm\(parsedLogSources\.length \? parsedLogSources : \[/);
});

test('auto scan time picker is owned by form field instead of a string controlled value', () => {
  const page = fs.readFileSync(pagePath, 'utf8');

  assert.match(page, /<Form\.Item name="auto_scan_times" noStyle>/);
  assert.doesNotMatch(page, /value=\{autoScanTimeValue\(autoScanTime\)\}/);
  assert.doesNotMatch(page, /setFieldValue\('auto_scan_times'/);
  assert.match(page, /auto_scan_times: autoScanTimeValue\(settings\.auto_scan_times\)/);
  assert.match(page, /auto_scan_times: formatAutoScanTime\(values\.auto_scan_times\)/);
});

test('maintenance ingest action uses source date range and action type', () => {
  const page = fs.readFileSync(pagePath, 'utf8');
  const styles = fs.readFileSync(stylesPath, 'utf8');

  assert.doesNotMatch(page, /const \[fullRebuild, setFullRebuild\]/);
  assert.doesNotMatch(page, /<Switch checked=\{fullRebuild\}/);
  assert.match(page, /type IngestAction = 'sync' \| 'rebuild';/);
  assert.match(page, /type IngestDateMode = 'all' \| 'single' \| 'range';/);
  assert.match(page, /const \[ingestAction, setIngestAction\]/);
  assert.match(page, /const \[dateMode, setDateMode\]/);
  assert.match(page, /DatePicker\.RangePicker/);
  assert.match(page, /所有历史日期/);
  assert.match(page, /日期范围/);
  assert.match(page, /开始处理新增日志/);
  assert.match(page, /开始重建历史数据/);
  assert.match(page, /本次处理：来源 =/);
  assert.match(styles, /\.maintenance-action-summary\s*\{/);
});

test('maintenance cards share compact fields and summary copy', () => {
  const page = fs.readFileSync(pagePath, 'utf8');
  const styles = fs.readFileSync(stylesPath, 'utf8');

  assert.match(page, /autoScanSummary/);
  assert.match(page, /upgradeSummary/);
  assert.match(page, /自动处理：/);
  assert.match(page, /只处理尚未完成入库的日期/);
  assert.match(page, /版本状态：/);
  assert.match(page, /检查更新/);
  assert.match(page, /本地升级包/);
  assert.doesNotMatch(page, /const INGEST_BUTTON_LABELS/);
  assert.doesNotMatch(page, /入库全部日志源的所有历史日志/);
  assert.doesNotMatch(page, /全量重建当前日志源的所选日期范围/);
  assert.match(styles, /\.maintenance-card-summary,\s*\.maintenance-action-summary\s*\{/);
  assert.match(styles, /\.maintenance-upgrade-grid\s*\{/);
  assert.match(styles, /\.maintenance-plan-grid\s*\{[^}]*grid-template-columns:[^;]+repeat\(4,/s);
});

test('log source management distinguishes directory and rsyslog receivers with selectable transport', () => {
  const page = fs.readFileSync(pagePath, 'utf8');
  const styles = fs.readFileSync(stylesPath, 'utf8');

  for (const required of ['添加文件目录源', '添加 RSyslog 接收源', '文件目录源', 'RSyslog 接收源', '接收协议', '监听端口', '接收文件保存目录', '允许的发送端地址（可选）', '压缩文件保存目录（可选）'] ) {
    assert.match(page, new RegExp(required), `日志源编辑器缺少文案：${required}`);
  }
  assert.match(page, /\{ value: 'udp', label: 'UDP' \}/);
  assert.match(page, /\{ value: 'tcp', label: 'TCP' \}/);
  assert.match(page, /className="source-management-list"/);
  assert.match(page, /<Modal/);
  assert.doesNotMatch(page, /<Form\.List name="log_sources">/);
  assert.match(styles, /\.source-management-list\s*\{/);
  assert.match(styles, /\.source-editor-grid\s*\{/);
  assert.doesNotMatch(page, /label="设备 ID"/);
  assert.doesNotMatch(page, /label="客户端 IP \/ 网段"/);
  assert.doesNotMatch(page, /兼容全匹配/);
  assert.doesNotMatch(page, /label="落盘目录"/);
});
