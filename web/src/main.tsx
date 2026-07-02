import React from 'react';
import ReactDOM from 'react-dom/client';
import { ConfigProvider, Spin, message } from 'antd';
import zhCN from 'antd/locale/zh_CN';
import { AppLayout, type RouteKey } from './layout/AppLayout';
import { LoginPage } from './pages/LoginPage';
import { HealthDashboard } from './pages/HealthDashboard';
import { LogSearchPage } from './pages/LogSearchPage';
import { IncrementalProgressPage } from './pages/IncrementalProgressPage';
import { SystemMaintenancePage } from './pages/SystemMaintenancePage';
import { apiGet, apiPost } from './api';
import './styles.css';

type SessionResponse = {
  authenticated?: boolean;
  ok?: boolean;
};

function App() {
  const [active, setActive] = React.useState<RouteKey>('dashboard');
  const [authenticated, setAuthenticated] = React.useState(false);
  const [checkingSession, setCheckingSession] = React.useState(true);

  const refreshSession = React.useCallback(async () => {
    try {
      const session = await apiGet<SessionResponse>('/api/session');
      setAuthenticated(Boolean(session.authenticated ?? session.ok));
    } catch {
      setAuthenticated(false);
    } finally {
      setCheckingSession(false);
    }
  }, []);

  React.useEffect(() => {
    void refreshSession();
  }, [refreshSession]);

  const logout = React.useCallback(async () => {
    try {
      await apiPost('/api/logout');
    } catch (error) {
      if (error instanceof Error) message.warning(error.message);
    } finally {
      setAuthenticated(false);
    }
  }, []);

  if (checkingSession) {
    return <Spin fullscreen tip="正在检查登录状态" />;
  }

  if (!authenticated) {
    return <LoginPage onSuccess={refreshSession} />;
  }

  return (
    <AppLayout active={active} onChange={setActive} onLogout={logout}>
      {active === 'dashboard' && <HealthDashboard onOpenProgress={() => setActive('progress')} />}
      {active === 'search' && <LogSearchPage onOpenProgress={() => setActive('progress')} />}
      {active === 'progress' && <IncrementalProgressPage />}
      {active === 'maintenance' && <SystemMaintenancePage onRequireLogin={() => setAuthenticated(false)} />}
    </AppLayout>
  );
}

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <ConfigProvider
      locale={zhCN}
      theme={{
        token: {
          colorPrimary: '#2563eb',
          colorInfo: '#2563eb',
          colorSuccess: '#16a34a',
          colorWarning: '#ca8a04',
          colorError: '#dc2626',
          colorText: '#172033',
          colorTextSecondary: '#627084',
          colorBorder: '#dfe7f1',
          colorBgLayout: '#f3f6fa',
          colorBgContainer: '#ffffff',
          borderRadius: 7,
          fontSize: 13,
          controlHeight: 32,
          fontFamily: '"Segoe UI", "PingFang SC", "Microsoft YaHei", system-ui, sans-serif',
        },
        components: {
          Button: {
            borderRadius: 7,
            controlHeight: 32,
            fontWeight: 600,
          },
          Input: {
            borderRadius: 7,
            controlHeight: 32,
          },
          DatePicker: {
            borderRadius: 7,
            controlHeight: 32,
          },
          Select: {
            borderRadius: 7,
            controlHeight: 32,
          },
          Table: {
            headerBg: '#f7f9fc',
            headerColor: '#526071',
            cellPaddingBlock: 9,
            cellPaddingInline: 8,
          },
          Tabs: {
            itemSelectedColor: '#1d4ed8',
            inkBarColor: '#2563eb',
          },
        },
      }}
    >
      <App />
    </ConfigProvider>
  </React.StrictMode>,
);
