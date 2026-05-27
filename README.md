# A2A Platform Frontend

A2A 平台前端项目，包含 Admin 管理端和 Human Client 人类客户端。

## 项目结构

```
├── apps/admin/          # Admin 管理前端 (React + Vite)
├── apps/human-client/  # Human Client 人类客户端 (React + Vite)
├── bridges/            # Bridge 脚本示例
├── docs/               # 文档
└── tests/              # 测试
```

## 前端项目

### Admin 管理端 (`apps/admin/`)

- 端口: `3001`
- 功能: Agent 管理、Group 管理、任务追踪、聊天记录、系统监控
- 技术栈: React 19 + Vite + Tailwind CSS + Zustand

### Human Client (`apps/human-client/`)

- 端口: `5174`
- 功能: 人类加入 A2A Group、IM 式群聊、P2P 聊天
- 技术栈: React 19 + Vite

## 快速开始

### 一键启动

```bash
# 默认后端 http://localhost:28090
./start-frontends.sh

# 自定义后端地址
./start-frontends.sh http://your-backend:port

# 停止
./stop-frontends.sh
```

### 手动启动

```bash
# Admin
 cd apps/admin
 npm install
 VITE_DEV_API_PROXY=http://localhost:28090 npm run dev

# Human Client
 cd apps/human-client
 npm install
 VITE_A2A_PLATFORM_URL=http://localhost:28090 npm run dev
```

## 环境变量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `BACKEND_URL` | `http://localhost:28090` | 后端 API 地址（脚本用） |
| `VITE_DEV_API_PROXY` | `http://localhost:18090` | Admin 后端代理地址 |
| `VITE_A2A_PLATFORM_URL` | `http://127.0.0.1:18090` | Human Client 后端地址 |

## 部署

构建为静态文件后，任何静态服务器均可托管：

```bash
# Admin
cd apps/admin
npm run build
# 输出: web/dist/

# Human Client
cd apps/human-client
VITE_A2A_PLATFORM_URL=https://your-api.com npm run build
# 输出: apps/human-client/dist/
```
