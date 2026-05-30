.PHONY: default run endpoint gen mock build

default:
	@echo "Available commands:"
	@echo "  make run      - 启动流程引擎服务"
	@echo "  make endpoint - 注入 web 路由端点"
	@echo "  make gen      - 生成 Wire 依赖注入"
	@echo "  make mock     - 生成 Mock 代码"
	@echo "  make build    - 编译项目"

run:
	EGO_DEBUG=true go run main.go server --config=config/config.yaml

endpoint:
	EGO_DEBUG=true go run main.go endpoint --config=config/config.yaml

gen:
	wire ./ioc/

mock:
	go generate ./...

build:
	go build ./...
