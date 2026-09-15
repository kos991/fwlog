import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import test from 'node:test';

const read = (file: string) => fs.readFileSync(path.resolve(file), 'utf8');

test('网页登录预览异步加载本地点阵地球且不阻塞登录表单', () => {
  const packageJson = JSON.parse(read('package.json')) as { dependencies?: Record<string, string> };
  const main = read('src/main.tsx');
  const scene = read('src/components/LoginGlobeScene.tsx');

  assert.ok(packageJson.dependencies?.three);
  assert.match(main, /get\('preview'\) === 'login'/);
  assert.match(main, /isLoginPreview[\s\S]*<LoginPage/);
  assert.match(scene, /import\('three'\)/);
  assert.match(scene, /globe-land-points\.json/);
  assert.match(scene, /PointsMaterial/);
  assert.match(scene, /BufferGeometry/);
  assert.match(scene, /Math\.min\(window\.devicePixelRatio \|\| 1, 1\.5\)/);
  assert.match(scene, /foreignLocations/);
  assert.match(scene, /createLocationLabel/);
  assert.match(scene, /globe\.rotation\.y/);
  assert.match(scene, /Tokyo|Frankfurt|Los Angeles/);
  assert.match(scene, /aria-hidden="true"/);
});

test('Chrome 插件登录页使用紧凑本地点阵地球', () => {
  const extensionRoot = path.resolve('..', 'chrome-extension');
  const html = fs.readFileSync(path.join(extensionRoot, 'popup.html'), 'utf8');
  const styles = fs.readFileSync(path.join(extensionRoot, 'popup.css'), 'utf8');
  assert.doesNotMatch(html, /login-globe/);
  assert.doesNotMatch(html, /login-globe\.js/);
  assert.doesNotMatch(styles, /login-globe/);
  assert.match(styles, /html, body\s*\{[^}]*width:\s*360px;/s);
  assert.ok(fs.existsSync(path.join(extensionRoot, 'vendor', 'three.module.min.js')));
  assert.ok(fs.existsSync(path.join(extensionRoot, 'globe-land-points.json')));
});
