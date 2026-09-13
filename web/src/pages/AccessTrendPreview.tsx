import React from 'react';
import { ReloadOutlined } from '@ant-design/icons';
import { Button, Empty, Segmented, Select, Spin, message } from 'antd';
import { EffectScatterChart } from 'echarts/charts';
import { GeoComponent, TooltipComponent } from 'echarts/components';
import {
  init,
  registerMap,
  use,
  type ECharts,
  type EChartsCoreOption,
} from 'echarts/core';
import { CanvasRenderer } from 'echarts/renderers';
import { apiGet } from '../api';
import chinaMap from '../assets/maps/china.json';
import worldMap from '../assets/maps/world.json';

use([GeoComponent, TooltipComponent, EffectScatterChart, CanvasRenderer]);
registerMap('china', chinaMap as unknown as Parameters<typeof registerMap>[1]);
registerMap('world', worldMap as unknown as Parameters<typeof registerMap>[1]);

type ViewMode = 'all' | 'domestic' | 'overseas';
type Point = {
  id: string;
  mode: Exclude<ViewMode, 'all'> | 'unknown';
  target_ip: string;
  target_region: string;
  count: number;
  coord?: [number, number];
};
type TrendResponse = {
  points: Point[];
  summary: {
    total: number;
    domestic: number;
    overseas: number;
    unknown?: number;
  };
};

function buildMapOption(mode: ViewMode, shown: Point[]): EChartsCoreOption {
  const mappedPoints = shown
    .filter((point) => point.coord)
    .map((point) => ({
      name: point.target_region,
      pointId: point.id,
      value: [...point.coord!, point.count],
      itemStyle: {
        color: point.mode === 'domestic' ? '#14b8a6' : '#f59e0b',
      },
    }));
  return {
    backgroundColor: 'transparent',
    tooltip: {
      trigger: 'item',
      borderWidth: 1,
      borderColor: '#dfe7f1',
      backgroundColor: 'rgba(255,255,255,.96)',
      textStyle: { color: '#26354b', fontSize: 12 },
      formatter: (params: unknown) => {
        const item = params as { data?: { pointId?: string } };
        const point = shown.find(
          (candidate) => candidate.id === item.data?.pointId,
        );
        return point
          ? `${point.target_region}<br/>目标网段：${point.target_ip}<br/>访问次数：${point.count.toLocaleString('zh-CN')}`
          : '';
      },
    },
    geo: {
      map: mode === 'domestic' ? 'china' : 'world',
      roam: true,
      top: 10,
      bottom: 8,
      left: 12,
      right: 12,
      itemStyle: {
        areaColor: '#e8eef7',
        borderColor: '#c6d3e3',
        borderWidth: 0.7,
      },
      emphasis: {
        itemStyle: { areaColor: '#dbe7f7' },
        label: { show: false },
      },
      select: { disabled: true },
      label: { show: false },
    },
    series: [
      {
        name: '目标地域节点',
        type: 'effectScatter',
        coordinateSystem: 'geo',
        zlevel: 2,
        rippleEffect: { scale: 2.3, brushType: 'stroke' },
        symbolSize: (value: unknown) =>
          Math.max(7, Math.min(13, Math.sqrt((value as number[])[2] || 0) / 6)),
        data: mappedPoints,
      },
    ],
  };
}

export function AccessTrendPreview({
  embedded = false,
  sourceID,
}: {
  embedded?: boolean;
  sourceID?: string;
}) {
  const chartRef = React.useRef<HTMLDivElement>(null);
  const chartInstanceRef = React.useRef<ECharts | null>(null);
  const abortRef = React.useRef<AbortController | null>(null);
  const [mode, setMode] = React.useState<ViewMode>('all');
  const [range, setRange] = React.useState('7d');
  const [data, setData] = React.useState<TrendResponse | null>(null);
  const [loading, setLoading] = React.useState(true);
  const [error, setError] = React.useState('');

  const load = React.useCallback(async () => {
    abortRef.current?.abort();
    const controller = new AbortController();
    abortRef.current = controller;
    setLoading(true);
    setError('');
    try {
      const params = new URLSearchParams({ range, mode });
      if (sourceID) params.set('source_id', sourceID);
      const payload = await apiGet<TrendResponse>(
        `/api/access-trend?${params}`,
        { signal: controller.signal },
      );
      setData(payload);
    } catch (reason) {
      if (!(reason instanceof DOMException && reason.name === 'AbortError')) {
        const detail = reason instanceof Error ? reason.message : '访问趋势加载失败';
        setError(detail);
        message.error(detail);
      }
    } finally {
      if (!controller.signal.aborted) setLoading(false);
    }
  }, [mode, range, sourceID]);

  React.useEffect(() => {
    void load();
    return () => abortRef.current?.abort();
  }, [load]);

  React.useEffect(() => {
    if (!chartRef.current) return;
    const chart = init(chartRef.current, undefined, { renderer: 'canvas' });
    chartInstanceRef.current = chart;
    const resizeObserver = new ResizeObserver(() => chart.resize());
    resizeObserver.observe(chartRef.current);
    return () => {
      resizeObserver.disconnect();
      chart.dispose();
      chartInstanceRef.current = null;
    };
  }, []);

  const shown = data?.points || [];
  const mappedCount = shown.filter((point) => point.coord).length;
  React.useEffect(() => {
    chartInstanceRef.current?.setOption(buildMapOption(mode, shown), true);
  }, [mode, shown]);

  const toolbar = (
    <div className="access-preview-toolbar">
      <Segmented
        value={mode}
        onChange={(value) => setMode(value as ViewMode)}
        options={[
          { label: '全部访问', value: 'all' },
          { label: '国内访问', value: 'domestic' },
          { label: '国外访问', value: 'overseas' },
        ]}
      />
      <Select
        size="small"
        value={range}
        onChange={setRange}
        options={[
          { value: 'today', label: '今天' },
          { value: '3d', label: '最近 3 天' },
          { value: '7d', label: '最近 7 天' },
        ]}
      />
      <Button
        size="small"
        icon={<ReloadOutlined />}
        aria-label="刷新访问趋势"
        onClick={() => void load()}
      />
    </div>
  );

  return (
    <main
      className={`access-preview${embedded ? ' access-preview--embedded' : ''}`}
    >
      {!embedded && (
        <header className="access-preview-header">
          <div>
            <span className="eyebrow">数据概览</span>
            <h1>访问趋势</h1>
          </div>
          {toolbar}
        </header>
      )}
      <section className="access-preview-grid">
        <div className="access-map-panel">
          <div className="access-map-topbar">
            <div className="access-map-meta">
              <span>
                <i className="access-dot access-dot--target" />
                目标地域节点
              </span>
              {!embedded && <span className="access-map-hint">滚轮缩放 · 拖动地图</span>}
            </div>
            <div className="access-stat-grid">
              <div>
                <span>访问总量</span>
                <strong>{(data?.summary.total || 0).toLocaleString('zh-CN')}</strong>
              </div>
              <div>
                <span>国内</span>
                <strong>{(data?.summary.domestic || 0).toLocaleString('zh-CN')}</strong>
              </div>
              <div>
                <span>国外</span>
                <strong>{(data?.summary.overseas || 0).toLocaleString('zh-CN')}</strong>
              </div>
            </div>
            {embedded && toolbar}
          </div>
          <div
            ref={chartRef}
            className="access-map"
            role="img"
            aria-label="国内国外访问目标地域地图"
          />
          {loading && (
            <div className="access-trend-overlay">
              <Spin size="small" /> 正在加载访问趋势
            </div>
          )}
          {!loading && error && (
            <div className="access-trend-overlay access-trend-error">{error}</div>
          )}
          {!loading && !error && mappedCount === 0 && (
            <div className="access-trend-overlay">
              <Empty
                image={Empty.PRESENTED_IMAGE_SIMPLE}
                description="当前时间范围暂无可定位的访问目标"
              />
            </div>
          )}
          <div className="access-map-footer">
            <span>节点 {mappedCount} / 50</span>
            <span>{range === 'today' ? '今天' : `最近 ${range === '3d' ? '3' : '7'} 天`}</span>
            <span>单位：访问次数</span>
          </div>
        </div>
      </section>
    </main>
  );
}
