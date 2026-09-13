import React from 'react';
import ReactDOM from 'react-dom/client';
import { ConfigProvider, message } from 'antd';
import zhCN from 'antd/locale/zh_CN';
import { AppLayout, type RouteKey } from './layout/AppLayout';
import { LoginPage } from './pages/LoginPage';
import { SetupPage } from './pages/SetupPage';
import { HealthDashboard } from './pages/HealthDashboard';
import { LogSearchPage } from './pages/LogSearchPage';
import { LogReceptionPage } from './pages/LogReceptionPage';
import { SystemMaintenancePage } from './pages/SystemMaintenancePage';
import { UpgradePreviewPage } from './pages/UpgradePreviewPage';
import { apiGet, apiPost, setOnUnauthorized } from './api';
import './styles.css';

const AccessTrendPreview = React.lazy(async () => {
  const module = await import('./pages/AccessTrendPreview');
  return { default: module.AccessTrendPreview };
});

type SessionResponse = {
  authenticated?: boolean;
  ok?: boolean;
};
type SetupStatus = { required: boolean; username?: string };

function App() {
  const [active, setActive] = React.useState<RouteKey>('dashboard');
  const [authenticated, setAuthenticated] = React.useState(false);
  const [setupRequired, setSetupRequired] = React.useState(false);
  const [setupError, setSetupError] = React.useState('');
  const isUpgradePreview = new URLSearchParams(window.location.search).get('preview') === 'upgrade';
  const isAccessPreview = new URLSearchParams(window.location.search).get('preview') === 'access';
  const isLoginPreview = new URLSearchParams(window.location.search).get('preview') === 'login';

  const refreshSession = React.useCallback(async () => {
    try {
      const session = await apiGet<SessionResponse>('/api/session');
      setAuthenticated(Boolean(session.authenticated ?? session.ok));
    } catch {
      setAuthenticated(false);
    }
  }, []);

  const refreshStartup = React.useCallback(async () => {
    try {
      const [setup, session] = await Promise.all([apiGet<SetupStatus>('/api/setup/status'), apiGet<SessionResponse>('/api/session')]);
      setSetupRequired(Boolean(setup.required));
      setAuthenticated(Boolean(session.authenticated ?? session.ok));
      setSetupError('');
    } catch (error) {
      setSetupError(error instanceof Error ? error.message : '无法读取系统状态');
    }
  }, []);

  React.useEffect(() => {
    setOnUnauthorized(() => setAuthenticated(false));
    return () => setOnUnauthorized(null);
  }, []);

  React.useEffect(() => {
    if (isUpgradePreview || isLoginPreview) return;
    void refreshStartup();
  }, [isLoginPreview, isUpgradePreview, refreshStartup]);

  const logout = React.useCallback(async () => {
    try {
      await apiPost('/api/logout');
    } catch (error) {
      if (error instanceof Error) message.warning(error.message);
    } finally {
      setAuthenticated(false);
    }
  }, []);

  if (isUpgradePreview) {
    return <UpgradePreviewPage />;
  }

  if (isAccessPreview) {
    return <React.Suspense fallback={<div className="app-loading" />}> <AccessTrendPreview /> </React.Suspense>;
  }

  if (isLoginPreview) {
    return <LoginPage onSuccess={() => undefined} />;
  }

  if (setupError) return <div className="app-loading setup-startup-error">{setupError}</div>;
  if (setupRequired) return <SetupPage onComplete={() => { setSetupRequired(false); void refreshSession(); }} />;

  if (!authenticated) {
    return <LoginPage onSuccess={refreshSession} />;
  }

  return (
    <AppLayout active={active} onChange={setActive} onLogout={logout}>
      {active === 'dashboard' && <HealthDashboard onOpenProgress={() => setActive('reception')} />}
      {active === 'search' && <LogSearchPage onOpenProgress={() => setActive('reception')} />}
      {active === 'reception' && <LogReceptionPage onRequireLogin={() => setAuthenticated(false)} />}
      {active === 'organize' && <SystemMaintenancePage scope="organize" onRequireLogin={() => setAuthenticated(false)} />}
      {active === 'maintenance' && <SystemMaintenancePage scope="system" onRequireLogin={() => setAuthenticated(false)} />}
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
