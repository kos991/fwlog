# 登录卡品牌集成实施计划

> **执行约束：**使用 `executing-plans` 在当前会话逐项执行，保持测试先行。

**目标：**将独立在左上角的 Logo 和“NAT 日志控制台”移入登录卡片，形成唯一视觉中心。

**架构：**`LoginPage` 继续负责认证状态，`LoginDataFlowScene` 继续只渲染背景拓扑。仅调整登录页 DOM 层级、动画选择器和 CSS 网格位置，不修改认证接口、Anime.js 生命周期或移动端场景降级。

**技术栈：**React 18、TypeScript 5、Ant Design 5、Anime.js 4.5.0、CSS Grid、Node 测试。

---

### 任务 1：锁定卡片内品牌结构

**文件：**

- 修改：`web/tests/loginAnimation.test.ts`
- 修改：`web/src/pages/LoginPage.tsx`

- [ ] **步骤 1：编写失败测试**

测试必须断言 `login-panel-product` 内包含 `login-logo` 和“NAT 日志控制台”，页面不再包含 `login-copy`、`login-intro` 或“进入控制台”。

- [ ] **步骤 2：运行红灯测试**

```powershell
cd web
npm.cmd test -- --test-name-pattern="login page separates"
```

预期：失败于卡片品牌结构尚不存在。

- [ ] **步骤 3：调整登录页 DOM**

登录卡头部结构固定为：

```tsx
<div className="login-panel-product">
  <span className="login-logo"><SafetyCertificateOutlined /></span>
  <div>
    <h1>NAT 日志控制台</h1>
    <Text type="secondary">管理员登录</Text>
  </div>
</div>
```

删除左侧 `login-copy` 和卡片中的“进入控制台”。表单、错误状态和页脚保持原样。

### 任务 2：调整卡片位置与动效选择器

**文件：**

- 修改：`web/src/styles.css`
- 修改：`web/src/animations/useLoginSceneMotion.ts`
- 修改：`web/tests/loginAnimation.test.ts`

- [ ] **步骤 1：更新布局测试**

断言 `.login-panel` 桌面使用 `grid-column: 2`，移动端恢复 `grid-column: 1`；删除对 `.login-copy` 和 `.login-intro` 的旧断言。

- [ ] **步骤 2：实现卡片头样式**

`.login-panel-product` 使用两列网格，Logo 固定 34px，产品名使用 20px 紧凑标题，“管理员登录”作为次级文本。桌面登录卡保持右列，移动端占据唯一列。

- [ ] **步骤 3：更新 Anime.js 入场选择器**

网格独立淡入，登录卡整体入场；删除 `.login-brand` 和 `.login-title-block` 选择器。节点图标动画、粒子循环、成功和失败反馈保持不变。

### 任务 3：完整验证

- [ ] **步骤 1：运行自动验证**

```powershell
cd web
npm.cmd test
npx.cmd tsc -b
npm.cmd run build
```

预期：34 项测试全部通过，TypeScript 和 Vite 构建成功。

- [ ] **步骤 2：浏览器验收**

检查 125% 缩放、`1440x1000`、`390x844` 和减少动态效果模式。卡片品牌不得溢出，左侧不得残留独立标题，拓扑与登录卡不得重叠。
