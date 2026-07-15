import React from 'react';
import { LockOutlined, LoginOutlined } from '@ant-design/icons';
import { Button, Form, Input, message, type InputRef } from 'antd';
import { apiPost } from '../api';
import { useLoginSceneMotion } from '../animations/useLoginSceneMotion';
import { BrandLogo } from '../components/BrandLogo';
import { LoginDataFlowScene } from '../components/LoginDataFlowScene';

const loginTitle = 'NAT 日志控制台';

type LoginPageProps = {
  onSuccess: () => void;
};

export function LoginPage({ onSuccess }: LoginPageProps) {
  const shellRef = React.useRef<HTMLDivElement>(null);
  const passwordInputRef = React.useRef<InputRef>(null);
  const { playSuccess, playError } = useLoginSceneMotion(shellRef);
  const [loading, setLoading] = React.useState(false);
  const [hasError, setHasError] = React.useState(false);

  const onFinish = async (values: { password: string }) => {
    if (loading) return;

    try {
      setLoading(true);
      setHasError(false);
      await apiPost('/api/login', values);
      await playSuccess();
      onSuccess();
    } catch (error) {
      setHasError(true);
      message.error(error instanceof Error ? error.message : '登录失败');
      await playError();
      passwordInputRef.current?.focus();
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
          <Form layout="vertical" onFinish={onFinish} requiredMark={false}>
            <Form.Item name="password" label="管理员密码" rules={[{ required: true, message: '请输入管理员密码' }]}>
              <Input.Password
                ref={passwordInputRef}
                className="login-password"
                prefix={<LockOutlined />}
                placeholder="请输入管理员密码"
                autoFocus
              />
            </Form.Item>
            <Button
              className="login-submit"
              type="primary"
              htmlType="submit"
              loading={loading}
              block
              icon={<LoginOutlined />}
            >
              登录
            </Button>
          </Form>
          <div className="login-panel-foot">仅限授权管理员访问</div>
        </section>
      </div>
    </div>
  );
}
