# eflow

eflow 是工单流程编排服务，负责工单创建、审批流实例流转、自动化任务触发、消息通知模板同步，以及历史数据迁移。项目基于 Go、Gin、GORM、easy-workflow、Kafka、Etcd 和 gRPC 构建，并与 eiam、ecmdb、etask、ealert/enotify 等服务协同工作。

## 功能概览

- 工单模板、工单实例、流程定义和流程实例管理
- 基于 easy-workflow 的审批流驱动和节点事件处理
- 自动化节点任务创建、重试、定时触发和执行结果同步
- 通过 etask 下发脚本任务，支持 Kafka 和 gRPC 执行模式

## 技术栈

- Go 1.26
- Gin / ginx
- GORM / MySQL
- Redis
- Kafka
- Etcd
- gRPC / Protobuf
- Wire
- Cobra / Viper
- easy-workflow

## 目录结构

```text
.
├── api/                  # protobuf 定义和生成代码
├── cmd/                  # Cobra 子命令：server、migrate、sync
├── config/               # 本地配置文件
├── deploy/               # 部署示例
├── internal/
│   ├── client/           # 外部服务 gRPC/HTTP 客户端
│   ├── domain/           # 领域对象
│   ├── event/            # 流程、工单、任务事件定义
│   ├── repository/       # 仓储和 DAO
│   ├── service/          # 核心业务服务
│   └── web/              # HTTP handler
├── ioc/                  # Wire 依赖注入
├── migrations/           # 数据库迁移脚本
├── pkg/                  # 通用工具包
├── main.go               # CLI 入口
└── README.md
```

## 环境依赖

运行服务前需要准备：

- MySQL，用于 eflow 主库和 easy-workflow 表
- Redis，用于缓存和运行时协作
- Kafka，用于部分自动化任务下发
- Etcd，用于 gRPC 服务发现
- eiam，用于认证、租户和用户信息
- etask，用于自动化脚本调度和执行
- ecmdb、ealert/enotify，按实际业务场景配置

## 配置

默认配置文件路径为 `config/config.yaml`，也可以通过 `--config` 指定。

```bash
go run main.go server --config=config/config.yaml
```

核心配置项：

```yaml
log:
  debug: true

web:
  host: "0.0.0.0"
  port: 8005

mysql:
  dsn: "<user>:<password>@tcp(<host>:3306)/ticket?charset=utf8mb4&parseTime=True&loc=Local"

redis:
  addr: "<host>:6379"
  password: "<password>"
  db: 0

kafka:
  network: tcp
  addresses:
    - "<host>:9092"

etcd:
  endpoints:
    - "<host>:2379"

encryption:
  version: "V1"
  key: "<shared-secret-key>"

grpc:
  client:
    eiam:
      name: "eiam"
      auth_token: "<token>"
    ecmdb:
      name: "ecmdb"
      auth_token: "<token>"
    etask:
      name: "scheduler"
      auth_token: "<token>"
```

注意：

- `encryption.key` 需要和历史敏感数据的加密 key 保持一致，否则迁移后的密文变量无法正确解密。
- `encryption.version` 当前用于兼容旧格式密文，迁移旧数据时通常使用 `V1`。
- 不要把真实密码、飞书/企微密钥、数据库 DSN 提交到公共仓库。

## 本地运行

安装依赖并启动：

```bash
go mod download
go run main.go server --config=config/config.yaml
```

也可以使用 Makefile：

```bash
make run
```

服务启动后会：

- 初始化 IOC 容器
- 注册 easy-workflow 事件处理器
- 启动后台任务
- 启动 HTTP/gRPC 等 server

## 常用命令

```bash
# 启动服务
go run main.go server --config=config/config.yaml

# 编译检查
go build ./...

# 运行测试
go test ./...

# 生成 Wire 依赖注入代码
wire ./ioc/

# 生成 mock
go generate ./...

# 同步全局通知模板
go run main.go sync template --config=config/config.yaml
```

如果安装了 Task：

```bash
task run
task gen
task mock
```

## 数据迁移

迁移命令：

```bash
go run main.go migrate --config=config/config.yaml
```

强制重新执行迁移：

```bash
go run main.go migrate --config=config/config.yaml --force
```

迁移配置位于 `migration` 节点：

```yaml
migration:
  source:
    mongo:
      dsn: "<mongo-dsn>"
      database: "<mongo-db>"
    mysql:
      dsn: "<source-mysql-dsn>"
  batch_size: 100
  timeout: "10m"
  auto_migrate: true
  reset_auto_increment: true
  truncate: false
  dry_run: false
```

迁移会执行：

- 旧数据复制和结构转换
- 将当前目标 MySQL 中持续运行过的旧 `task` 表安全拆分为 `automation_tasks` 和 `automation_task_attempts`
- 自动建表
- easy-workflow 引擎表初始化
- process instance 自增 ID 同步
- 任务、工作流、实例流程中的 codebook 关联修复

建议先设置 `dry_run: true` 验证迁移流程，再对目标库执行正式迁移。

旧 `task` 表是早期 Mongo 迁移的落库结果，并且旧版本运行期间仍会持续写入。二次拆表迁移直接读取
当前 `mysql.dsn`，不会重新从 Mongo 或 `migration.source.mysql.dsn` 生成任务。
该步骤使用独立迁移记录 `automation_task_v2_mysql`，不会受到早期 Mongo Task 迁移记录影响。

旧自动化任务迁移遵循安全模式：成功任务保留结果并标记为已推进，其他状态统一迁移为阻塞，
不会在新服务启动后自动续跑。旧 `external_id` 是 etask Task ID，不会被错误地当作新 execution ID。
历史节点名称统一显示为“自动化任务”，不再跨表追溯流程快照。缺少工单、流程实例或节点 ID 的脏数据
会保留在旧表并跳过，迁移汇总会输出数量和样例；重复业务键只保留已迁移的一条，不会覆盖已有记录。
正式迁移前应停止仍会写入旧 `task` 表的 eflow 服务。迁移器会检查任务 ID 和更新时间水位，
检测到迁移期间仍有新增或更新时不会写入完成记录；停服后可直接幂等重跑。
对已经运行并产生新数据的 MySQL 执行二次拆表时，必须保持 `truncate: false`，避免清空其他已迁移业务表。
迁移完成后请保留旧 `task` 表至少一个发布周期。

## 自动化任务变量

eflow 创建或重试自动化任务时，只在 Attempt 中保存业务输入快照。Runner 默认变量和敏感变量由
etask 在正式提交时统一合并，eflow 不再保存脚本源码、执行目标或变量明文。

相关链路：

- `internal/service/task/preparation.go`：组装业务输入并选择 Runner
- `internal/client/etask/submission.go`：通过统一 Scheduler 协议幂等提交执行
- `internal/repository/dao/task_attempt.go`：保存每次执行输入和外部 execution 引用

敏感变量由 etask 管理，eflow 不允许通过任务变量接口新增或覆盖敏感变量明文。

## 部署

`deploy/docker-compose.yaml` 提供了容器部署示例：

```bash
docker compose -f deploy/docker-compose.yaml up -d
```

部署时通常需要挂载生产配置：

```yaml
volumes:
  - ./prod.yaml:/app/config/config.yaml
```

请确认容器所在网络可以访问 MySQL、Redis、Kafka、Etcd 和依赖的内部服务。
