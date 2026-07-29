# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## Unreleased

## [v0.0.1](https://github.com/Duke1616/eflow/releases/tag/v0.0.1) - 2026-07-28

- [`a49062e`](https://github.com/Duke1616/eflow/commit/a49062e9e340deaa29b2f6a3652a2560d9a51891) chore: 评价工单通过 needs 处理
- [`56d2833`](https://github.com/Duke1616/eflow/commit/56d283392b65699162a00a75cf1990e9d242fbb8) chore: 优化时间线返回
- [`7f0a525`](https://github.com/Duke1616/eflow/commit/7f0a525991ed2b5519cef4caef12dca34b7d982c) chore: 支持评价能力
- [`7ff7f3b`](https://github.com/Duke1616/eflow/commit/7ff7f3b4819dd8138dd4baed8aa446fa7e5a7153) chore: 新增工单撤回补偿能力
- [`e7f3d6d`](https://github.com/Duke1616/eflow/commit/e7f3d6d81eab2d494e90991236e31cdbecee7b7b) fix: 修复 dispacher 错误
- [`184ea15`](https://github.com/Duke1616/eflow/commit/184ea156e65c750b3167ea2c28633d97a38f14f8) fix: 修复派发规则匹配错误
- [`2c7fae4`](https://github.com/Duke1616/eflow/commit/2c7fae4d7bd1b59f6555dbc1c872d201be3c80dc) chore: 修复调度时间获取失败
- [`6db1628`](https://github.com/Duke1616/eflow/commit/6db162894da7a85a305baa80ecc252fbae071433) chore: eiam 版本升级
- [`336adbe`](https://github.com/Duke1616/eflow/commit/336adbe2d6fcfca4ce7b48aff836654d995690ed) chore: 完善工单列表访问权限控制、自动化任务定时能力重构
- [`25f1bf5`](https://github.com/Duke1616/eflow/commit/25f1bf5893316f8f8ea7add2d8da8132fb0a8612) fix: 修正
- [`a406b50`](https://github.com/Duke1616/eflow/commit/a406b50e8cc7476af5cfc68e756b269a50744dfe) chore: 调整 web capability
- [`16ec901`](https://github.com/Duke1616/eflow/commit/16ec9018908792287975c651e15042ac3234da51) fix: 修复 sql 查询错误
- [`0b21a3b`](https://github.com/Duke1616/eflow/commit/0b21a3b50d885045d8b190cc7d3528375ec1a54a) refactor: 重构工单任务
- [`a37c14f`](https://github.com/Duke1616/eflow/commit/a37c14f754408487a510543bec53822187fd55ab) docs: README
- [`7b1e20f`](https://github.com/Duke1616/eflow/commit/7b1e20fc84f6fbfac7b76843df3693c46f933f4c) fix: 修复 todo 租户未彻底隔离风险
- [`a53390c`](https://github.com/Duke1616/eflow/commit/a53390c43ff32835676140decf0df2c96d938fa5) chore: 补充工单需要的 download 权限
- [`802f027`](https://github.com/Duke1616/eflow/commit/802f0270986832538827fe751a68f3efca98b251) chore: 提交工单补充 upload 权限
- [`4b9b35c`](https://github.com/Duke1616/eflow/commit/4b9b35c6866986547b1134e51a16e2cdb23317e3) chore: task dispatch 模式修复
- [`620f3ca`](https://github.com/Duke1616/eflow/commit/620f3ca4cd707993f838d690324123d88d14d8ba) chore: retry 重新录入变量
- [`f4f896d`](https://github.com/Duke1616/eflow/commit/f4f896dbecb61cc854bbf9bf6fafc3538333cd36) fix: 发送消息失败
- [`f9bd514`](https://github.com/Duke1616/eflow/commit/f9bd514bf0f5a154215104eb7805be42311d48e3) fix: history 表存在，校准自增 ID
- [`92d2c85`](https://github.com/Duke1616/eflow/commit/92d2c8509104e0c8980352c88e5e990f485c3d90) chore: 修改 result 为 mediumtext 类型
- [`58e4cc5`](https://github.com/Duke1616/eflow/commit/58e4cc5b7538efaffe167c6a0db64a13bdfa547f) fix: 多租户情 gorm plugin ，不可以使用 join 语法，否则隔离存在问题
- [`b6ca0e9`](https://github.com/Duke1616/eflow/commit/b6ca0e9f87eb1c9360418149bf5fabc894ab6d7c) chore: 修复用户解析
- [`63dfb55`](https://github.com/Duke1616/eflow/commit/63dfb5567be92b3ec6839407c6d16770ef09f731) chore: 同步模版 cmd 命令
- [`438e1cb`](https://github.com/Duke1616/eflow/commit/438e1cb32111783a8afec0895de25f86bfa5a993) chore: 升级 eiam 版本，避免同步自增 ID 问题
- [`8332295`](https://github.com/Duke1616/eflow/commit/833229506ea10946d74f7f4769e4c9ac8a75465f) chore: 升级 eiam 版本，避免同步自增 ID 问题
- [`81de96e`](https://github.com/Duke1616/eflow/commit/81de96ef83783bcc2bfc64ddae79e01ff1020f3d) chore: 升级 eiam 版本，避免同步自增 ID 问题
- [`41dff39`](https://github.com/Duke1616/eflow/commit/41dff39a428b333a138f359f6cf58ad6a6abdfa0) chore: 补充 owner_id 参数传递，使用 1 管理员
- [`fac491a`](https://github.com/Duke1616/eflow/commit/fac491a9095df357f86ea19d66724c518f4474c4) chore: 升级为使用新版 template_set，移除 notify binding 表
- [`8ab2ff1`](https://github.com/Duke1616/eflow/commit/8ab2ff109f7588b62c6ce37d9e23568946f5abe4) fix: 修复 hook 替换失败
- [`98a61cd`](https://github.com/Duke1616/eflow/commit/98a61cd6e9f0053889527593be9b0b77e8da4043) chore: 移除 codebook_uid 统一使用 codebook_id
- [`6b1832e`](https://github.com/Duke1616/eflow/commit/6b1832ee5b9e24aaa425b97ce40cc01a3a327e42) chore: 同步逻辑
- [`9adce6f`](https://github.com/Duke1616/eflow/commit/9adce6faaf5391fdffd393222e58749af92cb3ad) chore: 迁出 codebook、runner 到 etask 项目
- [`ca7a61e`](https://github.com/Duke1616/eflow/commit/ca7a61ee6875464a6455c363f07036bd2289de35) chore: 清理无用的接口
- [`c883557`](https://github.com/Duke1616/eflow/commit/c883557580b6a20aa3352c16915c3b31c1ba101e) chore: 新增模版相关接口
- [`4b65fe7`](https://github.com/Duke1616/eflow/commit/4b65fe7efa9c733ec03e73cb77c0e0095c378fb0) fix: 自动化任务运行失败
- [`4b8662d`](https://github.com/Duke1616/eflow/commit/4b8662d33662ff58d3d31a4a7e2fc0289d3a9b5e) fix: SQL
- [`62c830e`](https://github.com/Duke1616/eflow/commit/62c830e71680a07b919b2cc5328705a18c8bccf6) fix: SQL 编写错误
- [`d6bca14`](https://github.com/Duke1616/eflow/commit/d6bca14f0b298c7c1cd1c4336632620f5f63b3e7) chore: 完善 task 逻辑
- [`f527073`](https://github.com/Duke1616/eflow/commit/f5270734da3a57d9d298c9179bbe4b48a2fbd5e2) chore: 升级 eiam 依赖
- [`ce0958e`](https://github.com/Duke1616/eflow/commit/ce0958e4ebba5f6485c82b8d088935b915d6a699) chore: remove eiam local replace for docker build
- [`886aa8c`](https://github.com/Duke1616/eflow/commit/886aa8c87a10288e6633634d495246934fdf2542) chore: 自增ID 同步
- [`740ecc8`](https://github.com/Duke1616/eflow/commit/740ecc8ceee838b5737893dfdffb6f389d059d7d) chore: 去除重置自增ID
- [`9aa0cb4`](https://github.com/Duke1616/eflow/commit/9aa0cb4310ddcc16f277ff14e6a38087e279cf73) chore: task
- [`20043e8`](https://github.com/Duke1616/eflow/commit/20043e80f42d883a7db8b2fe5507d1250b7af919) chore: 打印日志
- [`3ca1492`](https://github.com/Duke1616/eflow/commit/3ca1492af34d233dbf43aff908e5f3abde8ef05e) chore: 添加 --force 子命令
- [`60c0249`](https://github.com/Duke1616/eflow/commit/60c0249422c2905e267bf53f1bf26067911e066a) chore: 修正 default 路径
- [`d0cf3fb`](https://github.com/Duke1616/eflow/commit/d0cf3fbca512314d11931d91ecf4ea57d2c4ce9f) chore: 修正 docker entiypoint
- [`19500b6`](https://github.com/Duke1616/eflow/commit/19500b664e6f6f09cd89ec7ea7d289e180aedd69) chore: 恢复加密
- [`96e169f`](https://github.com/Duke1616/eflow/commit/96e169f7f574c71870cd728b3dfc7f2442e8f78b) chore: 优化 migrate
- [`5effaf1`](https://github.com/Duke1616/eflow/commit/5effaf1feb7c70997f2c7d98e2c8f4124bad787a) chore: dockerfile golang 版本
- [`159b60c`](https://github.com/Duke1616/eflow/commit/159b60c716833d9970f33edb0d4ef293adb6a3de) fix: 不存在 verison 时报错
- [`e7f45f4`](https://github.com/Duke1616/eflow/commit/e7f45f4db3be849c6a96a3b4af2b70fc9c9ecb1b) chore: github action
- [`4af0362`](https://github.com/Duke1616/eflow/commit/4af0362e75515f8e39cc28bf69ad082b65750fd0) chore: 升级 eiam 版本
- [`64096ac`](https://github.com/Duke1616/eflow/commit/64096acb598ea08bd81051f244ce948f56703532) chore: 修正 web 能力
- [`5a2959d`](https://github.com/Duke1616/eflow/commit/5a2959de04af7dc2d3f796da78ecf6ea21a8d323) chore: 完善 web 能力
- [`74a0269`](https://github.com/Duke1616/eflow/commit/74a0269d8f7b13d0fa34a21918fb25ce84630193) chore: 完善 migrate 迁移行为
- [`187e6ac`](https://github.com/Duke1616/eflow/commit/187e6acdac1403890d19069bcb16a7c9b23bb04c) chore: 调整 web 能力说明
- [`bc87b82`](https://github.com/Duke1616/eflow/commit/bc87b8247c8abdcec2a093d45f4c4b7d902d4aac) chore: 调整 web 层能力名称，code码
- [`1da6c01`](https://github.com/Duke1616/eflow/commit/1da6c01dcc3d510447428a694f2173845855e4f3) chore: 调整细节
- [`ddcbc39`](https://github.com/Duke1616/eflow/commit/ddcbc39ee3ce00891d6fcf244fb984f59277f084) chore: easyflow 流程租户传递，允许 admin 权限用户提交他人的工单审批，日志记录
- [`2407e9a`](https://github.com/Duke1616/eflow/commit/2407e9a8ead1e2d7b4aa27fccd1045e322921639) chore: 优化生产者消费者模式代码逻辑, 新增飞书 event 事件租户ID传递
- [`01da5e2`](https://github.com/Duke1616/eflow/commit/01da5e219674cbc49cf8e2d3a4e1b74c34b59275) chore: 完善逻辑
- [`28ce117`](https://github.com/Duke1616/eflow/commit/28ce1174a2623fde85e3108fe9cfc8e8655b4138) fix: 修复 start job sql 判断错误，飞书流程进度推送
- [`c03747e`](https://github.com/Duke1616/eflow/commit/c03747efdfa181e9b47de81c49fa3991791974fe) fix: utime 对比时间错误, 工单无法结束
- [`c588ceb`](https://github.com/Duke1616/eflow/commit/c588ceb868499cbc90a4b1bc7f9b18a70396294b) chore: tenant_id 调整为 int64 类型
- [`b4f3b86`](https://github.com/Duke1616/eflow/commit/b4f3b8682c2f99579884062ec9d96821a8721c2d) chore: 完善代码测试, 新版本飞书回调
- [`ba2d19f`](https://github.com/Duke1616/eflow/commit/ba2d19f92e05d1e6f66693c9e5a100815de691c7) chore: 修复 detail、delete 接口 id 转换
- [`86866b4`](https://github.com/Duke1616/eflow/commit/86866b42870615155413883d267c198500958866) chore: 实现数据表迁移
- [`bb86e28`](https://github.com/Duke1616/eflow/commit/bb86e282555a5f183cbe436fd228aefc0d104202) chore: 完善数据迁移逻辑
- [`775dd20`](https://github.com/Duke1616/eflow/commit/775dd20e1d8aae0c47202f66a1614bf647051b31) chore: 逐步完善代码迁移
- [`85501cf`](https://github.com/Duke1616/eflow/commit/85501cff5ccafe7f666bc56586620282c1c7fb45) chore: service 目录建设
- [`84abb72`](https://github.com/Duke1616/eflow/commit/84abb72b8c58462c043683165283af110e471b18) chore: 迁移 codebook、runner、template、workflow 模块到本项目
- [`7e995c1`](https://github.com/Duke1616/eflow/commit/7e995c1df79f941d3a4b337262fb70e44a752554) first commit
