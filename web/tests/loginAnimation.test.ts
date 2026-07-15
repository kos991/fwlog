import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import test from 'node:test';

const resolve = (file: string) => path.resolve(file);
const read = (file: string) => fs.readFileSync(resolve(file), 'utf8');

test('login scene uses Anime.js V4 behind a scoped motion controller', () => {
  const packageJson = JSON.parse(read('package.json')) as {
    dependencies?: Record<string, string>;
  };
  const hookPath = 'src/animations/useLoginSceneMotion.ts';

  assert.equal(packageJson.dependencies?.animejs, '^4.5.0');
  assert.ok(fs.existsSync(resolve(hookPath)), `${hookPath} must exist`);

  const hook = read(hookPath);
  for (const api of ['animate', 'createScope', 'createTimeline', 'stagger']) {
    assert.match(hook, new RegExp(`\\b${api}\\b`));
  }
  assert.match(hook, /import\('animejs'\)/);
  assert.match(hook, /scope\.revert\(\)/);
  assert.match(hook, /prefers-reduced-motion/);
  assert.match(hook, /max-width:\s*900px/);
  assert.match(hook, /visibilitychange/);
  assert.doesNotMatch(hook, /'\[data-login-node\]'[\s\S]{0,220}translateY/);
  assert.match(hook, /\[data-login-node-icon\]\s*>\s*\*/);
  assert.match(hook, /data-login-logo-channel/);
  assert.match(hook, /data-login-logo-particle/);
  assert.doesNotMatch(hook, /data-login-telemetry-progress/);
  assert.match(hook, /data-login-title-char/);
  assert.match(hook, /anime\.set\('\[data-login-title-char\]'/);
  assert.doesNotMatch(hook, /createMotionPath\('#login-route-query'\)|data-route="query"/);
  assert.doesNotMatch(hook, /mouseenter|mouseleave|data-login-runtime/);
  assert.doesNotMatch(hook, /data-login-logo-shield|data-login-logo-check|data-login-logo-sweep/);
});

test('login page separates authentication from the decorative data flow scene', () => {
  const scenePath = 'src/components/LoginDataFlowScene.tsx';
  assert.ok(fs.existsSync(resolve(scenePath)), `${scenePath} must exist`);

  const page = read('src/pages/LoginPage.tsx');
  const scene = read(scenePath);
  const logo = read('src/components/BrandLogo.tsx');
  const layout = read('src/layout/AppLayout.tsx');

  assert.match(page, /<LoginDataFlowScene\s*\/>/);
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
  assert.match(scene, /data-login-grid/);
  assert.match(scene, /data-login-node/);
  assert.equal(scene.match(/data-login-node-icon/g)?.length, 5);
  assert.match(scene, /transform="translate\(718 344\)"/);
  assert.doesNotMatch(scene, /RuntimePulse|data-login-runtime/);
  assert.equal(scene.match(/data-login-path/g)?.length, 2);
  assert.match(scene, /M204 410 C264 410 286 278 354 278 H520 C600 278 632 400 718 400 H884 C954 400 986 298 1070 298/);
  assert.match(scene, /M437 334 C437 448 548 526 718 526/);
  assert.doesNotMatch(scene, /login-route-query|login-scene-path--quiet|data-route="query"/);
  assert.doesNotMatch(scene, /login-scene-path-flow/);
  assert.match(scene, /data-login-particle/);
  for (const label of ['日志源', '日志接收', '数据存储', '日志查询', '网络日志', '日志数据']) {
    assert.match(scene, new RegExp(label));
  }
  assert.match(scene, /login-scene-telemetry/);
  assert.match(scene, /日志处理流程/);
  assert.match(scene, /采集 · 接收 · 入库 · 查询/);
  for (const forbidden of ['RSyslog', 'UDP / TCP', 'ClickHouse', 'INGEST PIPELINE', 'SOURCE  ·  RECEIVE']) {
    assert.doesNotMatch(scene, new RegExp(forbidden));
  }
  assert.doesNotMatch(scene, /data-login-status-step|data-login-telemetry-progress/);
  assert.equal(scene.match(/<circle\s+data-login-particle/g)?.length, 3);
  assert.equal(logo.match(/data-login-logo-channel/g)?.length, 3);
  assert.equal(logo.match(/data-login-logo-particle/g)?.length, 3);
  assert.match(logo, /brand-mark__hub/);
  assert.doesNotMatch(logo, /shield|check|sweep/i);
  const particles = scene.match(/<circle\s+data-login-particle[\s\S]*?\/>/g) ?? [];
  assert.ok(particles.length > 0);
  for (const particle of particles) {
    assert.doesNotMatch(particle, /\s(?:cx|cy)=/);
  }
  assert.doesNotMatch(scene, /apiPost|\/api\/login/);
});

test('login styles provide responsive and reduced-motion fallbacks without legacy keyframes', () => {
  const styles = read('src/styles.css');

  assert.match(styles, /@media \(max-width: 900px\)/);
  assert.match(styles, /@media \(prefers-reduced-motion: reduce\)/);
  assert.match(styles, /\.login-data-scene/);
  assert.match(styles, /\.login-panel\.is-error/);
  assert.match(styles, /\.login-panel\s*\{[^}]*grid-column:\s*2;[^}]*background:\s*#fff;/s);
  assert.match(styles, /\.login-panel-product\s*\{[^}]*display:\s*grid;[^}]*grid-template-columns:\s*34px minmax\(0, 1fr\);/s);
  assert.match(styles, /--login-content-offset-y:\s*-32px;/);
  assert.match(styles, /\.login-data-scene\s*\{[^}]*transform:\s*translateY\(var\(--login-content-offset-y\)\);/s);
  assert.match(styles, /\.login-stage\s*\{[^}]*transform:\s*translateY\(var\(--login-content-offset-y\)\);/s);
  assert.match(styles, /\.login-stage\s*\{[^}]*pointer-events:\s*none;/s);
  assert.match(styles, /\.login-panel\s*\{[^}]*pointer-events:\s*auto;/s);
  assert.match(styles, /\.login-panel\s*\{[^}]*padding:\s*32px 32px 40px;/s);
  assert.doesNotMatch(styles, /\.login-scene-runtime/);
  assert.match(styles, /\.login-scene-telemetry__label/);
  assert.doesNotMatch(styles, /login-status-pulse|login-scene-path-flow|login-route-flow/);
  assert.doesNotMatch(styles, /\.login-scene-path--quiet|\.login-scene-particle--muted/);
  assert.match(styles, /\.login-scene-route-labels\s*\{[^}]*fill:\s*#64748b;[^}]*font-size:\s*15px;[^}]*font-weight:\s*650;/s);
  assert.match(styles, /\.login-scene-grid\s*\{[^}]*fill:\s*transparent;/s);
  assert.match(styles, /@media \(max-width: 900px\)[\s\S]*?\.login-data-scene\s*\{[^}]*display:\s*none;/);
  assert.match(styles, /@media \(max-width: 900px\)[\s\S]*?\.login-panel\s*\{[^}]*grid-column:\s*1;/);
  assert.doesNotMatch(styles, /@keyframes login-flow-/);
});
