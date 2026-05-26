#!/bin/bash
#
# 停止 A2A 前端服务
#

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PID_DIR="$SCRIPT_DIR/.run"

stop_service() {
    local name="$1"
    local pid_file="$PID_DIR/$2"

    if [ -f "$pid_file" ]; then
        local pid
        pid=$(cat "$pid_file" 2>/dev/null || true)
        if [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null; then
            echo "🛑 停止 $name (pid $pid)..."
            kill "$pid" 2>/dev/null || true
            sleep 1
            # 如果还在，强制杀
            if kill -0 "$pid" 2>/dev/null; then
                kill -9 "$pid" 2>/dev/null || true
            fi
            echo "✅ $name 已停止"
        else
            echo "⚠️  $name 未在运行"
        fi
        rm -f "$pid_file"
    else
        echo "⚠️  $name 未找到 PID 文件"
    fi
}

echo "========================================"
echo "  停止 A2A 前端服务"
echo "========================================"
echo ""

stop_service "Admin 前端" "admin.pid"
stop_service "Human Client" "human.pid"

echo ""
echo "========================================"
echo "  ✅ 全部已停止"
echo "========================================"
