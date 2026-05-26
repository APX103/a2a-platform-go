#!/bin/bash
#
# 一键启动 A2A 管理前端 + Human Client
# 用法:
#   ./start-frontends.sh                    # 使用默认后端 http://localhost:28090
#   ./start-frontends.sh http://host:port   # 自定义后端地址
#   BACKEND_URL=http://host:port ./start-frontends.sh
#

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BACKEND_URL="${BACKEND_URL:-${1:-http://localhost:28090}}"
ADMIN_PORT=3001
HUMAN_PORT=5174

# 释放指定端口
free_port() {
    local port="$1"
    local name="$2"
    local pids
    pids=$(lsof -ti:"$port" 2>/dev/null || true)
    if [ -n "$pids" ]; then
        echo "⚠️  $name 端口 $port 被占用，正在释放..."
        echo "$pids" | xargs kill -9 2>/dev/null || true
        sleep 1
        echo "✅ 端口 $port 已释放"
    fi
}

echo "========================================"
echo "  A2A 前端启动脚本"
echo "========================================"
echo "  后端地址: $BACKEND_URL"
echo "  Admin:    http://localhost:$ADMIN_PORT"
echo "  Human:    http://localhost:$HUMAN_PORT"
echo "========================================"
echo ""

# 保存 PID 的目录
PID_DIR="$SCRIPT_DIR/.run"
mkdir -p "$PID_DIR"

# 先释放端口（防止旧进程占用导致 Vite 自动切换端口）
free_port "$ADMIN_PORT" "Admin"
free_port "$HUMAN_PORT" "Human Client"

# 清理之前的 PID 文件
rm -f "$PID_DIR/admin.pid" "$PID_DIR/human.pid"

# 安装依赖（如果 node_modules 不存在）
install_if_needed() {
    local dir="$1"
    local name="$2"
    if [ ! -d "$dir/node_modules" ]; then
        echo "📦 $name 依赖未安装，正在安装..."
        (cd "$dir" && npm install)
        echo "✅ $name 依赖安装完成"
    else
        echo "✅ $name 依赖已存在"
    fi
}

install_if_needed "$SCRIPT_DIR/web/admin" "Admin 前端"
install_if_needed "$SCRIPT_DIR/apps/human-client" "Human Client"

echo ""
echo "🚀 启动服务..."
echo ""

# 启动 Admin
echo "🟢 启动 Admin 前端 (http://localhost:$ADMIN_PORT) ..."
(
    cd "$SCRIPT_DIR/web/admin"
    VITE_DEV_API_PROXY="$BACKEND_URL" npm run dev > "$PID_DIR/admin.log" 2>&1 &
    echo $! > "$PID_DIR/admin.pid"
)

# 启动 Human Client
echo "🟢 启动 Human Client (http://localhost:$HUMAN_PORT) ..."
(
    cd "$SCRIPT_DIR/apps/human-client"
    VITE_A2A_PLATFORM_URL="$BACKEND_URL" npm run dev > "$PID_DIR/human.log" 2>&1 &
    echo $! > "$PID_DIR/human.pid"
)

echo ""
echo "========================================"
echo "  ✅ 服务已启动"
echo "========================================"
echo "  Admin 前端: http://localhost:$ADMIN_PORT"
echo "  Human Client: http://localhost:$HUMAN_PORT"
echo "  后端代理: $BACKEND_URL"
echo ""
echo "  日志文件:"
echo "    Admin:    $PID_DIR/admin.log"
echo "    Human:    $PID_DIR/human.log"
echo ""
echo "  停止命令: ./stop-frontends.sh"
echo "========================================"
