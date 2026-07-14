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
});

test('login page separates authentication from the decorative data flow scene', () => {
  const scenePath = 'src/components/LoginDataFlowScene.tsx';
  assert.ok(fs.existsSync(resolve(scenePath)), `${scenePath} must exist`);

  const page = read('src/pages/LoginPage.tsx');
  const scene = read(scenePath);

  assert.match(page, /<LoginDataFlowScene\s*\/>/);
  assert.match(page, /playSuccess/);
  assert.match(page, /playError/);
  assert.match(page, /passwordInputRef/);
  assert.match(scene, /aria-hidden="true"/);
  assert.match(scene, /data-login-grid/);
  assert.match(scene, /data-login-node/);
  assert.match(scene, /data-login-path/);
  assert.match(scene, /data-login-particle/);
  for (const label of ['日志源', 'RSyslog 接收', '数据存储', '日志查询']) {
    assert.match(scene, new RegExp(label));
  }
  assert.doesNotMatch(scene, /apiPost|\/api\/login/);
});

test('login styles provide responsive and reduced-motion fallbacks without legacy keyframes', () => {
  const styles = read('src/styles.css');

  assert.match(styles, /@media \(max-width: 900px\)/);
  assert.match(styles, /@media \(prefers-reduced-motion: reduce\)/);
  assert.match(styles, /\.login-data-scene/);
  assert.match(styles, /\.login-panel\.is-error/);
  assert.doesNotMatch(styles, /@keyframes login-flow-/);
});
