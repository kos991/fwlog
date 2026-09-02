import React from 'react';
import { Alert, Button, Collapse, Empty, Popover, Spin, Tag, Tooltip } from 'antd';
import { ReloadOutlined, SearchOutlined } from '@ant-design/icons';
import { apiGet, apiPost } from '../api';
import {
  THREAT_PROVIDER_META,
  formatConfidence,
  riskColor,
  riskText,
  verdictText,
  type ThreatAnalysisResponse,
  type ThreatIntelligenceResult,
  type ThreatProvider,
  type ThreatProviderStatus,
  type ThreatResultResponse,
} from '../threatIntelligence';
import threatbookIcon from '../assets/threat-intelligence/threatbook.svg';
import nsfocusIcon from '../assets/threat-intelligence/nsfocus.svg';
import qianxinIcon from '../assets/threat-intelligence/qianxin.svg';
import tencentIcon from '../assets/threat-intelligence/tencent.svg';

const PROVIDER_ICONS: Record<ThreatProvider, string> = {
  threatbook: threatbookIcon,
  nsfocus: nsfocusIcon,
  qianxin: qianxinIcon,
  tencent: tencentIcon,
};

type ThreatIntelligencePopoverProps = {
  ip: string;
  provider: ThreatProvider;
};

function resultView(result: ThreatIntelligenceResult) {
  return (
    <div className="threat-intelligence-result">
      <div className="threat-result-summary">
        <Tag color={result.verdict === 'malicious' ? 'red' : result.verdict === 'suspicious' ? 'orange' : 'green'}>
          {verdictText(result.verdict)}
        </Tag>
        <Tag color={riskColor(result.risk_level)}>{riskText(result.risk_level)}</Tag>
      </div>
      <div className="threat-result-grid">
        <span>置信度</span><strong>{formatConfidence(result.confidence_score, result.confidence_level)}</strong>
        <span>分析时间</span><strong>{result.analyzed_at || '-'}</strong>
      </div>
      {result.summary ? <p className="threat-result-summary-text">{result.summary}</p> : null}
      {result.tags?.length ? <div className="threat-result-tags">{result.tags.map((tag) => <Tag key={tag}>{tag}</Tag>)}</div> : null}
      <Collapse
        ghost
        items={[{
          key: 'raw',
          label: '原始详情',
          children: <pre className="threat-result-raw">{JSON.stringify(result.raw_response ?? null, null, 2)}</pre>,
        }]}
      />
    </div>
  );
}

export function ThreatIntelligencePopover({ ip, provider }: ThreatIntelligencePopoverProps) {
  const [open, setOpen] = React.useState(false);
  const [loading, setLoading] = React.useState(false);
  const [analyzing, setAnalyzing] = React.useState(false);
  const [loaded, setLoaded] = React.useState(false);
  const [result, setResult] = React.useState<ThreatIntelligenceResult | null>(null);
  const [error, setError] = React.useState<string | null>(null);
  const meta = THREAT_PROVIDER_META[provider];

  const loadHistory = React.useCallback(async () => {
    if (!ip || loaded) return;
    setLoading(true);
    setError(null);
    try {
      const response = await apiGet<ThreatResultResponse>(`/api/threat-intelligence/providers/${provider}/results?ip=${encodeURIComponent(ip)}`);
      setResult(response.result ?? null);
      setLoaded(true);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : '读取历史结果失败');
    } finally {
      setLoading(false);
    }
  }, [ip, loaded, provider]);

  const analyze = React.useCallback(async () => {
    if (!ip || analyzing) return;
    setAnalyzing(true);
    setError(null);
    try {
      const response = await apiPost<ThreatAnalysisResponse>(`/api/threat-intelligence/providers/${provider}/analyze`, { ip });
      setResult(response.result ?? null);
      setLoaded(true);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : '本次分析失败');
    } finally {
      setAnalyzing(false);
    }
  }, [analyzing, ip, provider]);

  const content = (
    <div className="threat-intelligence-popover">
      {loading ? <div className="threat-result-loading"><Spin size="small" /> <span>正在读取本地历史…</span></div> : null}
      {!loading && error && !result ? <Alert type="error" showIcon message={error} /> : null}
      {!loading && !result && !error ? <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无本地结果" /> : null}
      {!loading && result ? resultView(result) : null}
      {error && result ? <Alert className="threat-result-error" type="error" showIcon message={`本次分析失败：${error}`} /> : null}
      <div className="threat-result-actions">
        <Button
          type="primary"
          size="small"
          icon={result ? <ReloadOutlined /> : <SearchOutlined />}
          loading={analyzing}
          onClick={() => void analyze()}
        >
          {result ? '重新分析' : '开始分析'}
        </Button>
      </div>
    </div>
  );

  return (
    <Popover
      title={`${meta.name}威胁情报`}
      content={content}
      trigger="click"
      open={open}
      onOpenChange={(nextOpen) => {
        setOpen(nextOpen);
        if (nextOpen) void loadHistory();
      }}
      overlayClassName="threat-intelligence-popover-overlay"
    >
      <Tooltip title={meta.name}>
        <Button
          type="text"
          shape="circle"
          className="threat-provider-trigger"
          aria-label={`${meta.name}威胁情报`}
          icon={<img src={PROVIDER_ICONS[provider]} alt="" />}
        />
      </Tooltip>
    </Popover>
  );
}

type ThreatIntelligenceActionsProps = {
  ip: string;
  providers: ThreatProviderStatus[];
};

export function ThreatIntelligenceActions({ ip, providers }: ThreatIntelligenceActionsProps) {
  if (!ip) return null;
  return (
    <div className="threat-intelligence-actions" aria-label="威胁情报平台">
      {providers
        .filter((item) => item.enabled && item.configured && !item.credential_error && item.last_test_status !== 'failed')
        .map((item) => <ThreatIntelligencePopover key={item.provider} ip={ip} provider={item.provider} />)}
    </div>
  );
}
