import {
  AreaChartOutlined,
  DatabaseOutlined,
  FileSearchOutlined,
  SettingOutlined,
} from '@ant-design/icons';
import { ProLayout } from '@ant-design/pro-components';
import type { ReactNode } from 'react';

export type RouteKey = 'dashboard' | 'search' | 'progress' | 'maintenance';

const route = {
  path: '/',
  routes: [
    { path: '/dashboard', name: '监控大屏', icon: <AreaChartOutlined /> },
    { path: '/search', name: '日志检索', icon: <FileSearchOutlined /> },
    { path: '/progress', name: '增量进度', icon: <DatabaseOutlined /> },
    { path: '/maintenance', name: '系统维护', icon: <SettingOutlined /> },
  ],
};

const pathMap: Record<string, RouteKey> = {
  '/dashboard': 'dashboard',
  '/search': 'search',
  '/progress': 'progress',
  '/maintenance': 'maintenance',
};

type AppLayoutProps = {
  active: RouteKey;
  onChange: (key: RouteKey) => void;
  children: ReactNode;
};

export function AppLayout({ active, onChange, children }: AppLayoutProps) {
  const activePath = `/${active}`;
  return (
    <ProLayout
      route={route}
      location={{ pathname: activePath }}
      title="NAT Query Service"
      layout="side"
      fixedHeader
      fixSiderbar
      token={{
        sider: {
          colorMenuBackground: '#ffffff',
          colorTextMenu: '#3f4654',
          colorTextMenuSelected: '#0958d9',
          colorBgMenuItemSelected: '#eaf3ff',
        },
        header: {
          colorBgHeader: '#ffffff',
          colorHeaderTitle: '#1f2633',
        },
      }}
      menuItemRender={(item, dom) => {
        const key = item.path ? pathMap[item.path] : undefined;
        if (!key) return dom;
        return (
          <a
            href={item.path}
            onClick={(event) => {
              event.preventDefault();
              onChange(key);
            }}
          >
            {dom}
          </a>
        );
      }}
      actionsRender={() => [<span className="admin-label" key="admin">管理员</span>]}
    >
      <main className="workbench">{children}</main>
    </ProLayout>
  );
}
