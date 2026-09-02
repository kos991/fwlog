import type { ThreatIntelligenceResult, ThreatProvider, ThreatProviderStatus } from './threatIntelligence';

export async function apiGet<T>(path: string, options?: Pick<RequestInit, 'signal'>): Promise<T> {
  return requestJSON<T>(path, { method: 'GET', signal: options?.signal });
}

export async function apiPost<T>(path: string, body?: unknown): Promise<T> {
  return requestJSON<T>(path, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
}

export async function apiUpload<T>(path: string, file: File): Promise<T> {
  const body = new FormData();
  body.append('package', file);
  return requestJSON<T>(path, { method: 'POST', body });
}

let onUnauthorized: (() => void) | null = null;

export function setOnUnauthorized(handler: (() => void) | null): void {
  onUnauthorized = handler;
}

async function requestJSON<T>(path: string, init: RequestInit): Promise<T> {
  const apiBaseURL = import.meta.env.VITE_API_BASE_URL || '';
  const useMock = String(import.meta.env.VITE_USE_MOCK || '').toLowerCase() === 'true';

  if (useMock) {
    return mockResponse<T>(path, init);
  }

  const controller = new AbortController();
  const timeoutId = window.setTimeout(() => controller.abort(), 30000);
  const externalSignal = init.signal;
  const abortExternal = () => controller.abort();
  externalSignal?.addEventListener('abort', abortExternal, { once: true });
  try {
    const response = await fetch(`${apiBaseURL}${path}`, {
      ...init,
      credentials: 'include',
      signal: controller.signal,
      headers: {
        Accept: 'application/json',
        ...(init.headers || {}),
      },
    });
    if (response.status === 401) {
      if (onUnauthorized) onUnauthorized();
      throw new Error('登录已失效，请重新登录');
    }
    if (!response.ok) {
      throw new Error(await readErrorMessage(response));
    }
    return (await response.json()) as T;
  } finally {
    window.clearTimeout(timeoutId);
    externalSignal?.removeEventListener('abort', abortExternal);
  }
}

let mockLogSources = [
  { source_id: 'sangfor-main', serial_number: 'SN-SANGFOR-001', log_tag: '深信服 NAT', log_dir: '/data/sangfor_fw_log', source_type: 'file', enabled: true },
  {
    source_id: 'rsyslog-main',
    serial_number: 'SN-RSYSLOG-001',
    log_tag: '核心防火墙',
    log_dir: '/data/fwlog/received/rsyslog-main',
    source_type: 'rsyslog',
    listen_protocol: 'udp',
    listen_host: '0.0.0.0',
    listen_port: 5514,
    spool_dir: '/data/fwlog/received/rsyslog-main',
    client_ip: '192.168.10.20',
    archive_dir: '',
    archive_retention_days: 0,
    enabled: true,
  },
];

const mockThreatCredential = 'test-key';
const mockThreatResultsPath = '/results';
const mockThreatAnalyzePath = '/analyze';
const mockThreatTestPath = '/test';
let mockThreatProviders: ThreatProviderStatus[] = [
  { provider: 'threatbook', name: '微步', enabled: true, configured: true, last_test_status: 'success' },
  { provider: 'nsfocus', name: '绿盟', enabled: false, configured: true, last_test_status: 'success' },
  { provider: 'qianxin', name: '奇安信', enabled: false, configured: false, last_test_status: '' },
  { provider: 'tencent', name: '腾讯', enabled: true, configured: true, last_test_status: 'success' },
];

function mockThreatResult(provider: ThreatProvider, ip = '8.8.8.8', analyzedAt = '2026-08-31T08:00:00+08:00'): ThreatIntelligenceResult {
  return {
    provider,
    ip,
    verdict: 'benign',
    risk_level: 'info',
    confidence_score: 96,
    confidence_level: 'high',
    tags: ['公共 DNS'],
    first_seen: null,
    last_seen: null,
    source_updated_at: analyzedAt,
    analyzed_at: analyzedAt,
    summary: '未发现威胁情报记录',
    raw_response: { source: 'local-mock', ip },
  };
}

let mockThreatResults: Partial<Record<ThreatProvider, ThreatIntelligenceResult>> = {
  threatbook: mockThreatResult('threatbook'),
  tencent: mockThreatResult('tencent'),
};

function mockThreatResponse<T>(path: string, init: RequestInit): T | undefined {
  const prefix = '/api/threat-intelligence/providers';
  if (path === prefix && init.method === 'GET') {
    return { providers: mockThreatProviders } as T;
  }
  if (!path.startsWith(`${prefix}/`)) return undefined;

  const request = new URL(path, 'http://fwlog.mock');
  const parts = request.pathname.split('/').filter(Boolean);
  const provider = parts[3] as ThreatProvider;
  const operation = parts[4] || 'configuration';
  const status = mockThreatProviders.find((item) => item.provider === provider);
  if (!status) return {} as T;

  if (operation === mockThreatResultsPath.slice(1) && init.method === 'GET') {
    return { result: mockThreatResults[provider] ?? null } as T;
  }
  if (operation === mockThreatAnalyzePath.slice(1) && init.method === 'POST') {
    const payload = typeof init.body === 'string' ? (JSON.parse(init.body) as { ip?: string }) : {};
    const previousResult = mockThreatResults[provider];
    const result = mockThreatResult(provider, payload.ip || '8.8.8.8', '2026-08-31T08:05:00+08:00');
    mockThreatResults = { ...mockThreatResults, [provider]: result };
    return { result, previous_result: previousResult } as T;
  }
  if (operation === mockThreatTestPath.slice(1) && init.method === 'POST') {
    const testedAt = '2026-08-31T08:10:00+08:00';
    const updated = { ...status, last_test_status: 'success' as const, last_test_message: '连接成功', last_tested_at: testedAt };
    mockThreatProviders = mockThreatProviders.map((item) => (item.provider === provider ? updated : item));
    return updated as T;
  }
  if (operation === 'configuration' && init.method === 'POST') {
    const payload = typeof init.body === 'string'
      ? (JSON.parse(init.body) as { enabled?: boolean; credential?: string | null; clear_credential?: boolean })
      : {};
    const hasCredential = typeof payload.credential === 'string' && payload.credential.trim() !== '';
    const configured = payload.clear_credential ? false : hasCredential ? payload.credential === mockThreatCredential : status.configured;
    const updated = { ...status, enabled: payload.enabled ?? status.enabled, configured };
    mockThreatProviders = mockThreatProviders.map((item) => (item.provider === provider ? updated : item));
    return updated as T;
  }
  return {} as T;
}

function mockResponse<T>(path: string, init: RequestInit): T {
  if (path.startsWith('/api/session') || path.startsWith('/api/login')) {
    return { authenticated: true } as T;
  }
  if (path.startsWith('/api/logout')) {
    return { authenticated: false } as T;
  }
  const threatResponse = mockThreatResponse<T>(path, init);
  if (threatResponse !== undefined) return threatResponse;
  if (path.startsWith('/api/health-dashboard')) {
    return {
      data_health: {
        total_logs: 18625430,
        ready_dates: 28,
        pending_dates: 2,
        importing_dates: 1,
        failed_dates: 0,
        queryable_start_date: '2026-06-01',
        queryable_end_date: '2026-07-01',
        last_successful_ingest_time: '2026-07-02 08:30:18',
        clickhouse_disk_used_bytes: 42860000000,
        today_rows: 812430,
        yesterday_rows: 1259044,
      },
      ingest_health: {
        status: 'importing',
        source_id: 'default',
        log_tag: '深信服 NAT',
        current_date: '2026-07-01',
        current_file: 'firewall.log-20260701.gz',
        files_total: 12,
        files_done: 8,
        bytes_total: 9800000000,
        bytes_done: 6200000000,
        rows_imported: 812430,
        progress_pct: 63,
        error: '',
        next_auto_scan_at: '2026-07-02 09:30:00',
      },
      log_trend: [
        { date: '2026-06-18', source_id: 'sangfor-main', log_tag: '深信服 NAT', value: 1259044 },
        { date: '2026-06-19', source_id: 'sangfor-main', log_tag: '深信服 NAT', value: 1182033 },
        { date: '2026-06-20', source_id: 'sangfor-main', log_tag: '深信服 NAT', value: 1324108 },
        { date: '2026-06-21', source_id: 'rsyslog-main', log_tag: '核心防火墙', value: 520410 },
        { date: '2026-06-22', source_id: 'sangfor-main', log_tag: '深信服 NAT', value: 1218820 },
        { date: '2026-06-22', source_id: 'rsyslog-main', log_tag: '核心防火墙', value: 612320 },
        { date: '2026-06-23', source_id: 'sangfor-main', log_tag: '深信服 NAT', value: 1410201 },
        { date: '2026-06-24', source_id: 'rsyslog-main', log_tag: '核心防火墙', value: 712900 },
        { date: '2026-06-25', source_id: 'sangfor-main', log_tag: '深信服 NAT', value: 1259044 },
        { date: '2026-06-26', source_id: 'sangfor-main', log_tag: '深信服 NAT', value: 812430 },
        { date: '2026-06-26', source_id: 'rsyslog-main', log_tag: '核心防火墙', value: 441200 },
        { date: '2026-06-27', source_id: 'sangfor-main', log_tag: '深信服 NAT', value: 980320 },
        { date: '2026-06-28', source_id: 'rsyslog-main', log_tag: '核心防火墙', value: 690100 },
        { date: '2026-06-29', source_id: 'sangfor-main', log_tag: '深信服 NAT', value: 1104300 },
        { date: '2026-06-30', source_id: 'sangfor-main', log_tag: '深信服 NAT', value: 1259044 },
        { date: '2026-07-01', source_id: 'sangfor-main', log_tag: '深信服 NAT', value: 812430 },
      ],
      system_health: {
        cpu: {
          status: 'ok',
          load_percent: 18.4,
          load_average: 0.74,
          cores: 4,
          description: 'CPU 正常',
        },
        memory: {
          status: 'ok',
          total_bytes: 17179869184,
          available_bytes: 9282686976,
          used_percent: 46,
          description: '内存正常',
        },
        database: {
          status: 'ok',
          version: '25.8.27.1',
          active_queries: 1,
          active_merges: 0,
          active_parts: 128,
          total_rows: 18625430,
          disk_used_bytes: 42860000000,
          description: 'ClickHouse 正常',
        },
      },
      ip_distribution: {
        top_source_ips: [{ name: '10.10.2.18', value: 481230 }],
        top_destination_ips: [{ name: '114.114.114.114', value: 221980 }],
        top_nat_ips: [{ name: '172.16.0.12', value: 845301 }],
        address_type_shares: [{ name: '内网', value: 72 }, { name: '公网', value: 28 }],
        log_tag_distribution: [{ name: '深信服 NAT', value: 18625430 }],
      },
      geo_distribution: {
        top_countries: [{ name: '中国', value: 821330 }],
        top_regions: [{ name: '广东', value: 312900 }],
        unrecognized_ip_rate: 0.08,
        geoip_loaded: true,
        geoip_status: '已加载',
      },
    } as T;
  }
  if (path.startsWith('/api/query')) {
    return {
      records: [
        {
          id: '1',
          timestamp: '2026-07-01 08:10:22',
          log_tag: '深信服 NAT',
          src_ip: '10.10.2.18',
          src_port: 53218,
          dst_ip: '114.114.114.114',
          dst_port: 53,
          nat_ip: '172.16.0.12',
          nat_port: 42001,
          protocol: 'UDP',
          action: 'ALLOW',
          src_ip_label: '办公终端',
          dst_geo: '中国',
          source_file: 'firewall.log-20260701.gz',
          source_offset: 184220,
          source_id: 'default',
          log_date: '2026-07-01',
          ingested_at: '2026-07-02 08:12:01',
        },
      ],
      total: 1,
      page: 1,
      page_size: 50,
      next_cursor: '',
      has_more: false,
      query_time_ms: 18,
      visibility: {
        partial: true,
        message: '所选时间包含未完成入库日期，已自动只查询已入库部分。',
        queried_ranges: [
          {
            log_date: '2026-07-01',
            start_time: '2026-07-01 00:00:00',
            end_time: '2026-07-01 23:59:59',
            status: 'ready',
          },
        ],
        skipped_dates: [{ log_date: '2026-07-02', status: 'importing', reason: '仍在入库' }],
      },
    } as T;
  }
  if (path.startsWith('/api/ingest-progress')) {
    return {
      status: 'importing',
      source_id: 'default',
      log_tag: '深信服 NAT',
      current_date: '2026-07-01',
      current_file: 'firewall.log-20260701.gz',
      files_total: 12,
      files_done: 8,
      bytes_total: 9800000000,
      bytes_done: 6200000000,
      rows_imported: 812430,
      progress_pct: 63,
      error: '',
      dates: [
        { log_date: '2026-07-01', status: 'importing', files_total: 12, files_done: 8, rows_imported: 812430, progress_pct: 63 },
        { log_date: '2026-06-30', status: 'ready', files_total: 11, files_done: 11, rows_imported: 1259044, progress_pct: 100 },
      ],
    } as T;
  }
  if (path.startsWith('/api/upgrade/check')) {
    return {
      current_version: 'v2.0.0',
      latest_version: 'v2.0.0',
      update_available: false,
      release_url: 'https://github.com/kos991/fwlog/releases/tag/v2.0.0',
      assets_ready: true,
      missing_assets: [],
      status: {
        state: 'idle',
        current_version: 'v2.0.0',
      },
    } as T;
  }
  if (path.startsWith('/api/upgrade/status') || path.startsWith('/api/upgrade/run')) {
    return {
      state: 'idle',
      current_version: 'v2.0.0',
      target_version: 'v2.0.0',
      message: '模拟升级状态',
    } as T;
  }
  if (path.startsWith('/api/receiver/status')) {
    return {
      'rsyslog-main': {
        source_id: 'rsyslog-main',
        protocol: 'udp',
        address: '0.0.0.0:5514',
        port: 5514,
        spool_dir: '/data/fwlog/received/rsyslog-main',
        client_ip: '192.168.10.20',
        running: true,
        error: '',
        last_client_ip: '192.168.10.20',
        last_received_at: '2026-07-14T08:30:00+08:00',
        received_messages: 128,
        archive_error: '',
        last_archive_at: '2026-07-14T00:01:00+08:00',
      },
    } as T;
  }
  if (path.startsWith('/api/settings')) {
    if (init.method === 'POST' && typeof init.body === 'string') {
      const payload = JSON.parse(init.body) as { log_sources?: unknown };
      if (typeof payload.log_sources === 'string') {
        const parsed = JSON.parse(payload.log_sources) as unknown;
        if (Array.isArray(parsed)) mockLogSources = parsed as typeof mockLogSources;
      }
    }
    return {
      log_dir: '/data/sangfor_fw_log',
      log_tag: '深信服 NAT',
      log_sources: mockLogSources,
      cidr_aliases: [{ cidr: '10.10.0.0/16', alias: '办公网段', enabled: true }],
      custom_ip_map_path: '/opt/fwlog/custom_ip_map.csv',
      geoip_db_path: '/data/index/GeoLite2-City.mmdb',
      auto_scan_enabled: 'false',
      auto_scan_interval_sec: '3600',
    } as T;
  }
  return {} as T;
}

export function buildQueryString(params: Record<string, unknown>): string {
  const query = new URLSearchParams();
  Object.entries(params).forEach(([key, value]) => {
    if (value === undefined || value === null || value === '') return;
    query.set(key, String(value));
  });
  const text = query.toString();
  return text ? `?${text}` : '';
}

async function readErrorMessage(response: Response): Promise<string> {
  const text = await response.text();
  if (!text) {
    return `请求失败，状态码 ${response.status}`;
  }
  try {
    const payload = JSON.parse(text) as { message?: unknown; error?: unknown };
    if (typeof payload.message === 'string' && payload.message.trim()) {
      return payload.message;
    }
    if (typeof payload.error === 'string' && payload.error.trim()) {
      return payload.error;
    }
  } catch {
    // 非 JSON 时保留原始响应，便于排查网关或静态服务错误。
  }
  return text;
}

export type QueryVisibility = {
  partial: boolean;
  message: string;
  queried_ranges: Array<{ log_date: string; start_time: string; end_time: string; status: string }>;
  skipped_dates: Array<{ log_date: string; status: string; reason: string }>;
};

export type DistributionItem = { name: string; value: number };

export type UpgradeStatus = {
  state: 'idle' | 'running' | 'succeeded' | 'failed' | string;
  current_version: string;
  target_version?: string;
  message?: string;
  error?: string;
  backup_path?: string;
  started_at?: string;
  finished_at?: string;
};

export type UpgradeCheckResponse = {
  current_version: string;
  latest_version: string;
  update_available: boolean;
  release_url: string;
  assets_ready: boolean;
  missing_assets: string[];
  message?: string;
  status: UpgradeStatus;
  runtime_version?: string;
  required_runtime_version?: string;
  runtime_compatible?: boolean;
};
