# 登录页 Anime.js 数据流动效实施计划

> **执行约束：**按任务逐项实施，每项遵循测试先行；本会话使用 `executing-plans` 内联执行，不启用子代理。

**目标：**将现有双卡片登录页改造成全屏浅色运维数据拓扑场景，并用 Anime.js V4 统一管理桌面入场、循环、成功和失败反馈。

**架构：**`LoginPage` 只负责认证状态和调用动效控制器；`LoginDataFlowScene` 只负责确定性的 SVG 场景；`useLoginSceneMotion` 负责 Anime.js 动态导入、作用域、媒体条件、动画生命周期和安全降级。样式继续集中在现有 `styles.css`，认证 API、Session 与 Cookie 行为保持不变。

**技术栈：**React 18、TypeScript 5、Ant Design 5、Anime.js 4.5.0、Vite 5、Node 内置测试运行器。

## 全局约束

- 仅新增 `animejs@4.5.0`，使用 `animate`、`createScope`、`createTimeline`、`stagger` 命名 API。
- 不修改 `/api/login`、`/api/session`、Cookie 或后端密码校验逻辑。
- 桌面数据流自动运行，不增加鼠标跟随或节点交互。
- `max-width: 900px` 时关闭背景循环；`prefers-reduced-motion: reduce` 时关闭循环、位移和路径绘制。
- Anime.js 导入或动画执行失败时，登录成功必须继续调用 `onSuccess()`，登录失败必须继续提示并恢复焦点。
- 循环只使用 `transform`、`opacity`、SVG 描边和路径进度，不动画布局尺寸。
- 验收视口固定为桌面 `1440x1000` 和移动端 `390x844`。

---

### 任务 1：锁定依赖和登录动效契约

**文件：**

- 修改：`web/package.json`
- 修改：`web/package-lock.json`
- 新增测试：`web/tests/loginAnimation.test.ts`

**接口：**

- 产出：`animejs` V4 依赖；后续任务必须提供 `LoginDataFlowScene`、`useLoginSceneMotion` 及其约定选择器。
- 依赖：现有 `node --test "tests/*.test.ts"` 测试方式。

- [ ] **步骤 1：编写失败测试**

新增源码契约测试，明确依赖、文件边界、降级分支和旧 CSS 动画移除要求：

```ts
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import test from 'node:test';

const read = (file: string) => fs.readFileSync(path.resolve(file), 'utf8');

test('login scene uses Anime.js V4 behind a scoped motion controller', () => {
  const pkg = JSON.parse(read('package.json'));
  const hook = read('src/animations/useLoginSceneMotion.ts');

  assert.equal(pkg.dependencies.animejs, '^4.5.0');
  for (const api of ['animate', 'createScope', 'createTimeline', 'stagger']) {
    assert.match(hook, new RegExp(`\\b${api}\\b`));
  }
  assert.match(hook, /import\('animejs'\)/);
  assert.match(hook, /scope\.revert\(\)/);
  assert.match(hook, /prefers-reduced-motion/);
  assert.match(hook, /max-width:\s*900px/);
});

test('login page separates authentication from the decorative data flow scene', () => {
  const page = read('src/pages/LoginPage.tsx');
  const scene = read('src/components/LoginDataFlowScene.tsx');

  assert.match(page, /<LoginDataFlowScene\s*\/>/);
  assert.match(page, /playSuccess/);
  assert.match(page, /playError/);
  assert.match(page, /passwordInputRef/);
  assert.match(scene, /aria-hidden="true"/);
  assert.match(scene, /data-login-path/);
  assert.match(scene, /data-login-particle/);
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
```

- [ ] **步骤 2：确认测试红灯**

执行：

```powershell
cd web
npm test -- --test-name-pattern="login"
```

预期：失败，原因是 `src/animations/useLoginSceneMotion.ts` 或 `src/components/LoginDataFlowScene.tsx` 不存在，且 `animejs` 尚未声明。

- [ ] **步骤 3：安装固定依赖**

执行：

```powershell
cd web
npm install animejs@4.5.0
```

预期：`package.json` 出现 `"animejs": "^4.5.0"`，锁文件解析到 `4.5.0`。

- [ ] **步骤 4：提交依赖和红灯契约**

```powershell
git add web/package.json web/package-lock.json web/tests/loginAnimation.test.ts
git commit -m "test: define animated login contracts"
```

### 任务 2：实现确定性的 SVG 数据拓扑场景

**文件：**

- 新增：`web/src/components/LoginDataFlowScene.tsx`
- 测试：`web/tests/loginAnimation.test.ts`

**接口：**

- 产出：`export function LoginDataFlowScene(): JSX.Element`。
- 选择器：`[data-login-grid]`、`[data-login-node]`、`[data-login-path]`、`[data-login-particle]`。
- 依赖：无认证状态、无 API、无动画库。

- [ ] **步骤 1：实现纯展示组件**

组件根节点使用以下结构，节点、路径和粒子全部为固定数量：

```tsx
export function LoginDataFlowScene() {
  return (
    <div className="login-data-scene" aria-hidden="true">
      <svg className="login-data-scene__canvas" viewBox="0 0 1200 760" preserveAspectRatio="xMidYMid slice">
        <g data-login-grid className="login-scene-grid">
          <path d="M0 120H1200M0 240H1200M0 360H1200M0 480H1200M0 600H1200" />
          <path d="M120 0V760M300 0V760M480 0V760M660 0V760M840 0V760M1020 0V760" />
        </g>
        <g className="login-scene-links">
          <path data-login-path id="login-route-main" d="M120 376 C260 376 280 250 430 250 S610 376 740 376 S920 250 1080 250" />
          <path data-login-path id="login-route-branch" d="M430 250 C560 250 590 520 740 520 S910 376 1080 376" />
        </g>
        <g className="login-scene-nodes">
          <g data-login-node data-kind="device" transform="translate(72 330)">
            <rect width="128" height="92" rx="8" />
            <text x="64" y="55" textAnchor="middle">日志源</text>
          </g>
          <g data-login-node data-kind="receiver" transform="translate(366 204)">
            <rect width="148" height="92" rx="8" />
            <text x="74" y="55" textAnchor="middle">RSyslog 接收</text>
          </g>
          <g data-login-node data-kind="storage" transform="translate(672 330)">
            <rect width="136" height="92" rx="8" />
            <text x="68" y="55" textAnchor="middle">数据存储</text>
          </g>
          <g data-login-node data-kind="query" transform="translate(1012 204)">
            <rect width="136" height="92" rx="8" />
            <text x="68" y="55" textAnchor="middle">日志查询</text>
          </g>
        </g>
        <g className="login-scene-particles">
          <circle data-login-particle data-route="main" r="5" />
          <circle data-login-particle data-route="main" r="4" />
          <circle data-login-particle data-route="branch" r="4" />
        </g>
      </svg>
    </div>
  );
}
```

实际 SVG 必须包含“日志源”“RSyslog 接收”“数据存储”“日志查询”四组中文标签，使用 `aria-hidden="true"` 排除装饰场景。

- [ ] **步骤 2：运行场景契约测试**

执行：

```powershell
cd web
npm test -- --test-name-pattern="login page separates"
```

预期：仍失败于 `LoginPage` 尚未引用场景，场景文件自身断言通过。

- [ ] **步骤 3：提交场景组件**

```powershell
git add web/src/components/LoginDataFlowScene.tsx
git commit -m "feat: add login data flow scene"
```

### 任务 3：实现 Anime.js 作用域与安全降级

**文件：**

- 新增：`web/src/animations/useLoginSceneMotion.ts`
- 测试：`web/tests/loginAnimation.test.ts`

**接口：**

- 消费：`root: React.RefObject<HTMLElement>`。
- 产出：`{ ready: boolean; playSuccess: () => Promise<void>; playError: () => Promise<void> }`。
- 选择器：任务 2 定义的数据属性，以及 `.login-brand`、`.login-title-block`、`.login-panel`。

- [ ] **步骤 1：实现控制器骨架和生命周期**

```ts
export type LoginSceneMotion = {
  ready: boolean;
  playSuccess: () => Promise<void>;
  playError: () => Promise<void>;
};

type AnimeModule = typeof import('animejs');
type AnimeScope = ReturnType<AnimeModule['createScope']>;

export function useLoginSceneMotion(root: React.RefObject<HTMLElement>): LoginSceneMotion {
  const scopeRef = React.useRef<AnimeScope | null>(null);
  const apiRef = React.useRef<AnimeModule | null>(null);
  const successRef = React.useRef<() => Promise<void>>(async () => undefined);
  const errorRef = React.useRef<() => Promise<void>>(async () => undefined);
  const [ready, setReady] = React.useState(false);

  React.useEffect(() => {
    let disposed = false;
    void import('animejs').then((anime) => {
      if (disposed || !root.current) return;
      apiRef.current = anime;
      scopeRef.current = anime.createScope({
        root,
        mediaQueries: {
          desktop: '(min-width: 901px)',
          reducedMotion: '(prefers-reduced-motion: reduce)',
        },
      }).add(() => {
        anime.createTimeline({ defaults: { ease: 'outExpo' } })
          .add('[data-login-grid], .login-brand', { opacity: [0, 1], duration: 260 })
          .add('[data-login-node]', { opacity: [0, 1], translateY: [10, 0], delay: anime.stagger(90), duration: 420 }, '-=120')
          .add('[data-login-path]', { opacity: [0, 1], strokeDashoffset: [1, 0], duration: 480 }, '-=280')
          .add('.login-panel', { opacity: [0, 1], translateY: [12, 0], duration: 420 }, '-=240');

        anime.animate('[data-login-particle]', {
          opacity: [0, 1, 1, 0],
          translateX: [0, 720],
          duration: 5600,
          delay: anime.stagger(720),
          loop: true,
          ease: 'linear',
        });

        successRef.current = () => new Promise<void>((resolve) => {
          anime.animate('.login-shell', { opacity: [1, 0], duration: 350, ease: 'inOutQuad', onComplete: resolve });
        });
        errorRef.current = () => new Promise<void>((resolve) => {
          anime.animate('.login-panel', { translateX: [0, -5, 5, -3, 3, 0], duration: 240, ease: 'inOutQuad', onComplete: resolve });
        });
      });
      setReady(true);
    }).catch(() => setReady(false));
    return () => {
      disposed = true;
      scopeRef.current?.revert();
      scopeRef.current = null;
      apiRef.current = null;
    };
  }, [root]);

  const playSuccess = React.useCallback(async () => {
    try { await successRef.current(); } catch { return; }
  }, []);
  const playError = React.useCallback(async () => {
    try { await errorRef.current(); } catch { return; }
  }, []);

  return { ready, playSuccess, playError };
}
```

`playSuccess` 和 `playError` 必须捕获所有动画异常并自行完成 Promise；减少动态效果、移动端或依赖不可用时立即 resolve。页面隐藏时暂停循环，恢复可见时继续。

- [ ] **步骤 2：补全 Anime.js V4 时间线**

入场顺序固定为网格/品牌、节点、路径、粒子、登录区；桌面粒子使用 `animate('[data-login-particle]', { loop: true, duration: 5600, delay: stagger(720) })` 错峰运行。成功动画最长 350ms，失败动画只移动 `.login-panel` 约 5px 并恢复原位。

- [ ] **步骤 3：运行作用域契约测试和类型检查**

```powershell
cd web
npm test -- --test-name-pattern="Anime.js V4"
npx tsc -b
```

预期：契约测试通过，TypeScript 无错误。

- [ ] **步骤 4：提交控制器**

```powershell
git add web/src/animations/useLoginSceneMotion.ts
git commit -m "feat: orchestrate login scene motion"
```

### 任务 4：集成登录状态与完整页面样式

**文件：**

- 修改：`web/src/pages/LoginPage.tsx`
- 修改：`web/src/styles.css`
- 测试：`web/tests/loginAnimation.test.ts`

**接口：**

- 消费：`LoginDataFlowScene`、`useLoginSceneMotion(rootRef)`。
- 保持：`LoginPage({ onSuccess }: { onSuccess: () => void })`。
- 行为：成功等待 `playSuccess()` 后调用 `onSuccess()`；失败显示现有中文消息、播放局部反馈并恢复密码焦点。

- [ ] **步骤 1：改造登录页状态流**

```tsx
const rootRef = React.useRef<HTMLDivElement>(null);
const passwordInputRef = React.useRef<InputRef>(null);
const [hasError, setHasError] = React.useState(false);
const { playSuccess, playError } = useLoginSceneMotion(rootRef);

const onFinish = async (values: { password: string }) => {
  if (loading) return;
  setLoading(true);
  setHasError(false);
  try {
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
```

根节点挂载 `ref={rootRef}`，渲染 `<LoginDataFlowScene />`，登录区使用 `login-panel${hasError ? ' is-error' : ''}`。保留“本机管理”“NAT 日志控制台”“管理员登录”“进入控制台”“管理员密码”“登录”“仅限授权管理员访问”文案。

- [ ] **步骤 2：替换登录页样式**

删除旧 `.login-copy`、`.login-flow-*` 和三个 `@keyframes login-flow-*`。新样式要求：

```css
.login-shell { min-height: 100dvh; overflow: hidden; background: #f5f7fa; }
.login-data-scene { position: absolute; inset: 0; pointer-events: none; }
.login-stage { min-height: 100dvh; grid-template-columns: minmax(0, 1fr) minmax(340px, 380px); }
.login-panel { border: 1px solid var(--fw-line); border-radius: 8px; background: #fff; }
.login-panel.is-error { border-color: var(--fw-danger); }

@media (max-width: 900px) {
  .login-data-scene { display: none; }
  .login-stage { grid-template-columns: 1fr; }
  .login-panel { width: min(100%, 420px); }
}

@media (prefers-reduced-motion: reduce) {
  .login-shell *, .login-shell *::before, .login-shell *::after {
    animation-duration: 0.01ms !important;
    animation-iteration-count: 1 !important;
    scroll-behavior: auto !important;
  }
}
```

桌面品牌与标题保持在左侧可读区域，登录工具区固定在右侧；场景、标题和面板不得重叠。移动端只保留品牌、标题和登录区，页面不得横向滚动。

- [ ] **步骤 3：运行全部前端验证**

```powershell
cd web
npm test
npx tsc -b
npm run build
```

预期：全部 Node 测试通过，TypeScript 无错误，Vite 生产构建成功输出到 `internal/server/web/dist`。

- [ ] **步骤 4：浏览器验收**

启动：

```powershell
cd web
npm run dev -- --port 5174
```

检查 `1440x1000`、`390x844` 和 `prefers-reduced-motion: reduce`。验证登录失败后密码框恢复焦点，键盘回车可提交，桌面动画非空且移动端没有场景循环、横向滚动或文字溢出；检查控制台无未处理异常。

- [ ] **步骤 5：提交页面集成**

```powershell
git add web/src/pages/LoginPage.tsx web/src/styles.css web/tests/loginAnimation.test.ts internal/server/web/dist
git commit -m "feat: redesign login with animated data flow"
```

## 最终检查

- [ ] 运行 `git diff --check`，不得出现空白错误。
- [ ] 运行 `git status --short`，确认只保留明确需要的未提交产物。
- [ ] 对照设计规格逐项确认：桌面入场、循环、成功、失败、移动端、减少动态效果、动态导入失败和卸载清理均已有实现或验收证据。
