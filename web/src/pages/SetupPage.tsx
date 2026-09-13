import React from 'react';
import { Button, Form, Input, message } from 'antd';
import { LockOutlined, SafetyCertificateOutlined } from '@ant-design/icons';
import { apiPost } from '../api';
import { BrandLogo } from '../components/BrandLogo';

type SetupPageProps = { onComplete: () => void };

export function SetupPage({ onComplete }: SetupPageProps) {
  const [loading, setLoading] = React.useState(false);
  const [visible, setVisible] = React.useState(false);
  const [form] = Form.useForm();
  const submit = async (values: { password: string; confirm_password: string }) => {
    if (values.password !== values.confirm_password) {
      form.setFields([{ name: 'confirm_password', errors: ['两次输入的密码不一致'] }]);
      return;
    }
    setLoading(true);
    try {
      await apiPost('/api/setup', values);
      message.success('管理员创建成功，请登录');
      onComplete();
    } catch (error) {
      message.error(error instanceof Error ? error.message : '初始化失败');
    } finally {
      setLoading(false);
    }
  };
  return (
    <div className="login-shell setup-shell">
      <div className="login-stage">
        <section className="login-panel setup-panel" aria-label="初始化管理员">
          <div className="login-panel-product"><BrandLogo className="login-logo" /><div><h1>初始化管理员</h1><p className="setup-subtitle">首次使用请设置管理员密码</p></div></div>
          <div className="setup-account"><SafetyCertificateOutlined /><span>管理员账号</span><strong>admin</strong></div>
          <Form form={form} className="login-form" layout="vertical" requiredMark={false} onFinish={submit}>
            <Form.Item name="password" label="管理员密码" rules={[{ required: true, message: '请输入管理员密码' }, { min: 6, message: '密码至少需要 6 个字符' }]}>
              <Input.Password prefix={<LockOutlined />} visibilityToggle={{ visible, onVisibleChange: setVisible }} placeholder="请输入管理员密码" autoFocus autoComplete="new-password" />
            </Form.Item>
            <Form.Item name="confirm_password" label="确认密码" dependencies={['password']} rules={[{ required: true, message: '请再次输入密码' }]}>
              <Input.Password prefix={<LockOutlined />} visibilityToggle={{ visible, onVisibleChange: setVisible }} placeholder="请再次输入密码" autoComplete="new-password" />
            </Form.Item>
            <Button type="primary" htmlType="submit" block loading={loading}>创建管理员</Button>
          </Form>
          <div className="login-panel-foot">密码保存后仅管理员本人可修改</div>
        </section>
      </div>
    </div>
  );
}
