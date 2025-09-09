#!/bin/bash

# WebSocket负载均衡系统启动脚本
# 正确的架构：客户端 -> 负载均衡器 -> 多个服务端

echo "🚀 WebSocket负载均衡系统启动"
echo "========================================"
echo ""

# 检查依赖
check_dependencies() {
    echo "📋 检查系统依赖..."

    # 检查Go
    if ! command -v go &> /dev/null; then
        echo "❌ 未安装Go语言环境"
        echo "请访问 https://golang.org/dl/ 下载并安装Go"
        exit 1
    fi
    echo "✅ Go $(go version | cut -d' ' -f3)"
    echo ""
}

# 编译程序
build_program() {
    echo "🔧 编译Go程序..."
    
    if ! go build -o websocket-system .; then
        echo "❌ Go编译失败"
        exit 1
    fi
    echo "✅ Go程序编译成功"
    echo ""
}

# 启动后端服务器集群
start_backend_servers() {
    echo "🖥️ 启动后端服务器集群..."

    # 启动服务器节点1
    echo "启动服务端节点1 (端口8081)..."
    ./websocket-system -service=server -mode=single -port=8081 -node=node1 &
    NODE1_PID=$!
    echo "✅ 节点1已启动，PID: $NODE1_PID"

    # 启动服务器节点2
    echo "启动服务端节点2 (端口8082)..."
    ./websocket-system -service=server -mode=single -port=8082 -node=node2 &
    NODE2_PID=$!
    echo "✅ 节点2已启动，PID: $NODE2_PID"

    # 启动服务器节点3
    echo "启动服务端节点3 (端口8083)..."
    ./websocket-system -service=server -mode=single -port=8083 -node=node3 &
    NODE3_PID=$!
    echo "✅ 节点3已启动，PID: $NODE3_PID"

    echo ""
    echo "⏳ 等待服务端节点启动完成..."
    sleep 5

    # 检查服务器状态
    echo "📊 检查服务端节点状态..."
    check_server_health 8081 "节点1"
    check_server_health 8082 "节点2"
    check_server_health 8083 "节点3"
    
    echo ""
}

# 检查服务器健康状态
check_server_health() {
    local port=$1
    local name=$2
    
    if curl -s http://localhost:$port/health > /dev/null; then
        echo "✅ $name 健康检查通过 (端口$port)"
    else
        echo "❌ $name 健康检查失败 (端口$port)"
    fi
}

# 启动负载均衡器
start_loadbalancer() {
    echo "⚖️ 启动负载均衡器..."
    
    echo "负载均衡器配置:"
    echo "  • 端口: 8080"
    echo "  • 策略: round_robin"
    echo "  • 后端服务器: node1(8081), node2(8082), node3(8083)"
    echo ""
    
    ./websocket-system -service=loadbalancer -port=8080 -strategy=round_robin &
    LB_PID=$!
    echo "✅ 负载均衡器已启动，PID: $LB_PID"
    
    echo ""
    echo "⏳ 等待负载均衡器启动完成..."
    sleep 3
    
    # 检查负载均衡器状态
    if curl -s http://localhost:8080/health > /dev/null; then
        echo "✅ 负载均衡器健康检查通过"
    else
        echo "❌ 负载均衡器健康检查失败"
    fi
    
    echo ""
}

# 显示系统信息
show_system_info() {
    echo "🎉 系统启动完成！"
    echo "===================="
    echo ""
    echo "📍 系统架构:"
    echo "  多个客户端 ──→ 负载均衡器(8080) ──→ 多个服务端"
    echo ""
    echo "🔗 访问地址:"
    echo "  🌐 负载均衡器入口: http://localhost:8080"
    echo "  🔌 客户端连接: ws://localhost:8080/ws"
    echo ""
    echo "🖥️ 后端节点独立管理界面:"
    echo "  • Node1: http://localhost:8081/web-node.html (PID: $NODE1_PID)"
    echo "  • Node2: http://localhost:8082/web-node.html (PID: $NODE2_PID)"
    echo "  • Node3: http://localhost:8083/web-node.html (PID: $NODE3_PID)"
    echo ""
    echo "📊 各节点API接口:"
    echo "  • Node1 客户端列表: http://localhost:8081/api/clients"
    echo "  • Node2 客户端列表: http://localhost:8082/api/clients"
    echo "  • Node3 客户端列表: http://localhost:8083/api/clients"
    echo ""
    echo "⚖️ 七层负载均衡器 (纯转发):"
    echo "  • 端口: 8080 (PID: $LB_PID)"
    echo "  • 功能: HTTP/WebSocket 请求转发 + 会话保持"
    echo ""
    echo "🧪 测试流程:"
    echo "  1. 启动多个客户端: ./websocket-system -service=client -name=客户端A"
    echo "  2. 再启动客户端B: ./websocket-system -service=client -name=客户端B"
    echo "  3. 访问负载均衡器: http://localhost:8080 (会自动转发到后端节点)"
    echo "  4. 打开各节点管理界面查看客户端分布:"
    echo "     • http://localhost:8081/web-node.html"
    echo "     • http://localhost:8082/web-node.html"
    echo "     • http://localhost:8083/web-node.html"
    echo "  5. 测试会话保持: 刷新页面应始终访问同一后端节点"
    echo "  6. 关闭某个服务端测试故障转移: kill $NODE1_PID"
    echo ""
    echo "🛑 停止系统:"
    echo "  kill $NODE1_PID $NODE2_PID $NODE3_PID $LB_PID"
    echo "  或者按 Ctrl+C"
    echo ""
    echo "💡 新架构特性验证:"
    echo "  ✅ 七层负载均衡器 (纯转发，类似nginx)"
    echo "  ✅ 每个后端节点独立Web管理界面"
    echo "  ✅ 会话保持机制 (同用户固定访问同一节点)"
    echo "  ✅ HTTP/WebSocket双重代理"
    echo "  ✅ 后端节点故障自动转移"
    echo "  ✅ 负载均衡器只做转发，不提供业务功能"
    echo "  ✅ 客户端连接到负载均衡器，自动分配到后端"
    echo "  ✅ 各节点独立显示客户端列表"
    echo ""
}

# 清理函数
cleanup() {
    echo ""
    echo "🧹 正在清理系统..."
    kill $NODE1_PID $NODE2_PID $NODE3_PID $LB_PID 2>/dev/null
    echo "✅ 系统已停止"
    exit 0
}

# 主函数
main() {
    # 捕获中断信号
    trap cleanup INT TERM

    # 检查依赖
    check_dependencies
    
    # 编译程序
    build_program

    # 启动后端服务器
    start_backend_servers

    # 启动负载均衡器
    start_loadbalancer

    # 显示系统信息
    show_system_info

    # 等待用户中断
    echo "按 Ctrl+C 停止系统..."
    wait
}

# 执行主函数
main