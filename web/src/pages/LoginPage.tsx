import React from 'react';
import { LockOutlined, SafetyCertificateOutlined } from '@ant-design/icons';
import { Alert, Button, Card, Form, Input, Typography, message } from 'antd';
import { apiPost } from '../api';

const { Paragraph, Title } = Typography;

type LoginPageProps = {
  onSuccess: () => Promise<void> | void;
};

type LoginResponse = {
  authenticated?: boolean;
  ok?: boolean;
};

export function LoginPage({ onSuccess }: LoginPageProps) {
  const [submitting, setSubmitting] = React.useState(false);
  const [error, setError] = React.useState('');

  const handleFinish = async (values: { password: string }) => {
    setSubmitting(true);
    setError('');
    try {
      const response = await apiPost<LoginResponse>('/api/login', values);
      if (!response.authenticated && !response.ok) {
        throw new Error('登录失败，请检查管理员密码');
      }
      await onSuccess();
      message.success('登录成功');
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : '登录失败');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="login-shell">
      <Card bordered={false} className="login-panel">
        <div className="login-brand">
          <div className="login-brand-mark">
            <SafetyCertificateOutlined />
          </div>
          <div>
            <Title level={3}>FWLOG 运维台</Title>
            <Paragraph type="secondary">
              使用单一管理员密码登录，前端会通过 <code>/api/login</code> 与 <code>/api/session</code> 完成会话校验。
            </Paragraph>
          </div>
        </div>
        {error ? <Alert type="error" showIcon message={error} className="block-alert" /> : null}
        <Form layout="vertical" onFinish={handleFinish} requiredMark={false}>
          <Form.Item
            name="password"
            label="管理员密码"
            rules={[{ required: true, message: '请输入管理员密码' }]}
          >
            <Input.Password
              autoFocus
              prefix={<LockOutlined />}
              placeholder="请输入管理员密码"
              autoComplete="current-password"
            />
          </Form.Item>
          <Button type="primary" htmlType="submit" loading={submitting} block icon={<LockOutlined />}>
            登录
          </Button>
        </Form>
      </Card>
    </div>
  );
}
