import React from 'react';
import { EyeInvisibleOutlined, EyeOutlined, LockOutlined, LoginOutlined, SafetyCertificateOutlined } from '@ant-design/icons';
import { Button, Form, Input, message, type InputRef } from 'antd';
import { apiPost } from '../api';
import { useLoginSceneMotion } from '../animations/useLoginSceneMotion';
import { BrandLogo } from '../components/BrandLogo';
import { LoginDataFlowScene } from '../components/LoginDataFlowScene';

const loginTitle = 'NAT 日志控制台';

type LoginPageProps = {
  onSuccess: () => void;
};

type LoginMode = 'password' | 'mfa';

export function LoginPage({ onSuccess }: LoginPageProps) {
  const previewMfa = React.useMemo(() => new URLSearchParams(window.location.search).get('mfa') === '1', []);
  const shellRef = React.useRef<HTMLDivElement>(null);
  const passwordInputRef = React.useRef<InputRef>(null);
  const mfaInputRef = React.useRef<InputRef>(null);
  const { playSuccess, playError } = useLoginSceneMotion(shellRef);
  const [loading, setLoading] = React.useState(false);
  const [hasError, setHasError] = React.useState(false);
  const [passwordVisible, setPasswordVisible] = React.useState(false);
  const [mfaChallenge, setMfaChallenge] = React.useState('');
  const [useRecoveryCode, setUseRecoveryCode] = React.useState(false);
  const [loginMode, setLoginMode] = React.useState<LoginMode>(() => previewMfa ? 'mfa' : 'password');
  const [form] = Form.useForm();

  const onFinish = async (values: { password?: string; mfa_code?: string }) => {
    if (loading) return;

    try {
      setLoading(true);
      setHasError(false);
      const response = await apiPost<{ authenticated?: boolean; mfa_required?: boolean; challenge?: string }>('/api/login', mfaChallenge
        ? { challenge: mfaChallenge, mfa_code: values.mfa_code, use_recovery_code: useRecoveryCode }
        : loginMode === 'mfa'
          ? { mode: 'mfa', mfa_code: values.mfa_code, use_recovery_code: useRecoveryCode }
          : { mode: 'password', password: values.password });
      if (response.mfa_required && response.challenge) {
        setMfaChallenge(response.challenge);
        window.setTimeout(() => mfaInputRef.current?.focus(), 0);
        return;
      }
      await playSuccess();
      onSuccess();
    } catch (error) {
      setHasError(true);
      message.error(error instanceof Error ? error.message : '登录失败');
      await playError();
      ((mfaChallenge || loginMode === 'mfa') ? mfaInputRef.current : passwordInputRef.current)?.focus();
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="login-shell" ref={shellRef}>
      <LoginDataFlowScene />
      <div className="login-stage">
        <section className={hasError ? 'login-panel is-error' : 'login-panel'} aria-label="管理员登录">
          <div className="login-panel-product">
            <BrandLogo className="login-logo" />
            <div>
              <h1 aria-label={loginTitle}>
                {Array.from(loginTitle).map((character, index) => (
                  <span data-login-title-char aria-hidden="true" key={`${character}-${index}`}>
                    {character === ' ' ? '\u00a0' : character}
                  </span>
                ))}
              </h1>
            </div>
          </div>
          {!mfaChallenge && (
            <div className="login-mode-switch" role="tablist" aria-label="登录方式">
              <button
                type="button"
                role="tab"
                aria-selected={loginMode === 'password'}
                className={loginMode === 'password' ? 'is-active' : ''}
                onClick={() => {
                  setLoginMode('password');
                  form.resetFields();
                }}
              >
                密码登录
              </button>
              <button
                type="button"
                role="tab"
                aria-selected={loginMode === 'mfa'}
                className={loginMode === 'mfa' ? 'is-active' : ''}
                onClick={() => {
                  setLoginMode('mfa');
                  form.resetFields();
                }}
              >
                MFA 登录
              </button>
            </div>
          )}
          <Form form={form} className="login-form" layout="vertical" onFinish={onFinish} requiredMark={false}>
            <div className="login-form-area">
              {!mfaChallenge && loginMode === 'password' && <Form.Item name="password" label="管理员密码" rules={[{ required: true, message: '请输入管理员密码' }]}>
                <Input.Password
                  ref={passwordInputRef}
                  className="login-password"
                  prefix={<LockOutlined />}
                  placeholder="请输入管理员密码"
                  autoFocus
                  autoComplete="current-password"
                  visibilityToggle={{ visible: passwordVisible, onVisibleChange: setPasswordVisible }}
                  iconRender={(visible) => visible ? <EyeOutlined /> : <EyeInvisibleOutlined />}
                />
              </Form.Item>}
              {(mfaChallenge || loginMode === 'mfa') && <Form.Item
                name="mfa_code"
                label={useRecoveryCode ? '恢复码' : '身份验证器验证码'}
                rules={[{ required: true, message: useRecoveryCode ? '请输入恢复码' : '请输入 6 位验证码' }]}
              >
                <Input
                  ref={mfaInputRef}
                  className="login-password"
                  prefix={<SafetyCertificateOutlined />}
                  inputMode={useRecoveryCode ? 'text' : 'numeric'}
                  maxLength={useRecoveryCode ? 10 : 6}
                  placeholder={useRecoveryCode ? '请输入一次性恢复码' : '请输入 6 位验证码'}
                  autoFocus
                  autoComplete={useRecoveryCode ? 'off' : 'one-time-code'}
                />
              </Form.Item>}
              <Button
                className="login-submit"
                type="primary"
                htmlType="submit"
                loading={loading}
                block
                icon={<LoginOutlined />}
              >
                {mfaChallenge ? '验证并登录' : loginMode === 'mfa' ? '继续验证' : '登录'}
              </Button>
              {(mfaChallenge || loginMode === 'mfa') && (
                <div className="login-mfa-actions">
                  <Button
                    type="link"
                    className="login-mfa-switch"
                    onClick={() => {
                      setUseRecoveryCode((value) => !value);
                      form.resetFields(['mfa_code']);
                    }}
                  >
                    {useRecoveryCode ? '使用身份验证器验证码' : '使用恢复码'}
                  </Button>
                  {mfaChallenge && <Button
                    type="link"
                    className="login-mfa-back"
                    onClick={() => {
                      setMfaChallenge('');
                      setLoginMode('password');
                      setUseRecoveryCode(false);
                      form.resetFields();
                      window.setTimeout(() => passwordInputRef.current?.focus(), 0);
                    }}
                  >
                    返回密码登录
                  </Button>}
                </div>
              )}
            </div>
          </Form>
          <div className="login-panel-foot">仅限授权管理员访问</div>
        </section>
      </div>
    </div>
  );
}
