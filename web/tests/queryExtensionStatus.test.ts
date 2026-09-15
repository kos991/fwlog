import assert from 'node:assert/strict';
import { createRequire } from 'node:module';
import fs from 'node:fs';
import path from 'node:path';
import test from 'node:test';

const require = createRequire(import.meta.url);
const { summarizeReceiverStatus, availablePercent } = require('../../chrome-extension/status-metrics.js');
const { buildDateAvailability, monthCells, parseDateTimeInput } = require('../../chrome-extension/date-availability.js');

test('query extension uses the Manifest V3 action and service worker model', () => {
  const manifest = JSON.parse(fs.readFileSync(path.resolve('../chrome-extension/manifest.json'), 'utf8')) as {
    manifest_version?: number;
    action?: { default_popup?: string };
    background?: { service_worker?: string };
    permissions?: string[];
    content_security_policy?: { extension_pages?: string };
    browser_action?: unknown;
    background_page?: unknown;
  };

  assert.equal(manifest.manifest_version, 3);
  assert.equal(manifest.action?.default_popup, 'popup.html');
  assert.equal(manifest.background?.service_worker, 'background.js');
  assert.deepEqual(manifest.permissions, ['storage']);
  assert.equal(manifest.content_security_policy?.extension_pages, "script-src 'self'; object-src 'self'");
  assert.equal(manifest.browser_action, undefined);
  assert.equal(manifest.background_page, undefined);
});

test('query extension summarizes all receiver sources without treating lifetime counts as daily counts', () => {
  const status = summarizeReceiverStatus({
    edge: {
      running: true,
      error: '',
      last_received_at: '2026-09-10T12:00:00+08:00',
      received_messages: 12,
    },
    core: {
      running: false,
      error: 'listen failed',
      last_received_at: '2026-09-10T12:05:00+08:00',
      received_messages: 8,
    },
  });

  assert.deepEqual(status, {
    state: 'failed',
    configured: 2,
    running: 1,
    failed: 1,
    receivedMessages: 20,
    lastReceivedAt: '2026-09-10T12:05:00+08:00',
  });
});

test('query extension distinguishes unconfigured receivers from running receivers without messages', () => {
  assert.equal(summarizeReceiverStatus({}).state, 'unconfigured');
  assert.equal(summarizeReceiverStatus({ edge: { running: true } }).state, 'waiting');
});

test('query extension calculates available disk percentage from capacity bytes', () => {
  assert.equal(availablePercent(400, 100), 25);
  assert.equal(availablePercent(0, 0), null);
});

test('query extension accepts manual date time formats and rejects invalid values', () => {
  assert.equal(parseDateTimeInput('2026-09-10 14:30'), '2026-09-10 14:30:00');
  assert.equal(parseDateTimeInput('2026/9/10 4:05:06'), '2026-09-10 04:05:06');
  assert.equal(parseDateTimeInput('202609101430'), '2026-09-10 14:30:00');
  assert.equal(parseDateTimeInput('2026-02-30 14:30'), '');
});

test('query extension calendar follows the main search page date status priority', () => {
  const availability = buildDateAvailability({
    dates: [
      { log_date: '2026-09-09', status: 'ready', rows_imported: 128 },
      { log_date: '2026-09-10', status: 'ready', rows_imported: 256 },
      { log_date: '2026-09-10', status: 'importing', progress_pct: 50 },
      { log_date: '2026-09-11', status: 'failed', error: '文件读取失败' },
    ],
  });

  assert.equal(availability.firstReady, '2026-09-09');
  assert.equal(availability.latestReady, '2026-09-10');
  assert.deepEqual(availability.dates['2026-09-10'], { kind: 'importing', title: '正在入库，50%' });
  assert.equal(availability.dates['2026-09-11'].kind, 'failed');
  assert.equal(monthCells(2026, 9).length, 42);
});

test('query extension uses the approved compact result and telemetry layout', () => {
  const html = fs.readFileSync(path.resolve('../chrome-extension/popup.html'), 'utf8');
  const script = fs.readFileSync(path.resolve('../chrome-extension/popup.js'), 'utf8');
  const styles = fs.readFileSync(path.resolve('../chrome-extension/popup.css'), 'utf8');

  assert.match(html, /class="tab-indicator"/);
  assert.match(html, /id="plugin-version"/);
  assert.match(html, /id="date-picker" class="date-picker"/);
  assert.match(html, /date-availability\.js/);
  assert.match(html, /id="body" class="result-list"/);
  assert.match(html, /id="source-id"/);
  assert.match(html, /id="page-number"/);
  assert.match(html, /class="telemetry"/);
  assert.doesNotMatch(html, /<table|class="system-metrics"/);
  assert.match(script, /function renderLogRecord\(row\)/);
  assert.match(script, /destinationGeo\(row\)/);
  assert.match(script, /class="geo-label"/);
  assert.match(script, /\/api\/ingest-progress\?range=all&include_ready=true/);
  assert.match(script, /parseDateTimeInput/);
  assert.match(script, /source_id: \$\('source-id'\)/);
  assert.match(script, /function renderSourceStatus/);
  assert.match(script, /aria-expanded/);
  assert.match(styles, /--font-data: "Cascadia Mono"/);
  assert.match(styles, /grid-template-columns: 88px 62px/);
  assert.match(styles, /grid-template-columns: minmax\(0, 1fr\) 64px/);
  assert.match(styles, /\.calendar-legend/);
});
