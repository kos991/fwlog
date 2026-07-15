# 登录页柔性主链流程图实施计划

> **执行要求：** 按任务顺序修改、测试和浏览器复核；只处理登录页左侧流程图。

**目标：** 将当前旧版三路径流程图改为用户确认的 A 版柔性主链，只保留完整主链和归档支线。

**实现方式：** 继续使用 `LoginDataFlowScene` 的 SVG 节点和 Anime.js 路径粒子。节点坐标与文案保持不变，只替换路径、标签和粒子集合，并删除灰色查询支线对应动画。

**技术栈：** React、TypeScript、SVG、Anime.js V4、Node Test Runner、Vite。

## 全局约束

- 登录卡片、BrandLogo、背景网格和左下角 `INGEST PIPELINE` 不变。
- 五个节点坐标、尺寸和技术文案不变。
- 只保留蓝色主链与绿色归档支线。
- 不新增悬停浮层、额外流光线或节点位移动画。
- 不回退工作区其他未提交改动。

---

### 任务 1：锁定 A 版路径契约

**文件：**
- 修改：`web/tests/loginAnimation.test.ts`

**接口：**
- 输入：`LoginDataFlowScene.tsx` 和 `useLoginSceneMotion.ts` 源码文本。
- 输出：两条路径、三颗粒子和无查询支线动画的静态断言。

- [ ] **步骤 1：修改路径断言**

```ts
assert.equal(scene.match(/data-login-path/g)?.length, 2);
assert.match(scene, /M204 410 C264 410 286 278 354 278 H520 C600 278 632 400 718 400 H884 C954 400 986 298 1070 298/);
assert.match(scene, /M437 334 C437 448 548 526 718 526/);
assert.doesNotMatch(scene, /login-route-query|login-scene-path--quiet|data-route="query"/);
assert.equal(scene.match(/<circle\s+data-login-particle/g)?.length, 3);
assert.doesNotMatch(hook, /createMotionPath\('#login-route-query'\)|data-route="query"/);
```

- [ ] **步骤 2：运行测试并确认先失败**

运行：`npm.cmd test -- --test-name-pattern="login"`

预期：旧版仍有三条路径和查询粒子，因此登录流程测试失败。

### 任务 2：实现柔性主链

**文件：**
- 修改：`web/src/components/LoginDataFlowScene.tsx`
- 修改：`web/src/animations/useLoginSceneMotion.ts`
- 修改：`web/src/styles.css`

**接口：**
- 输入：任务 1 的路径与粒子断言。
- 输出：`login-route-ingest` 和 `login-route-archive` 两条可供 Anime.js 使用的路径。

- [ ] **步骤 1：替换 SVG 路径和标签**

```tsx
<path id="login-route-ingest" d="M204 410 C264 410 286 278 354 278 H520 C600 278 632 400 718 400 H884 C954 400 986 298 1070 298" />
<path id="login-route-archive" d="M437 334 C437 448 548 526 718 526" />
```

保留 `采集 / 入库 / 检索 / 归档` 四个路径标签，删除 `login-route-query`。

- [ ] **步骤 2：收敛路径粒子**

```tsx
<circle data-login-particle data-route="ingest" className="login-scene-particle" r="5" />
<circle data-login-particle data-route="ingest" className="login-scene-particle" r="4" />
<circle data-login-particle data-route="archive" className="login-scene-particle login-scene-particle--green" r="4" />
```

- [ ] **步骤 3：删除查询支线动画和无用样式**

删除 `queryMotion`、查询粒子动画、`.login-scene-path--quiet` 和 `.login-scene-particle--muted`。

- [ ] **步骤 4：运行前端测试**

运行：`npm.cmd test`

预期：`39` 项测试全部通过。

### 任务 3：构建与浏览器验收

**文件：**
- 验证：`web/src/components/LoginDataFlowScene.tsx`
- 验证：`web/src/animations/useLoginSceneMotion.ts`
- 验证：`web/src/styles.css`

**接口：**
- 输入：任务 2 的最终页面。
- 输出：可构建且在桌面、移动端表现正确的登录页。

- [ ] **步骤 1：运行生产构建和格式检查**

运行：

```powershell
npm.cmd run build
git diff --check -- web/src/components/LoginDataFlowScene.tsx web/src/animations/useLoginSceneMotion.ts web/src/styles.css web/tests/loginAnimation.test.ts
```

预期：构建成功；除既有 chunk 和换行提示外无错误。

- [ ] **步骤 2：桌面浏览器验证**

确认主链依次连接五个业务关系中的四个主节点，归档支线在压缩归档左边缘结束，没有灰色悬空线和绿色无目标延伸线；蓝绿粒子均持续移动。

- [ ] **步骤 3：移动端验证**

确认 `900px` 以下隐藏流程图，登录卡片无横向溢出。
