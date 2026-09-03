# EFlow 全局工程规范与 AI 协作准则 (AGENTS.md)

EFlow 是企业级工单流转与工作流编排平台，基于 EasyFlow 图引擎、EIAM 权限治理与低代码表单。

## 1. 架构分层与依赖单向流转铁律
$$\text{Transport (Web)} \longrightarrow \text{Service} \longrightarrow \text{Repository} \longrightarrow \text{DAO / Engine}$$
- **Domain (`internal/domain/`)**：纯 Go 领域模型，严禁依赖 Gin、GORM 等传输或持久化包。
- **Transport (`internal/web/`)**：薄适配层，采用 `Define(name, code).Bind(...)` 声明路由，**严禁写业务逻辑，严禁直接访问 DAO**。
- **Service (`internal/service/`)**：核心业务编排、状态机推进与事件分发。禁止反向依赖 Transport。
- **Engine 防腐 (`internal/service/engine/`)**：隔离底层工作流引擎，业务与表现层逻辑下沉到 Service，杜绝引擎层二次序列化侵入。
- **Repository & DAO (`internal/repository/`)**：Repo 负责领域对象双向映射；DAO 负责 SQL、行级排他锁与事务控制。

## 2. 权限契约与 EIAM 自发现规范
- **声明式受控注入**：Handler 统一使用 `h.Define("名称", "编码").Bind(...)`；跨领域路由挂载必须使用 `h.For(model.Xxx)`。
- **强类型权限依赖 (Needs)**：
  - 本服务受控依赖**强制引用 `perm.Xxx.Yyy` 强类型常量**，禁止手写字面量字符串；外部服务依赖保留标准 URN。
  - 严格根据前端交互定义依赖，严禁过度授予运维级高危权限（如 ViewTasks）。
- **工具链自动化**：路由或权限修改后，必须执行 `permgen` 自动化生成契约代码与权限文档：
  - `pkg/contract/perm/zz_generated_perms.go`
  - `pkg/contract/model/zz_generated_models.go`
  - `docs/permissions.md`

## 3. 并发安全与可靠性法则
- **并发写冲突防护**：工单动态数据合并（如 `MergeTicketData`）必须在事务中使用行级排他锁（`FOR UPDATE`），杜绝并发覆盖。
- **异步协程生命周期脱钩**：异步消息投递、事件处理等协程，必须使用 `context.WithoutCancel(ctx)` 解除超时绑定，严防 Context 泄漏与静默失败。
- **状态流转终态保护**：流转与撤回操作必须遵循工单状态机，禁止覆盖终态。

## 4. 现代 Go 编码习惯与代码质量
- **集合处理规范**：数据变换、切片映射、过滤与去重**优先使用 `github.com/samber/lo`**（如 `lo.Map`、`lo.Find`、`lo.Uniq`、`lo.KeyBy`、`lo.Associate`），杜绝手写低效的 $O(N^2)$ 线性循环或状态 map。
- **命名与接口规范**：
  - 核心接口以 `I` 开头（如 `IWorkflowCoreService`），实现结构体小写不导出。
  - 变量/函数使用 `camelCase`，常量使用 `UPPER_SNAKE_CASE`，文件名使用 `kebab-case`。
- **语言与注释风格**：所有回复、文档、代码注释**必须使用简体中文**；注释重点说明**“为什么这样设计”**，禁止无意义复述代码。
- **错误包装**：跨层返回错误必须使用 `fmt.Errorf("...: %w", err)` 携带上下文并保留根因。

## 5. 构建工具链与测试约束（强制）
- **构建与代码生成优先**：
  - 权限契约与文档生成：`permgen`
  - 依赖整理：`go mod tidy`
  - 依赖注入：变更 `ioc/` 依赖后运行 `wire ./ioc`
- **单元测试规范**：
  - Service / Repo 单元测试统一采用 **Table-Driven（表驱动测试 `testCases := []struct{...}`）**。
  - Mock 依赖使用 `go.uber.org/mock/gomock`，Mock 代码集中存放于各自包的 `mocks/` 子目录中。
  - 代码提交前必须保证 `go test ./...` 100% 通过。
