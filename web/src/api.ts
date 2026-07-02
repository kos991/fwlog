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
  if (import.meta.env.DEV && !apiBaseURL) {
    return mockResponse<T>(path);
  }

  try {
    const response = await fetch(`${apiBaseURL}${path}`, {
      ...init,
      credentials: 'include',
      headers: {
        Accept: 'application/json',
        ...(init.headers || {}),
      },
    });
    if (!response.ok) {
      throw new Error(await response.text() || `请求失败：${response.status}`);
    }
    return (await response.json()) as T;
  } catch (error) {
    if (import.meta.env.DEV) {
      return mockResponse<T>(path);
    }
    throw error;
  }
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
        clickhouse_disk_used_bytes: 42_860_000_000,
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
        bytes_total: 9_800_000_000,
        bytes_done: 6_200_000_000,
        rows_imported: 812430,
        progress_pct: 63,
        error: '',
        next_auto_scan_at: '2026-07-02 09:30:00',
      },
      ip_distribution: {
        top_source_ips: [
          { name: '10.10.2.18', value: 481230 },
          { name: '10.10.9.77', value: 359021 },
          { name: '10.12.1.44', value: 283402 },
          { name: '10.10.3.91', value: 221980 },
          { name: '10.12.6.20', value: 198450 },
          { name: '10.11.8.56', value: 176230 },
          { name: '10.10.14.8', value: 154902 },
          { name: '10.13.2.70', value: 130441 },
        ],
        top_destination_ips: [
          { name: '114.114.114.114', value: 221980 },
          { name: '223.5.5.5', value: 198450 },
          { name: '119.29.29.29', value: 176880 },
          { name: '8.8.8.8', value: 154210 },
          { name: '180.76.76.76', value: 127960 },
          { name: '1.1.1.1', value: 100230 },
          { name: '101.226.4.6', value: 92330 },
          { name: '202.96.128.86', value: 80120 },
        ],
        top_nat_ips: [
          { name: '172.16.0.12', value: 845301 },
          { name: '172.16.0.18', value: 612480 },
          { name: '172.16.0.21', value: 501220 },
          { name: '172.16.0.32', value: 388120 },
          { name: '172.16.0.45', value: 290230 },
          { name: '172.16.0.52', value: 210440 },
          { name: '172.16.0.61', value: 155860 },
          { name: '172.16.0.70', value: 98040 },
        ],
        address_type_shares: [
          { name: '内网', value: 72 },
          { name: '公网', value: 28 },
        ],
        log_tag_distribution: [{ name: '深信服 NAT', value: 18625430 }],
      },
      geo_distribution: {
        top_countries: [
          { name: '中国', value: 821330 },
          { name: '美国', value: 128440 },
          { name: '日本', value: 82420 },
          { name: '德国', value: 61980 },
          { name: '新加坡', value: 58410 },
          { name: '巴西', value: 42330 },
          { name: '韩国', value: 39870 },
          { name: '英国', value: 35120 },
        ],
        top_regions: [
          { name: '广东', value: 312900 },
          { name: '浙江', value: 188040 },
          { name: '江苏', value: 162800 },
          { name: '上海', value: 130420 },
          { name: '北京', value: 112560 },
          { name: '四川', value: 88430 },
          { name: '湖北', value: 70320 },
          { name: '福建', value: 58190 },
        ],
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
      bytes_total: 9_800_000_000,
      bytes_done: 6_200_000_000,
      rows_imported: 812430,
      progress_pct: 63,
      error: '',
      dates: [
        { log_date: '2026-07-01', status: 'importing', files_total: 12, files_done: 8, rows_imported: 812430, progress_pct: 63 },
        { log_date: '2026-06-30', status: 'ready', files_total: 11, files_done: 11, rows_imported: 1259044, progress_pct: 100 },
      ],
    } as T;
  }
  if (path.startsWith('/api/settings')) {
    return {
      log_dir: '/data/sangfor_fw_log',
      log_tag: '深信服 NAT',
      log_sources: [
        { source_id: 'sangfor-main', log_tag: '深信服 NAT', log_dir: '/data/sangfor_fw_log', enabled: true },
        { source_id: 'branch-fw', log_tag: '分部防火墙', log_dir: '/data/branch_fw_log', enabled: true },
        { source_id: 'vpn-edge', log_tag: 'VPN 出口', log_dir: '/data/vpn_edge_log', enabled: false },
      ],
      cidr_aliases: [
        { cidr: '10.10.0.0/16', alias: '办公网段', enabled: true },
        { cidr: '10.12.0.0/16', alias: '生产网段', enabled: true },
        { cidr: '172.16.0.0/24', alias: 'NAT 地址池', enabled: false },
      ],
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

export type QueryVisibility = {
  partial: boolean;
  message: string;
  queried_ranges: Array<{ log_date: string; start_time: string; end_time: string; status: string }>;
  skipped_dates: Array<{ log_date: string; status: string; reason: string }>;
};

export type DistributionItem = { name: string; value: number };
