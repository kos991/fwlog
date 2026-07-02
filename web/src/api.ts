export async function apiGet<T>(path: string): Promise<T> {
  const response = await fetch(path, {
    credentials: 'include',
    headers: { Accept: 'application/json' },
  });
  return readJSON<T>(response);
}

export async function apiPost<T>(path: string, body?: unknown): Promise<T> {
  const response = await fetch(path, {
    method: 'POST',
    credentials: 'include',
    headers: {
      Accept: 'application/json',
      'Content-Type': 'application/json',
    },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  return readJSON<T>(response);
}

export function buildQueryString(params: Record<string, unknown>): string {
  const query = new URLSearchParams();
  Object.entries(params).forEach(([key, value]) => {
    if (value === undefined || value === null || value === '') return;
    if (Array.isArray(value)) {
      value.forEach((item) => query.append(key, String(item)));
      return;
    }
    query.set(key, String(value));
  });
  const text = query.toString();
  return text ? `?${text}` : '';
}

async function readJSON<T>(response: Response): Promise<T> {
  if (!response.ok) {
    const text = await response.text();
    throw new Error(text || `请求失败：${response.status}`);
  }
  return (await response.json()) as T;
}

export type QueryVisibility = {
  partial: boolean;
  message: string;
  queried_ranges: Array<{
    log_date: string;
    start_time: string;
    end_time: string;
    status: string;
  }>;
  skipped_dates: Array<{
    log_date: string;
    status: string;
    reason: string;
  }>;
};

export type QueryRecord = {
  id?: string;
  timestamp: string;
  log_tag: string;
  src_ip: string;
  src_port: number;
  dst_ip: string;
  dst_port: number;
  nat_ip: string;
  nat_port: number;
  protocol: string;
  action: string;
  src_ip_label: string;
  dst_geo: string;
  source_file: string;
  source_offset: number;
  source_id: string;
  log_date: string;
  ingested_at: string;
};

export type QueryResponse = {
  records: QueryRecord[];
  total: number;
  page: number;
  page_size: number;
  query_time_ms: number;
  visibility: QueryVisibility;
};

export type DistributionItem = { name: string; value: number };

export type HealthDashboardResponse = {
  data_health: Record<string, unknown>;
  ingest_health: Record<string, unknown>;
  ip_distribution: {
    top_source_ips?: DistributionItem[];
    top_destination_ips?: DistributionItem[];
    top_nat_ips?: DistributionItem[];
    address_type_shares?: DistributionItem[];
    log_tag_distribution?: DistributionItem[];
  };
  geo_distribution: {
    top_countries?: DistributionItem[];
    top_regions?: DistributionItem[];
    unrecognized_ip_rate?: number;
    geoip_loaded?: boolean;
    geoip_status?: string;
  };
};
