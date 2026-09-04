import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import test from 'node:test';

const pagePath = path.resolve('src/pages/SystemMaintenancePage.tsx');
const stylesPath = path.resolve('src/styles.css');

test('system settings tabs expose clear task names', () => {
  const page = fs.readFileSync(pagePath, 'utf8');
  const styles = fs.readFileSync(stylesPath, 'utf8');

  for (const required of [
    "key: 'source'",
    "日志来源",
    "key: 'ip'",
    "CIDR 别名",
    "key: 'ingest'",
    "日志入库",
    "key: 'upgrade'",
    "程序升级",
    "key: 'threat-intelligence'",
    "威胁情报",
    "key: 'security'",
    "账号安全",
  ]) {
    assert.match(page, new RegExp(required), `系统设置缺少页签文案：${required}`);
  }
  assert.match(page, /系统管理/);
  assert.match(page, /IP 映射文件/);
  assert.match(page, /显示名称/);
  assert.match(page, /serial_number\?: string;/);
  assert.match(page, /label="设备序列号"/);
  assert.match(page, /source\.serial_number \|\| '-'/);
  assert.match(page, /maintenance-plan-card maintenance-plan-card--schedule/);
  assert.match(page, /maintenance-run-card maintenance-run-card--manual/);
  assert.match(page, /maintenance-run-card maintenance-run-card--upgrade/);
  assert.match(page, /API Key 或 Token/);
  assert.match(page, /ThreatIntelligenceSettingsPanel/);
  assert.equal((page.match(/<section className="ops-section maintenance-card maintenance-panel">/g) || []).length, 6);
  assert.equal((page.match(/className="maintenance-panel-note"/g) || []).length, 6);
  assert.equal((page.match(/className="maintenance-panel-stack"/g) || []).length, 2);
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
});

test('maintenance page uses unified Chinese copy only', () => {
  const page = fs.readFileSync(pagePath, 'utf8');

  assert.doesNotMatch(page, />Source</);
  assert.doesNotMatch(page, /All enabled sources/);
  assert.doesNotMatch(page, />Manual</);
  assert.doesNotMatch(page, /Auto upgrade/);
  assert.doesNotMatch(page, /自动升级/);

  for (const required of ['日志来源', '全部已启用日志来源', '日期范围', '导入新增日志', '重新导入所选日期', '开始处理', '程序升级', '程序更新', '本地升级包', '选择升级包', '上传并安装']) {
    assert.match(page, new RegExp(required), `维护页缺少统一文案：${required}`);
  }
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
  assert.match(page, /开始导入/);
  assert.match(page, /开始重新导入/);
  assert.match(page, /本次处理：日志来源 =/);
  assert.match(styles, /\.maintenance-action-summary\s*\{/);
});

test('maintenance cards share compact fields and summary copy', () => {
  const page = fs.readFileSync(pagePath, 'utf8');
  const styles = fs.readFileSync(stylesPath, 'utf8');

  assert.match(page, /autoScanSummary/);
  assert.match(page, /upgradeSummary/);
  assert.match(page, /自动扫描：/);
  assert.match(page, /只导入尚未完成的日期/);
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
