# 访问趋势地图预览报告

> 本报告后续已扩展为正式数据概览替换记录：访问趋势地图已嵌入 `HealthDashboard`，不再只是独立预览。

## 1. 目标与范围

提供一个不影响现有数据概览的独立预览，用真实地图展示国内、国外及全部访问流向，并支持点击线路查看 NAT 访问明细。

## 2. 改动文件

- [`web/src/pages/AccessTrendPreview.tsx`](../../web/src/pages/AccessTrendPreview.tsx)
- [`web/src/assets/maps/china.json`](../../web/src/assets/maps/china.json)
- [`web/src/assets/maps/world.json`](../../web/src/assets/maps/world.json)
- [`web/src/main.tsx`](../../web/src/main.tsx)
- [`web/src/styles.css`](../../web/src/styles.css)
- [`web/tests/accessTrendPreview.test.ts`](../../web/tests/accessTrendPreview.test.ts)
- [`web/package.json`](../../web/package.json)
- [`web/package-lock.json`](../../web/package-lock.json)

## 3. 关键内容

- 使用 ECharts Geo 和本地 GeoJSON，国内模式显示中国省级地图，全部/国外模式显示世界地图。
- 支持全部访问、国内访问、国外访问切换；线路和节点可点击，并同步更新右侧详情。
- 桌面主体高度固定为 460px，适配现有概览卡片；窄屏自动上下排列。
- 预览通过 `?preview=access` 独立访问，并使用动态加载隔离 ECharts 与地图资源，不进入常规登录和控制台首屏加载路径。
- 正式数据概览的嵌入态移除重复的“数据概览 / 访问趋势”标题栏，将访问范围、时间范围和刷新操作收进右侧详情栏顶部，地图与详情仍保持单一卡片布局。
- 根据最终布局反馈，将筛选操作和访问线路进一步合并为地图左下两行浮动底栏；右侧仅保留统计和当前线路详情，并取消内部滚动条。窄屏下底栏回到地图下方的正常文档流。

## 4. 验收

- `npm.cmd test`：64 项通过。
- `npm.cmd run build`：通过。
- 1440px 浏览器检查：页面宽度无溢出，主体高度 460px，Canvas 正常渲染。
- 768px 浏览器检查：页面宽度无溢出，国内模式显示中国地图和两条国内线路。
- 测试服务器 `https://192.168.0.130:8080/?preview=access`：HTTPS 200，1440px 下主体高度 460px、页面无横向溢出、地图 Canvas 正常渲染且控制台无错误。

截图：

- [桌面预览](../../.tmp-preview/access-trend.png)
- [窄屏预览](../../.tmp-preview/access-trend-narrow.png)
- [测试服务器桌面预览](../../.tmp-preview/remote-access-desktop.png)
- [嵌入态最终布局](../../.tmp-preview/access-trend-embedded-final.png)
- [地图底栏桌面布局](../../.tmp-preview/access-trend-dock-desktop.png)
- [地图底栏窄屏布局](../../.tmp-preview/access-trend-dock-narrow.png)

## 5. 版本同步

版本保持 `v2.1.2`，本轮在当前补丁版本中继续完善正式数据概览页面，不单独升版。

## 6. 风险与备注

当前访问记录为模拟数据。正式接入需要新增或扩展后端地理访问聚合接口，并明确无法 GeoIP 定位的内网地址展示规则。

本轮远端替换前备份为 `/opt/fwlog/fwlog.backup-20260909102523`。

## 7. 结论

紧凑型真实地图已嵌入正式数据概览；重复标题已移除，筛选和线路操作已集中在地图左下，右侧详情不再产生内部滚动。当前仍需后续接入真实 GeoIP 聚合数据。
