#!/bin/bash

echo "=== 测试 Dashboard API ==="
echo ""

# 启动示例程序
cd /workspace/examples/dashboard
go run main.go &
PID=$!
echo "启动 Dashboard 示例程序，PID: $PID"

# 等待服务器启动
sleep 3

echo ""
echo "1. 测试 /api/stats - 统计信息"
curl -s http://localhost:8080/api/stats | jq '.'

echo ""
echo "2. 测试 /api/tasks - 任务列表"
curl -s http://localhost:8080/api/tasks | jq '.[0]'

echo ""
echo "3. 等待一些任务执行..."
sleep 8

echo ""
echo "4. 测试 /api/history - 历史记录（显示前2条）"
curl -s "http://localhost:8080/api/history?limit=2" | jq '.records[0]'

echo ""
echo "5. 再次查看统计信息（应该显示真实的执行时长）"
curl -s http://localhost:8080/api/stats | jq '{avgDuration, totalDuration, historyRecords}'

echo ""
echo "=== 测试完成，停止程序 ==="
kill $PID 2>/dev/null
