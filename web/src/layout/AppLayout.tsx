import {
  DashboardOutlined,
  FileSyncOutlined,
  PoweroffOutlined,
  SafetyCertificateOutlined,
  SearchOutlined,
  SettingOutlined,
  UserOutlined,
} from '@ant-design/icons';
import { ProLayout } from '@ant-design/pro-components';
import { Button, Tooltip } from 'antd';
import { useEffect, useState, type ReactNode } from 'react';

export type RouteKey = 'dashboard' | 'search' | 'progress' | 'maintenance';

const route = {
  path: '/',
  routes: [
    { path: '/dashboard', name: '数据概览', icon: <DashboardOutlined /> },
    { path: '/search', name: '日志查询', icon: <SearchOutlined /> },
    { path: '/progress', name: '入库进度', icon: <FileSyncOutlined /> },
    { path: '/maintenance', name: '系统设置', icon: <SettingOutlined /> },
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
  onLogout: () => void;
  children: ReactNode;
};

export function AppLayout({ active, onChange, onLogout, children }: AppLayoutProps) {
  const [collapsed, setCollapsed] = useState(true);

  useEffect(() => {
    const collapseOnce = () => {
      if (document.querySelector('.ant-layout-sider-collapsed')) return;
      document.querySelector<HTMLElement>('.ant-pro-sider-collapsed-button')?.click();
    };
    const timers = [80, 240, 520].map((delay) => window.setTimeout(collapseOnce, delay));
    return () => timers.forEach((timer) => window.clearTimeout(timer));
  }, []);

  const keepCollapsed = () => {
    setCollapsed(true);
    window.setTimeout(() => {
      if (document.querySelector('.ant-layout-sider-collapsed')) return;
      document.querySelector<HTMLElement>('.ant-pro-sider-collapsed-button')?.click();
    }, 80);
  };

  return (
    <ProLayout
      route={route}
      location={{ pathname: `/${active}` }}
      logo={<span className="brand-logo"><SafetyCertificateOutlined /></span>}
      title="NAT 日志控制台"
      layout="side"
      fixedHeader
      fixSiderbar
      collapsed={collapsed}
      onCollapse={setCollapsed}
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
              keepCollapsed();
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
