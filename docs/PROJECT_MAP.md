# A2A Platform 项目地图

这份文档用于快速理解仓库边界，也作为后续整理项目时的检查清单。

## 产品形态

这个项目是一个 Go 单体服务，内嵌 React 管理后台。

- `cmd/server`: 进程入口、HTTP 路由装配、中间件、优雅退出、SPA 服务。
- `internal/config`: YAML 配置加载、环境变量展开、默认值补齐。
- `internal/svc`: 数据库初始化、迁移、Store、Registry、共享依赖组装。
- `internal/handler`: Agent、Task、Context、Trace、Event、Builtin Agent、MCP SSE 等 HTTP handler。
- `internal/engine`: 内建 LLM Agent 循环、流式输出、工具调用、回复持久化。
- `internal/llm`: Provider 接口以及 OpenAI/Anthropic 适配。
- `internal/tools`: 内建工具、动态工具、子代理工具、任务工具。
- `internal/bridge`: 配置式 HTTP/CLI Bridge Agent。
- `internal/mcpclient`: MCP stdio/SSE 客户端。
- `internal/model`: 持久化与 API 类型。
- `internal/events`: 进程内 SSE 事件广播。
- `web/admin`: React/Vite 管理后台源码。
- `web`: Go embed 边界，负责嵌入前端资源。
- `etc`: 示例运行配置。
- `sql`: 外部数据库初始化 SQL。
- `tests/e2e`: 黑盒测试，需要本机 `localhost:18090` 已有运行中的服务。

## 构建与测试路径

- `make test` 运行后端单元测试，不要求先构建前端。
- `make build` 会先构建 React，再用 `-tags frontend` 编译 Go 二进制，把真实 `web/dist` 嵌进去。
- `go test ./tests/e2e` 不是单元测试；需要先启动平台。
- 本地单二进制开发优先使用 `etc/config-sqlite.yaml`。
- MySQL 形态优先使用 `docker compose up -d`。

## 当前主要混乱点

- `cmd/server/main.go` 职责偏多：路由、中间件、启动逻辑、辅助函数都在一个文件里。
- `internal/svc/servicecontext.go` 同时承担数据库连接、schema 字符串、迁移、依赖组装。
- `internal/handler/handler.go` 覆盖面太宽，后续改接口时建议按资源继续拆分。
- `docs/superpowers` 更像过程产物，`docs/specs` 和 `docs/plans` 更像项目文档；建议明确哪些是 canonical 文档。
- `a2a-stack.sh` 带有个人机器路径、固定工具、固定 bridge 名称；更适合作为本地便利脚本，而不是通用入口。
- 根目录 `SUMMARY.html` 看起来是生成文档；如果不是手写维护，建议移到 `docs/generated` 或不纳入源码。

## 建议整理顺序

1. 把 `cmd/server/main.go` 的路由注册拆到独立 router/server 组件。
2. 把 `internal/svc/servicecontext.go` 里的 schema 和迁移逻辑拆出去。
3. 按 API 资源继续拆分 `internal/handler/handler.go`。
4. 增加轻量 CI：后端跑 `make test`，前端跑 typecheck/lint。
5. 确定文档分层：README/USAGE/PROJECT_MAP 为入口，计划和生成 HTML 单独归档。
6. 将 `a2a-stack.sh` 参数化，或明确标注为个人本地脚本。
