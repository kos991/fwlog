import React from 'react';
import { BarChartOutlined, FolderOpenOutlined } from '@ant-design/icons';
import { Tabs } from 'antd';
import { IncrementalProgressPage } from './IncrementalProgressPage';
import { SystemMaintenancePage } from './SystemMaintenancePage';

type LogReceptionPageProps = {
  onRequireLogin: () => void;
  initialTab?: 'progress' | 'sources';
};

export function LogReceptionPage({ onRequireLogin, initialTab = 'progress' }: LogReceptionPageProps) {
  return (
    <div className="page-stack log-reception-page">
      <section className="page-header">
        <div><h1>日志接收</h1></div>
      </section>
      <Tabs
        className="maintenance-tabs log-reception-tabs"
        defaultActiveKey={initialTab}
        items={[
          {
            key: 'progress',
            label: <span className="tab-label"><BarChartOutlined />接收进度</span>,
            children: <IncrementalProgressPage embedded />,
          },
          {
            key: 'sources',
            label: <span className="tab-label"><FolderOpenOutlined />日志来源</span>,
            children: <SystemMaintenancePage scope="sources" embedded onRequireLogin={onRequireLogin} />,
          },
        ]}
      />
    </div>
  );
}
