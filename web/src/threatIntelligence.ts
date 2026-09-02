export const THREAT_PROVIDER_META = {
  threatbook: { name: '微步' },
  nsfocus: { name: '绿盟' },
  qianxin: { name: '奇安信' },
  tencent: { name: '腾讯' },
} as const;

export type ThreatProvider = keyof typeof THREAT_PROVIDER_META;

export type ThreatProviderStatus = {
  provider: ThreatProvider;
  name: string;
  enabled: boolean;
  configured: boolean;
  credential_error?: string;
  last_test_status?: 'success' | 'failed' | '';
  last_test_message?: string;
  last_tested_at?: string;
};

export type ThreatIntelligenceResult = {
  provider: ThreatProvider;
  ip: string;
  verdict: 'malicious' | 'suspicious' | 'benign' | 'unknown';
  risk_level: 'critical' | 'high' | 'medium' | 'low' | 'info' | 'unknown';
  confidence_score: number | null;
  confidence_level: 'high' | 'medium' | 'low' | 'unknown';
  tags: string[];
  first_seen: string | null;
  last_seen: string | null;
  source_updated_at: string | null;
  analyzed_at: string;
  summary: string;
  raw_response: unknown;
};

export type ThreatProviderListResponse = { providers: ThreatProviderStatus[] };
export type ThreatResultResponse = { result: ThreatIntelligenceResult | null };
export type ThreatAnalysisResponse = {
  result?: ThreatIntelligenceResult;
  previous_result?: ThreatIntelligenceResult;
};

export function visibleThreatProviders(statuses: ThreatProviderStatus[]): ThreatProviderStatus[] {
  return statuses.filter(
    (item) => item.enabled && item.configured && !item.credential_error && item.last_test_status !== 'failed',
  );
}

const VERDICT_TEXT: Record<ThreatIntelligenceResult['verdict'], string> = {
  malicious: '恶意',
  suspicious: '可疑',
  benign: '良性',
  unknown: '未知',
};

const RISK_TEXT: Record<ThreatIntelligenceResult['risk_level'], string> = {
  critical: '严重',
  high: '高',
  medium: '中',
  low: '低',
  info: '信息',
  unknown: '未知',
};

const RISK_COLOR: Record<ThreatIntelligenceResult['risk_level'], string> = {
  critical: 'red',
  high: 'orange',
  medium: 'gold',
  low: 'blue',
  info: 'green',
  unknown: 'default',
};

const CONFIDENCE_TEXT: Record<ThreatIntelligenceResult['confidence_level'], string> = {
  high: '高',
  medium: '中',
  low: '低',
  unknown: '未知',
};

export function verdictText(verdict: string): string {
  return VERDICT_TEXT[verdict as ThreatIntelligenceResult['verdict']] ?? VERDICT_TEXT.unknown;
}

export function riskText(risk: string): string {
  return RISK_TEXT[risk as ThreatIntelligenceResult['risk_level']] ?? RISK_TEXT.unknown;
}

export function riskColor(risk: string): string {
  return RISK_COLOR[risk as ThreatIntelligenceResult['risk_level']] ?? RISK_COLOR.unknown;
}

export function formatConfidence(score: number | null | undefined, level: string): string {
  if (typeof score === 'number' && Number.isFinite(score)) return String(score);
  return CONFIDENCE_TEXT[level as ThreatIntelligenceResult['confidence_level']] ?? CONFIDENCE_TEXT.unknown;
}
