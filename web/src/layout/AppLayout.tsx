import {
  DashboardOutlined,
  FileSyncOutlined,
  FolderOpenOutlined,
  PoweroffOutlined,
  SearchOutlined,
  SettingOutlined,
  SyncOutlined,
  UserOutlined,
} from '@ant-design/icons';
import { ProLayout } from '@ant-design/pro-components';
import { Button, Tooltip } from 'antd';
import { type ReactNode } from 'react';
import { BrandLogo } from '../components/BrandLogo';

export type RouteKey = 'dashboard' | 'search' | 'reception' | 'organize' | 'maintenance';

const route = {
  path: '/',
  routes: [
    { path: '/dashboard', name: '数据概览', icon: <DashboardOutlined /> },
    { path: '/search', name: '日志查询', icon: <SearchOutlined /> },
    {
      path: '/progress-center',
      name: '日志管理',
      icon: <FileSyncOutlined />,
      routes: [
        { path: '/progress-reception', name: '日志接收', icon: <FolderOpenOutlined /> },
        { path: '/progress/organize', name: '日志整理', icon: <SyncOutlined /> },
      ],
    },
    { path: '/maintenance', name: '系统设置', icon: <SettingOutlined /> },
  ],
};

const pathMap: Record<string, RouteKey> = {
  '/dashboard': 'dashboard',
  '/search': 'search',
  '/progress-reception': 'reception',
  '/progress/organize': 'organize',
  '/maintenance': 'maintenance',
};

const routePathMap: Record<RouteKey, string> = {
  dashboard: '/dashboard',
  search: '/search',
  reception: '/progress-reception',
  organize: '/progress/organize',
  maintenance: '/maintenance',
};

type AppLayoutProps = {
  active: RouteKey;
  onChange: (key: RouteKey) => void;
  onLogout: () => void;
  children: ReactNode;
};

export function AppLayout({ active, onChange, onLogout, children }: AppLayoutProps) {
  return (
    <ProLayout
      route={route}
      location={{ pathname: routePathMap[active] }}
      logo={<BrandLogo className="brand-logo" />}
      title="NAT 日志控制台"
      layout="side"
      fixedHeader
      fixSiderbar
      siderWidth={64}
      collapsed
      collapsedButtonRender={false}
      token={{
        sider: {
          colorMenuBackground: '#ffffff',
          colorTextMenu: '#465568',
          colorTextMenuSelected: '#1d4ed8',
          colorBgMenuItemSelected: '#eef4ff',
        },
        header: {
          colorBgHeader: '#ffffff',
          colorHeaderTitle: '#172033',
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
      menuFooterRender={(props) => {
        const collapsed = props?.collapsed;
        return (
          <div className={collapsed ? 'admin-footer admin-footer-collapsed' : 'admin-footer'} aria-label="管理员">
            <span className="admin-badge"><UserOutlined /></span>
            <span className="admin-name">管理员</span>
            <Tooltip title="退出登录" placement="right">
              <Button
                className="admin-logout"
                type="text"
                icon={<PoweroffOutlined />}
                aria-label="退出登录"
                onClick={onLogout}
              />
            </Tooltip>
          </div>
        );
      }}
    >
      <main className="workbench">{children}</main>
    </ProLayout>
  );
}
