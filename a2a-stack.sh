#!/bin/bash
# A2A 全栈启动/停止脚本
# Usage: ./a2a-stack.sh start|stop|status

set -e

ACTION="${1:-start}"
COMPOSE_DIR="$HOME/work/a2a-platform-go"
BRIDGES_DIR="$COMPOSE_DIR/bridges"
LOG_DIR="$HOME/.local/share/a2a-stack"
PID_DIR="$LOG_DIR"

mkdir -p "$LOG_DIR"

# 确保 PATH 包含 node
export PATH="$HOME/node/node-v24.12.0-linux-x64/bin:$PATH"

start() {
    echo "=== Starting A2A Stack ==="

    # 1. Docker compose (MySQL + Platform)
    echo "[1/4] Starting Docker compose..."
    cd "$COMPOSE_DIR"
    docker compose up -d 2>&1 | tail -5

    # 等待平台就绪
    echo "[1/4] Waiting for platform..."
    for i in $(seq 1 30); do
        if curl -s http://localhost:18090/health 2>/dev/null | grep -q ok; then
            echo "  Platform ready."
            break
        fi
        sleep 2
    done

    # 2. OpenCode headless server
    if curl -s http://localhost:4096/session >/dev/null 2>&1; then
        echo "[2/4] OpenCode headless already running on :4096"
    else
        echo "[2/4] Starting OpenCode headless server..."
        nohup opencode serve --port 4096 --hostname 0.0.0.0 --pure \
            >> "$LOG_DIR/opencode-headless.log" 2>&1 &
        echo $! > "$PID_DIR/opencode-headless.pid"
        sleep 3
        if curl -s http://localhost:4096/session >/dev/null 2>&1; then
            echo "  OpenCode headless ready on :4096"
        else
            echo "  WARNING: OpenCode headless may not be ready yet"
        fi
    fi

    # 3. opencode bridge
    if curl -s http://localhost:10006/.well-known/agent.json >/dev/null 2>&1; then
        echo "[3/4] opencode bridge already running on :10006"
    else
        echo "[3/4] Starting opencode bridge..."
        nohup a2a-bridge -c "$BRIDGES_DIR/opencode-bridge.yaml" \
            >> "$LOG_DIR/opencode-bridge.log" 2>&1 &
        echo $! > "$PID_DIR/opencode-bridge.pid"
        sleep 2
        echo "  opencode bridge ready on :10006"
    fi

    # 4. hermes bridge
    if curl -s http://localhost:10004/.well-known/agent.json >/dev/null 2>&1; then
        echo "[4/4] hermes bridge already running on :10004"
    else
        echo "[4/4] Starting hermes bridge..."
        nohup a2a-bridge -c "$BRIDGES_DIR/hermes-bridge.yaml" \
            >> "$LOG_DIR/hermes-bridge.log" 2>&1 &
        echo $! > "$PID_DIR/hermes-bridge.pid"
        sleep 2
        echo "  hermes bridge ready on :10004"
    fi

    # 5. 确保所有 agents 都注册了
    echo ""
    echo "=== Registering agents ==="
    for agent_name in hermes opencode; do
        case $agent_name in
            hermes) port=10004 ;;
            opencode) port=10006 ;;
        esac
        status=$(curl -s http://localhost:18090/api/agents | python3 -c "
import sys,json
agents=json.load(sys.stdin)
for a in agents:
    if a['name']=='$agent_name':
        print(a['status'])
        break
else:
    print('not_found')
" 2>/dev/null)
        if [ "$status" = "connected" ]; then
            echo "  $agent_name: already connected"
        else
            echo "  Registering $agent_name..."
            # 删旧记录
            docker exec a2a-mysql mysql -ua2a -pa2a_secret_2024 a2a_platform \
                -e "DELETE FROM agents WHERE name='$agent_name';" 2>/dev/null
            curl -s -X POST http://localhost:18090/api/agents \
                -H "Content-Type: application/json" \
                -d "{\"name\":\"$agent_name\",\"type\":\"bridge\",\"url\":\"http://127.0.0.1:$port\",\"port\":$port}" \
                --max-time 15 2>&1
            echo ""
        fi
    done

    echo ""
    echo "=== A2A Stack Status ==="
    status_report
}

stop() {
    echo "=== Stopping A2A Stack ==="

    # 停 bridges
    for pidfile in "$PID_DIR"/*.pid; do
        [ -f "$pidfile" ] || continue
        name=$(basename "$pidfile" .pid)
        pid=$(cat "$pidfile")
        if kill -0 "$pid" 2>/dev/null; then
            echo "  Stopping $name (PID $pid)..."
            kill "$pid" 2>/dev/null || true
        fi
        rm -f "$pidfile"
    done

    # 停 opencode serve（如果不在 pid 文件里）
    pkill -f "opencode serve" 2>/dev/null && echo "  Stopped opencode serve" || true

    # 停 docker compose
    echo "  Stopping docker compose..."
    cd "$COMPOSE_DIR" && docker compose down 2>&1 | tail -3

    echo "  Done."
}

status_report() {
    echo "Services:"
    for port_name in "4096:OpenCode Headless" "10006:opencode bridge" "10004:hermes bridge" "18090:A2A Platform" "13306:MySQL"; do
        port="${port_name%%:*}"
        name="${port_name#*:}"
        if curl -s --max-time 2 "http://localhost:$port" >/dev/null 2>&1 || ss -tlnp | grep -q ":$port "; then
            echo "  ✅ $name (:$port)"
        else
            echo "  ❌ $name (:$port)"
        fi
    done
    echo ""
    echo "A2A Agents:"
    curl -s http://localhost:18090/api/agents 2>/dev/null | python3 -c "
import sys,json
try:
    agents=json.load(sys.stdin)
    for a in agents:
        icon = '✅' if a['status']=='connected' else '❌'
        print(f'  {icon} {a[\"name\"]} — {a[\"status\"]}')
except: print('  (platform not reachable)')
" 2>/dev/null
}

case "$ACTION" in
    start) start ;;
    stop) stop ;;
    status) status_report ;;
    restart) stop; sleep 2; start ;;
    *) echo "Usage: $0 {start|stop|status|restart}" ;;
esac
