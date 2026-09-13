# 访问趋势正式数据接入修复报告

## 1. 问题与范围

访问趋势地图此前使用前端固定线路演示数据；左侧导航 Logo 未复用登录页已有的动态粒子路径效果。本轮按已确认口径改为正式 NAT 日志聚合数据：公网目标判断国内/国外、五元组聚合、最多展示 50 条、默认 30 天、区分 `source_id`。

## 2. 改动文件

- [internal/model/types.go](../../../internal/model/types.go)：新增访问趋势响应模型。
- [internal/storage/clickhouse/access_trend.go](../../../internal/storage/clickhouse/access_trend.go)：新增 ClickHouse 五元组聚合查询。
- [internal/server/access_trend_adapter.go](../../../internal/server/access_trend_adapter.go)：新增趋势服务、时间范围和公网地域分类。
- [internal/server/handlers/access_trend.go](../../../internal/server/handlers/access_trend.go)：新增 JSON handler。
- [internal/server/router.go](../../../internal/server/router.go)：注册受认证保护的 `/api/access-trend`。
- [web/src/pages/AccessTrendPreview.tsx](../../../web/src/pages/AccessTrendPreview.tsx)：移除固定 flows，接入正式接口并增加加载、错误、空数据状态。
- [web/src/components/BrandLogo.tsx](../../../web/src/components/BrandLogo.tsx)：增加作用域内 GSAP 粒子动画。
- [web/src/layout/AppLayout.tsx](../../../web/src/layout/AppLayout.tsx)：启用导航 Logo 动画。

## 3. 数据与安全口径

- 聚合键为 `source_id + src_ip + src_port + dst_ip + dst_port + protocol`，NAT 地址和端口取该组最新时间的值。
- 使用 `max(timestamp)` 作为 `last_seen`。
- 趋势接口沿用 `requireAuth`，不暴露未登录数据。
- 私有地址、未知公网地址不伪造地理坐标；无 GeoIP/CIDR 时线路仅进入无坐标数据。

## 4. 验收与测试

- `go test -count=1 ./...` 通过。
- `npm.cmd run build` 通过。
- `npm.cmd test` 通过，66 项全部通过。
- Windows 本地 `go test -race` 因 CGO 未启用不可执行，未将其结果写作通过。

## 5. 版本与风险

版本保持 `v2.1.2`，未修改版本文件。趋势查询当前从 `nat_logs` 做受限聚合并返回最多 500 个候选组、前端展示 50 条；日志规模极大时应继续建设按天五元组物化聚合表，再切换查询源。

## 6. 结论

访问趋势已不再依赖演示线路，页面筛选、日志来源、NAT 映射和最近访问时间均来自后端接口；导航 Logo 恢复了可降级的动态粒子效果。
