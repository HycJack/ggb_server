#!/bin/bash
# start.sh - 后台不回收模式启动二进制程序
# 用法：./start.sh

# 配置项
BINARY="./bin/ggb_server"  # 替换为你的二进制文件路径
LOG_FILE="nohup.out"     # 输出日志文件名
PID_FILE="ggb_server.pid"   # 记录进程ID的文件

# 检查程序是否已在运行
if [ -f "$PID_FILE" ]; then
    PID=$(cat "$PID_FILE")
    if ps -p "$PID" > /dev/null; then
        echo "⚠️ 程序已在运行 (PID: $PID)"
        exit 1
    else
        rm -f "$PID_FILE"  # 清理无效PID文件
    fi
fi

# 启动程序（关键命令）
nohup "$BINARY" > "$LOG_FILE" 2>&1 &
PID=$!

# 保存进程ID
echo "$PID" > "$PID_FILE"
echo "✅ 启动成功！PID: $PID"
echo "📋 日志输出: $LOG_FILE"