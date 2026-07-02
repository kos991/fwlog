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
    } catch (error) {
      setAuthenticated(false);
      if (error instanceof Error) message.warning(error.message);
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
          colorPrimary: '#1677ff',
          borderRadius: 6,
          fontFamily: '"Segoe UI", "PingFang SC", "Microsoft YaHei", system-ui, sans-serif',
        },
      }}
    >
      <App />
    </ConfigProvider>
  </React.StrictMode>,
);
