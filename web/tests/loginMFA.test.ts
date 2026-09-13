import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import test from 'node:test';

const read = (file: string) => fs.readFileSync(path.resolve(file), 'utf8');

test('login page exposes password visibility toggle and MFA code step', () => {
  const page = read('src/pages/LoginPage.tsx');
  assert.match(page, /visibilityToggle/);
  assert.match(page, /twoFactorCode|totp|mfa_code/);
  assert.match(page, /EyeOutlined|EyeInvisibleOutlined/);
  assert.match(page, /mfa_required/);
  assert.match(page, /get\('mfa'\) === '1'/);
  assert.match(page, /密码登录/);
  assert.match(page, /MFA 登录/);
  assert.match(page, /role="tablist"/);
  assert.match(page, /loginMode === 'mfa'/);
  assert.match(page, /mode: 'mfa'/);
  assert.match(page, /mode: 'password'/);
  assert.match(page, /className="login-form-area"/);
  assert.match(page, /autoComplete="current-password"/);
  assert.match(page, /autoComplete=\{useRecoveryCode \? 'off' : 'one-time-code'\}/);
  assert.match(page, /返回密码登录/);
});

test('login styles reserve stable MFA action space and mobile layout', () => {
  const styles = read('src/styles.css');
  assert.match(styles, /\.login-form-area\s*\{[^}]*min-height:\s*112px;/s);
  assert.match(styles, /\.login-mfa-actions\s*\{[^}]*display:\s*flex;/s);
  assert.match(styles, /\.login-mode-switch\s*\{[^}]*grid-template-columns:\s*1fr 1fr;/s);
  assert.match(styles, /@media \(max-width: 900px\)[\s\S]*?\.login-stage\s*\{[^}]*transform:\s*none;/s);
});

test('MFA setup renders a scannable authenticator QR code', () => {
  const page = read('src/pages/SystemMaintenancePage.tsx');
  const styles = read('src/styles.css');
  assert.match(page, /QRCode\.toDataURL\(response\.otpauth_uri/);
  assert.match(page, /alt="MFA 身份验证器二维码"/);
  assert.match(page, /className="mfa-qr-code"/);
  assert.match(page, /将此密钥添加到身份验证器/);
  assert.doesNotMatch(page, /或使用 URI/);
  assert.match(styles, /\.mfa-setup-content\s*\{/);
  assert.match(styles, /\.mfa-qr-code\s*\{[\s\S]*width:\s*188px;/);
});
