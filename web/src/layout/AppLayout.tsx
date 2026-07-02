import {
  LogoutOutlined,
  SafetyCertificateOutlined,
  SearchOutlined,
  SettingOutlined,
  SyncOutlined,
  FundProjectionScreenOutlined,
} from '@ant-design/icons';
import { ProLayout } from '@ant-design/pro-components';
import { Button, Tooltip } from 'antd';
import { useEffect, useRef, type ReactNode } from 'react';

export type RouteKey = 'dashboard' | 'search' | 'progress' | 'maintenance';

const route = {
  path: '/',
  routes: [
    { path: '/dashboard', name: '数据概览', icon: <FundProjectionScreenOutlined /> },
    { path: '/search', name: '日志查询', icon: <SearchOutlined /> },
    { path: '/progress', name: '入库进度', icon: <SyncOutlined /> },
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
  const collapsedOnce = useRef(false);

  useEffect(() => {
    let attempts = 0;
    const timer = window.setInterval(() => {
      if (collapsedOnce.current) return;
      attempts += 1;
      if (document.querySelector('.ant-layout-sider-collapsed')) {
        collapsedOnce.current = true;
        window.clearInterval(timer);
        return;
      }
      const button = document.querySelector<HTMLElement>('.ant-pro-sider-collapsed-button');
      if (button) {
        button.click();
        collapsedOnce.current = true;
        window.clearInterval(timer);
        return;
      }
      if (attempts > 20) {
        window.clearInterval(timer);
      }
    }, 50);
    return () => window.clearInterval(timer);
  }, []);

  return (
    <ProLayout
      route={route}
      location={{ pathname: `/${active}` }}
      logo={<span className="brand-logo"><SafetyCertificateOutlined /></span>}
      title="NAT 日志控制台"
      layout="side"
      fixedHeader
      fixSiderbar
      defaultCollapsed
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
      menuFooterRender={(props) => {
        const collapsed = props?.collapsed;
        return (
          <div className={collapsed ? 'admin-footer admin-footer-collapsed' : 'admin-footer'} aria-label="管理员">
            <Tooltip title="退出登录" placement="right">
              <Button
                className="admin-logout"
                type="text"
                icon={<LogoutOutlined />}
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
