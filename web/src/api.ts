export async function apiGet<T>(path: string): Promise<T> {
  return requestJSON<T>(path, { method: 'GET' });
}

export async function apiPost<T>(path: string, body?: unknown): Promise<T> {
  return requestJSON<T>(path, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
}

async function requestJSON<T>(path: string, init: RequestInit): Promise<T> {
  const apiBaseURL = import.meta.env.VITE_API_BASE_URL || '';
  const useMock = String(import.meta.env.VITE_USE_MOCK || '').toLowerCase() === 'true';

  if (useMock) {
    return mockResponse<T>(path);
  }

  const response = await fetch(`${apiBaseURL}${path}`, {
    ...init,
    credentials: 'include',
    headers: {
      Accept: 'application/json',
      ...(init.headers || {}),
    },
  });
  if (!response.ok) {
    throw new Error(await readErrorMessage(response));
  }
  return (await response.json()) as T;
}

function mockResponse<T>(path: string): T {
  if (path.startsWith('/api/session') || path.startsWith('/api/login')) {
    return { authenticated: true } as T;
  }
  if (path.startsWith('/api/logout')) {
    return { authenticated: false } as T;
  }
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
        12, 18, 9, 28, 65, 81, 44, 39, 55, 61, 48, 72,
        88, 94, 76, 69, 83, 102, 97, 86, 64, 42, 31, 20,
      ].map((value, index) => ({ name: `${String(index).padStart(2, '0')}:00`, value })),
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
      current_version: 'v1.1.0',
      latest_version: 'v1.1.0',
      update_available: false,
      release_url: 'https://github.com/kos991/fwlog/releases/tag/v1.1.0',
      assets_ready: true,
      missing_assets: [],
      status: {
        state: 'idle',
        current_version: 'v1.1.0',
      },
    } as T;
  }
  if (path.startsWith('/api/upgrade/status') || path.startsWith('/api/upgrade/run')) {
    return {
      state: 'idle',
      current_version: 'v1.1.0',
      target_version: 'v1.1.0',
      message: '模拟升级状态',
    } as T;
  }
  if (path.startsWith('/api/settings')) {
    return {
      log_dir: '/data/sangfor_fw_log',
      log_tag: '深信服 NAT',
      log_sources: [{ source_id: 'sangfor-main', log_tag: '深信服 NAT', log_dir: '/data/sangfor_fw_log', enabled: true }],
      cidr_aliases: [{ cidr: '10.10.0.0/16', alias: '办公网段', enabled: true }],
      custom_ip_map_path: '/opt/nat-query/custom_ip_map.csv',
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
    // 非 JSON 时保留原始响应，方便排查网关或静态服务错误。
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
};
