import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import test from 'node:test';

const resolve = (file: string) => path.resolve(file);
const read = (file: string) => fs.readFileSync(resolve(file), 'utf8');

test('login motion uses GSAP behind a scoped React controller', () => {
  const packageJson = JSON.parse(read('package.json')) as {
    dependencies?: Record<string, string>;
  };
  const hookPath = 'src/animations/useLoginSceneMotion.ts';

  assert.equal(packageJson.dependencies?.gsap, '^3.15.0');
  assert.equal(packageJson.dependencies?.['@gsap/react'], '^2.1.2');
  assert.ok(fs.existsSync(resolve(hookPath)), `${hookPath} must exist`);

  const hook = read(hookPath);
  assert.match(hook, /useGSAP/);
  assert.match(hook, /scope:\s*root/);
  assert.match(hook, /prefers-reduced-motion/);
  assert.match(hook, /visibilitychange/);
  assert.match(hook, /data-login-logo-channel/);
  assert.match(hook, /data-login-logo-particle/);
  assert.match(hook, /data-login-title-char/);
  assert.match(hook, /gsap\.set\('\[data-login-title-char\]'/);
  assert.match(hook, /MotionPathPlugin/);
  assert.match(hook, /data-login-node/);
  assert.match(hook, /data-login-path/);
  assert.match(hook, /data-login-particle/);
});

test('login page separates authentication from the decorative globe scene', () => {
  const scenePath = 'src/components/LoginDataFlowScene.tsx';
  assert.ok(fs.existsSync(resolve(scenePath)), `${scenePath} must exist`);

  const page = read('src/pages/LoginPage.tsx');
  const scene = read(scenePath);
  const logo = read('src/components/BrandLogo.tsx');
  const layout = read('src/layout/AppLayout.tsx');

  assert.match(page, /<LoginDataFlowScene\s*\/>/);
  assert.doesNotMatch(page, /LoginGlobeScene/);
  assert.match(page, /playSuccess/);
  assert.match(page, /playError/);
  assert.match(page, /passwordInputRef/);
  assert.match(page, /<BrandLogo className="login-logo"\s*\/>/);
  assert.match(page, /data-login-title-char/);
  assert.doesNotMatch(page, /data-login-subtitle/);
  assert.doesNotMatch(page, /<Text[^>]*>管理员登录<\/Text>/);
  assert.match(
    page,
    /<section className=\{hasError[\s\S]*?<div className="login-panel-product">[\s\S]*?<BrandLogo className="login-logo"\s*\/>[\s\S]*?<h1 aria-label=\{loginTitle\}>/,
  );
  assert.match(layout, /<BrandLogo className="brand-logo"\s*\/>/);
  assert.doesNotMatch(layout, /SafetyCertificateOutlined/);
  assert.doesNotMatch(page, /className="login-copy"|login-intro|进入控制台|查看入库状态/);
  assert.match(scene, /aria-hidden="true"/);
  assert.match(scene, /<svg/);
  assert.match(scene, /data-login-path/);
  assert.match(scene, /data-login-node/);
  assert.match(scene, /data-login-particle/);
  assert.doesNotMatch(scene, /https?:\/\//);
  assert.equal(logo.match(/data-login-logo-channel/g)?.length, 3);
  assert.equal(logo.match(/data-login-logo-particle/g)?.length, 3);
  assert.match(logo, /brand-mark__hub/);
  assert.doesNotMatch(logo, /shield|check|sweep/i);
  assert.doesNotMatch(scene, /apiPost|\/api\/login/);
});

test('login globe styles preserve the compact panel and responsive fallback', () => {
  const styles = read('src/styles.css');

  assert.match(styles, /@media \(max-width: 900px\)/);
  assert.match(styles, /@media \(prefers-reduced-motion: reduce\)/);
  assert.match(styles, /\.login-data-scene/);
  assert.match(styles, /\.login-panel\.is-error/);
  assert.match(styles, /\.login-panel\s*\{[^}]*grid-column:\s*2;[^}]*border:\s*1px solid var\(--fw-line\);[^}]*background:\s*#fff;/s);
  assert.match(styles, /\.login-panel::before\s*\{[^}]*background:\s*linear-gradient\(90deg/s);
  assert.match(styles, /\.login-panel::after\s*\{[^}]*height:\s*2px;[^}]*animation:\s*login-border-sheen/s);
  assert.match(styles, /@keyframes login-border-sheen/);
  assert.match(styles, /\.login-panel-product\s*\{[^}]*display:\s*grid;[^}]*grid-template-columns:\s*34px minmax\(0, 1fr\);/s);
  assert.match(styles, /--login-content-offset-y:\s*-32px;/);
  assert.match(styles, /\.login-stage\s*\{[^}]*transform:\s*translateY\(var\(--login-content-offset-y\)\);/s);
  assert.match(styles, /\.login-stage\s*\{[^}]*pointer-events:\s*none;/s);
  assert.match(styles, /\.login-panel\s*\{[^}]*pointer-events:\s*auto;/s);
  assert.match(styles, /\.login-panel\s*\{[^}]*max-width:\s*360px;[^}]*padding:\s*22px 24px 24px;/s);
  assert.match(styles, /\.login-data-scene\s*\{[^}]*position:\s*absolute;[^}]*opacity:\s*0\.82;/s);
  assert.match(styles, /\.login-scene-path--glow\s*\{/);
  assert.match(styles, /\.login-scene-particle\s*\{[^}]*stroke:\s*#fff;/s);
  assert.match(styles, /\.login-stage\s*\{[^}]*grid-template-columns:\s*minmax\(0, 1fr\) minmax\(340px, 380px\);/s);
  assert.match(styles, /@media \(max-width: 900px\)[\s\S]*?\.login-data-scene\s*\{[^}]*display:\s*none;/);
  assert.match(styles, /@media \(max-width: 900px\)[\s\S]*?\.login-panel\s*\{[^}]*grid-column:\s*1;/);
});

test('session check runs in the background so the login page renders immediately', () => {
  const main = read('src/main.tsx');
  assert.doesNotMatch(main, /checkingSession/);
  assert.doesNotMatch(main, /Spin fullscreen/);
  assert.match(main, /void refreshSession\(\)/);
  assert.match(main, /if \(!authenticated\) \{[\s\S]*?<LoginPage/);
});
