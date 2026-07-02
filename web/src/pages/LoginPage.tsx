import React from 'react';
import { LockOutlined, LoginOutlined, SafetyCertificateOutlined } from '@ant-design/icons';
import { Button, Form, Input, Typography, message } from 'antd';
import { apiPost } from '../api';

const { Text } = Typography;

type LoginPageProps = {
  onSuccess: () => void;
};

export function LoginPage({ onSuccess }: LoginPageProps) {
  const [loading, setLoading] = React.useState(false);

  const onFinish = async (values: { password: string }) => {
    try {
      setLoading(true);
      await apiPost('/api/login', values);
      onSuccess();
    } catch (error) {
      message.error(error instanceof Error ? error.message : '登录失败');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="login-shell">
      <div className="login-stage">
        <section className="login-copy" aria-label="NAT 日志控制台">
          <div className="login-brand">
            <span className="login-logo">
              <SafetyCertificateOutlined />
            </span>
            <span>本机管理</span>
          </div>
          <div className="login-title-block">
            <h1>NAT 日志控制台</h1>
            <p>查看入库状态、查询 NAT 日志、维护系统配置。</p>
          </div>
          <svg className="login-flow-art" viewBox="0 0 520 260" role="img" aria-label="日志流转示意">
            <defs>
              <linearGradient id="loginFlowLine" x1="0%" x2="100%" y1="0%" y2="0%">
                <stop offset="0%" stopColor="#16a34a" />
                <stop offset="48%" stopColor="#1677ff" />
                <stop offset="100%" stopColor="#123a6f" />
              </linearGradient>
              <linearGradient id="loginFlowNode" x1="0%" x2="100%" y1="0%" y2="100%">
                <stop offset="0%" stopColor="#ffffff" />
                <stop offset="100%" stopColor="#edf7ff" />
              </linearGradient>
            </defs>
            <path className="login-flow-line" d="M52 126 C132 46 202 190 280 112 S408 48 468 126" />
            <path
              className="login-flow-line login-flow-line-b"
              d="M54 168 C150 122 204 144 270 168 S394 218 468 150"
            />
            <g className="login-flow-node">
              <rect x="32" y="86" width="86" height="64" rx="10" />
              <path d="M54 112h42M54 128h28" />
            </g>
            <g className="login-flow-node login-flow-node-b">
              <rect x="218" y="68" width="92" height="72" rx="10" />
              <path d="M242 100h44M242 118h30" />
            </g>
            <g className="login-flow-node login-flow-node-c">
              <rect x="392" y="92" width="92" height="66" rx="10" />
              <path d="M416 118h42M416 134h28" />
            </g>
            <circle className="login-flow-dot" r="7" />
            <circle className="login-flow-dot login-flow-dot-b" r="6" />
          </svg>
        </section>

        <section className="login-panel" aria-label="管理员登录">
          <div className="login-panel-head">
            <Text type="secondary">管理员登录</Text>
            <h2>进入控制台</h2>
          </div>
          <Form layout="vertical" onFinish={onFinish} requiredMark={false}>
            <Form.Item name="password" label="管理员密码" rules={[{ required: true, message: '请输入管理员密码' }]}>
              <Input.Password
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
