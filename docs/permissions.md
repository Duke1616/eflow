# EFLOW 全局权限大盘与元数据字典

> 本文档由 `permgen` 基于全仓 AST 静态分析自动生成。请勿手动修改。
>
> 💡 **联动包含机制**：当为角色分配某项操作权限时，系统将**自动附带拥有**其“联动包含”中的权限，无需管理员手动重复勾选（例如：勾选“修改用户”会自动附带拥有“用户详情”权限）。

- **受控业务模块数**: 5
- **受控权限点总数**: 50


## 模块: 工单模板/执行单元路由 (`dispatch`)

- **所属服务**: `ticket`
- **定义源码**: `internal/web/dispatch/handler.go`

| 操作名称 | 完整权限码 | 作用域 | 归属类型 | 暴露状态 | 联动包含权限 | 宿主源码位置 |
|:---|:---|:---|:---|:---|:---|:---|
| 创建执行单元路由 | `ticket:dispatch:add` | 租户级 | 本级 | 正常 | 查询流程自动化节点 · `ticket:workflow:view_automation_nodes`<br>`task:runner:view_by_ids` | `internal/web/dispatch/handler.go` 行 33 |
| 删除执行单元路由 | `ticket:dispatch:delete` | 租户级 | 本级 | 正常 | - | `internal/web/dispatch/handler.go` 行 41 |
| 修改执行单元路由 | `ticket:dispatch:edit` | 租户级 | 本级 | 正常 | 查询流程自动化节点 · `ticket:workflow:view_automation_nodes`<br>`task:runner:view_by_ids` | `internal/web/dispatch/handler.go` 行 37 |
| 复制执行单元路由 | `ticket:dispatch:sync` | 租户级 | 本级 | 正常 | 根据流程获取模板 · `ticket:template:view_by_workflow_id` | `internal/web/dispatch/handler.go` 行 44 |
| 执行单元路由列表 | `ticket:dispatch:view` | 租户级 | 本级 | 正常 | 工单模板详情 · `ticket:template:get`<br>`task:runner:view_by_ids`<br>`task:runner:view_by_codebook_id`<br>查询流程自动化节点 · `ticket:workflow:view_automation_nodes` | `internal/web/dispatch/handler.go` 行 48 |

---


## 模块: 工单中心 (`manager`)

- **所属服务**: `ticket`
- **定义源码**: `internal/web/task/handler.go`

| 操作名称 | 完整权限码 | 作用域 | 归属类型 | 暴露状态 | 联动包含权限 | 宿主源码位置 |
|:---|:---|:---|:---|:---|:---|:---|
| 任务节点表单配置 | `ticket:manager:form_config` | 租户级 | 本级 | 正常 | 工单模板详情 · `ticket:template:get`<br>工单详情 · `ticket:manager:get` | `internal/web/ticket/handler.go` 行 129 |
| 工单详情 | `ticket:manager:get` | 租户级 | 本级 | 正常 | 流程轨迹图 · `ticket:manager:graph`<br>流转时间线 · `ticket:manager:timeline`<br>`cmdb:tools:download` | `internal/web/ticket/handler.go` 行 72 |
| 流程轨迹图 | `ticket:manager:graph` | 租户级 | 跨域 (workflow) | 正常 | - | `internal/web/workflow/handler.go` 行 77 |
| 历史工单 | `ticket:manager:history` | 租户级 | 本级 | 正常 | 批量获取模板详情 · `ticket:template:view_by_ids`<br>工单详情 · `ticket:manager:get`<br>评价工单 · `ticket:manager:rate` | `internal/web/ticket/handler.go` 行 94 |
| 我发起的工单 | `ticket:manager:my_start` | 租户级 | 本级 | 正常 | 批量获取模板详情 · `ticket:template:view_by_ids`<br>工单详情 · `ticket:manager:get` | `internal/web/ticket/handler.go` 行 100 |
| 我的待办工单 | `ticket:manager:my_todo` | 租户级 | 本级 | 正常 | 批量获取模板详情 · `ticket:template:view_by_ids`<br>工单详情 · `ticket:manager:get` | `internal/web/ticket/handler.go` 行 89 |
| 同意审批 | `ticket:manager:pass` | 租户级 | 本级 | 正常 | 任务节点表单配置 · `ticket:manager:form_config` | `internal/web/ticket/handler.go` 行 105 |
| 重新启动流程 | `ticket:manager:process_restart` | 租户级 | 本级 | 正常 | - | `internal/web/ticket/handler.go` 行 68 |
| 评价工单 | `ticket:manager:rate` | 租户级 | 本级 | 静默 (不暴露) | - | `internal/web/ticket/handler.go` 行 124 |
| 驳回审批 | `ticket:manager:reject` | 租户级 | 本级 | 正常 | 任务节点表单配置 · `ticket:manager:form_config` | `internal/web/ticket/handler.go` 行 110 |
| 撤销工单 | `ticket:manager:revoke` | 租户级 | 本级 | 正常 | - | `internal/web/ticket/handler.go` 行 120 |
| 提交工单 | `ticket:manager:submit` | 租户级 | 本级 | 正常 | 工单模板详情 · `ticket:template:get`<br>收藏状态变更 · `ticket:template:toggle_favorite`<br>模板收藏夹 · `ticket:template:view_favorite`<br>工单模板列表 · `ticket:template:view`<br>查询模板分组摘要 · `ticket:template:view_group_summary`<br>`cmdb:tools:upload` | `internal/web/ticket/handler.go` 行 62 |
| 流转时间线 | `ticket:manager:timeline` | 租户级 | 本级 | 正常 | - | `internal/web/ticket/handler.go` 行 78 |
| 所有待办工单 | `ticket:manager:todo` | 租户级 | 本级 | 正常 | 批量获取模板详情 · `ticket:template:view_by_ids`<br>工单详情 · `ticket:manager:get` | `internal/web/ticket/handler.go` 行 83 |
| 转交审批人 | `ticket:manager:transfer` | 租户级 | 本级 | 正常 | `iam:user:view` | `internal/web/ticket/handler.go` 行 115 |
| 关联自动化任务 | `ticket:manager:view_tasks` | 租户级 | 跨域 (task) | 正常 | - | `internal/web/task/handler.go` 行 42 |

---


## 模块: 工单中心/自动化任务 (`task`)

- **所属服务**: `ticket`
- **定义源码**: `internal/web/task/handler.go`

| 操作名称 | 完整权限码 | 作用域 | 归属类型 | 暴露状态 | 联动包含权限 | 宿主源码位置 |
|:---|:---|:---|:---|:---|:---|:---|
| 执行尝试日志 | `ticket:task:logs` | 租户级 | 本级 | 静默 (不暴露) | - | `internal/web/task/handler.go` 行 56 |
| 重试自动化任务 | `ticket:task:retry` | 租户级 | 本级 | 正常 | - | `internal/web/task/handler.go` 行 46 |
| 强制终止自动化任务 | `ticket:task:terminate` | 租户级 | 本级 | 正常 | - | `internal/web/task/handler.go` 行 49 |
| 自动化任务列表 | `ticket:task:view` | 租户级 | 本级 | 正常 | - | `internal/web/task/handler.go` 行 39 |
| 执行尝试列表 | `ticket:task:view_attempts` | 租户级 | 本级 | 正常 | 执行尝试日志 · `ticket:task:logs`<br>`task:execution:logs` | `internal/web/task/handler.go` 行 52 |

---


## 模块: 工单模板 (`template`)

- **所属服务**: `ticket`
- **定义源码**: `internal/web/template/handler.go`

| 操作名称 | 完整权限码 | 作用域 | 归属类型 | 暴露状态 | 联动包含权限 | 宿主源码位置 |
|:---|:---|:---|:---|:---|:---|:---|
| 创建工单模板 | `ticket:template:add` | 租户级 | 本级 | 正常 | 查询模板分组列表 · `ticket:template:view_group`<br>流程列表 · `ticket:workflow:view`<br>流程详情 · `ticket:workflow:get` | `internal/web/template/handler.go` 行 57 |
| 创建模板分类 | `ticket:template:add_group` | 租户级 | 本级 | 正常 | - | `internal/web/template/handler.go` 行 91 |
| 删除工单模板 | `ticket:template:delete` | 租户级 | 本级 | 正常 | - | `internal/web/template/handler.go` 行 65 |
| 删除模板分类 | `ticket:template:delete_group` | 租户级 | 本级 | 正常 | - | `internal/web/template/handler.go` 行 99 |
| 修改工单模板 | `ticket:template:edit` | 租户级 | 本级 | 正常 | 工单模板详情 · `ticket:template:get`<br>查询模板分组列表 · `ticket:template:view_group`<br>流程列表 · `ticket:workflow:view`<br>流程详情 · `ticket:workflow:get` | `internal/web/template/handler.go` 行 61 |
| 修改模板分类 | `ticket:template:edit_group` | 租户级 | 本级 | 正常 | - | `internal/web/template/handler.go` 行 95 |
| 工单模板详情 | `ticket:template:get` | 租户级 | 本级 | 正常 | - | `internal/web/template/handler.go` 行 37 |
| 获取流程绑定模板校验链 | `ticket:template:rules_by_workflow_id` | 租户级 | 本级 | 静默 (不暴露) | - | `internal/web/template/handler.go` 行 53 |
| 收藏状态变更 | `ticket:template:toggle_favorite` | 租户级 | 本级 | 静默 (不暴露) | - | `internal/web/template/handler.go` 行 70 |
| 工单模板列表 | `ticket:template:view` | 租户级 | 本级 | 正常 | 批量获取流程详情 · `ticket:workflow:view_by_ids`<br>查询模板分组摘要 · `ticket:template:view_group_summary` | `internal/web/template/handler.go` 行 40 |
| 批量获取模板详情 | `ticket:template:view_by_ids` | 租户级 | 本级 | 静默 (不暴露) | - | `internal/web/template/handler.go` 行 45 |
| 根据流程获取模板 | `ticket:template:view_by_workflow_id` | 租户级 | 本级 | 静默 (不暴露) | - | `internal/web/template/handler.go` 行 49 |
| 模板收藏夹 | `ticket:template:view_favorite` | 租户级 | 本级 | 静默 (不暴露) | - | `internal/web/template/handler.go` 行 74 |
| 查询模板分组列表 | `ticket:template:view_group` | 租户级 | 本级 | 静默 (不暴露) | - | `internal/web/template/handler.go` 行 81 |
| 查询模板分组摘要 | `ticket:template:view_group_summary` | 租户级 | 本级 | 静默 (不暴露) | - | `internal/web/template/handler.go` 行 86 |

---


## 模块: 流程管理 (`workflow`)

- **所属服务**: `ticket`
- **定义源码**: `internal/web/workflow/handler.go`

| 操作名称 | 完整权限码 | 作用域 | 归属类型 | 暴露状态 | 联动包含权限 | 宿主源码位置 |
|:---|:---|:---|:---|:---|:---|:---|
| 创建流程 | `ticket:workflow:add` | 租户级 | 本级 | 正常 | 流程详情 · `ticket:workflow:get`<br>`iam:user:view` | `internal/web/workflow/handler.go` 行 39 |
| 删除流程 | `ticket:workflow:delete` | 租户级 | 本级 | 正常 | - | `internal/web/workflow/handler.go` 行 47 |
| 流程发布 | `ticket:workflow:deploy` | 租户级 | 本级 | 正常 | 流程详情 · `ticket:workflow:get` | `internal/web/workflow/handler.go` 行 50 |
| 修改流程 | `ticket:workflow:edit` | 租户级 | 本级 | 正常 | 流程详情 · `ticket:workflow:get`<br>`iam:user:view` | `internal/web/workflow/handler.go` 行 43 |
| 流程详情 | `ticket:workflow:get` | 租户级 | 本级 | 正常 | - | `internal/web/workflow/handler.go` 行 72 |
| 流程列表 | `ticket:workflow:view` | 租户级 | 本级 | 正常 | `iam:user:view` | `internal/web/workflow/handler.go` 行 56 |
| 查询流程自动化节点 | `ticket:workflow:view_automation_nodes` | 租户级 | 本级 | 静默 (不暴露) | - | `internal/web/workflow/handler.go` 行 68 |
| 批量获取流程详情 | `ticket:workflow:view_by_ids` | 租户级 | 本级 | 静默 (不暴露) | - | `internal/web/workflow/handler.go` 行 64 |
| 模糊检索流程模板 | `ticket:workflow:view_by_keyword` | 租户级 | 本级 | 静默 (不暴露) | - | `internal/web/workflow/handler.go` 行 60 |

---


