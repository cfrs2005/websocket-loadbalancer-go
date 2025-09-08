# WebSocket负载均衡系统 - 服务器管理指令

## 📋 目录
- [系统概览](#系统概览)
- [服务器管理](#服务器管理)
- [客户端管理](#客户端管理)
- [故障转移测试](#故障转移测试)
- [监控和API](#监控和api)
- [常见问题](#常见问题)

## 🏗️ 系统概览

### 架构图
```
多个客户端 ──→ 负载均衡器(8080) ──→ 多个服务端
    ↓                ↓                    ↓
  客户端A         Web管理界面          node1(8081)
  客户端B         API接口             node2(8082)
  客户端C         健康检查             node3(8083)
```

### 端口分配
- **负载均衡器**: 8080
- **服务端节点1**: 8081 (node1)
- **服务端节点2**: 8082 (node2)
- **服务端节点3**: 8083 (node3)

## 🖥️ 服务器管理

### 启动完整系统
```bash
# 一键启动所有组件
./start-loadbalancer.sh

# 手动启动负载均衡器
./websocket-system -service=loadbalancer -port=8080 -strategy=round_robin

# 手动启动单个服务端节点
./websocket-system -service=server -mode=single -port=8081 -node=node1 &
./websocket-system -service=server -mode=single -port=8082 -node=node2 &
./websocket-system -service=server -mode=single -port=8083 -node=node3 &
```

### 关闭服务器节点

#### 🛑 关闭8081服务器 (node1)
```bash
# 方法1：通过端口查找并关闭
lsof -ti:8081 | xargs kill

# 方法2：如果知道PID
kill <PID>

# 方法3：强制关闭
lsof -ti:8081 | xargs kill -9

# 验证关闭
lsof -i:8081  # 应该没有输出
```

#### 🛑 关闭8082服务器 (node2)
```bash
# 方法1：通过端口查找并关闭（推荐）
lsof -ti:8082 | xargs kill

# 方法2：如果知道PID
kill <PID>

# 方法3：强制关闭
lsof -ti:8082 | xargs kill -9

# 验证关闭
lsof -i:8082  # 应该没有输出
```

#### 🛑 关闭8083服务器 (node3)
```bash
# 方法1：通过端口查找并关闭
lsof -ti:8083 | xargs kill

# 方法2：如果知道PID
kill <PID>

# 方法3：强制关闭
lsof -ti:8083 | xargs kill -9

# 验证关闭
lsof -i:8083  # 应该没有输出
```

### 启动服务器节点

#### ✅ 启动8081服务器 (node1)
```bash
# 标准启动
./websocket-system -service=server -mode=single -port=8081 -node=node1 &

# 后台启动并记录日志
nohup ./websocket-system -service=server -mode=single -port=8081 -node=node1 > logs/node1.log 2>&1 & echo "Node1 PID: $!"

# 验证启动
curl -s http://localhost:8081/health
```

#### ✅ 启动8082服务器 (node2)
```bash
# 标准启动
./websocket-system -service=server -mode=single -port=8082 -node=node2 &

# 后台启动并记录日志
nohup ./websocket-system -service=server -mode=single -port=8082 -node=node2 > logs/node2.log 2>&1 & echo "Node2 PID: $!"

# 验证启动
curl -s http://localhost:8082/health
```

#### ✅ 启动8083服务器 (node3)
```bash
# 标准启动
./websocket-system -service=server -mode=single -port=8083 -node=node3 &

# 后台启动并记录日志
nohup ./websocket-system -service=server -mode=single -port=8083 -node=node3 > logs/node3.log 2>&1 & echo "Node3 PID: $!"

# 验证启动
curl -s http://localhost:8083/health
```

## 👥 客户端管理

### 启动客户端
```bash
# 启动客户端A
./websocket-system -service=client -name=客户端A &

# 启动客户端B
./websocket-system -service=client -name=客户端B &

# 启动客户端C
./websocket-system -service=client -name=客户端C &

# 启动自定义名称的客户端
./websocket-system -service=client -name="我的测试客户端" &
```

### 客户端连接验证
```bash
# 查看所有连接的客户端
curl -s http://localhost:8080/api/clients | python3 -m json.tool

# 查询特定客户端名字（需要客户端ID）
curl -s "http://localhost:8080/api/query?client_id=<CLIENT_ID>" | python3 -m json.tool
```

## 🧪 故障转移测试

### 完整的故障转移测试流程

#### 测试node2(8082)故障转移
```bash
# 1. 查看当前系统状态
echo "=== 当前客户端分布 ==="
curl -s http://localhost:8080/api/clients | python3 -m json.tool

echo -e "\n=== 当前服务器状态 ==="
curl -s http://localhost:8080/api/backends | python3 -m json.tool

# 2. 关闭8082服务器
echo -e "\n🛑 关闭8082服务器..."
lsof -ti:8082 | xargs kill
echo "✅ 8082服务器已关闭"

# 3. 等待健康检查器检测（10-15秒）
echo "⏳ 等待健康检查器检测..."
sleep 12

# 4. 查看服务器状态变化
echo "=== 8082关闭后的服务器状态 ==="
curl -s http://localhost:8080/api/backends | python3 -m json.tool

# 5. 启动新客户端测试故障转移
echo -e "\n🚀 启动测试客户端验证故障转移..."
./websocket-system -service=client -name=故障转移测试客户端 &
TEST_CLIENT_PID=$!

# 6. 等待客户端连接
sleep 3

# 7. 查看新客户端的分配情况
echo "=== 新客户端分配情况 ==="
curl -s http://localhost:8080/api/clients | python3 -m json.tool

# 8. 重启8082服务器
echo -e "\n✅ 重启8082服务器..."
./websocket-system -service=server -mode=single -port=8082 -node=node2 &
NEW_NODE2_PID=$!
echo "Node2重启，新PID: $NEW_NODE2_PID"

# 9. 等待健康检查器检测恢复
echo "⏳ 等待健康检查器检测恢复..."
sleep 12

# 10. 查看最终状态
echo "=== 8082恢复后的服务器状态 ==="
curl -s http://localhost:8080/api/backends | python3 -m json.tool

echo -e "\n=== 最终客户端分布 ==="
curl -s http://localhost:8080/api/clients | python3 -m json.tool

# 11. 清理测试客户端
kill $TEST_CLIENT_PID 2>/dev/null
echo -e "\n✅ 故障转移测试完成"
```

#### 批量故障转移测试
```bash
# 依次关闭和重启所有服务器测试
for port in 8081 8082 8083; do
    echo "=== 测试端口 $port ==="
    
    # 关闭服务器
    lsof -ti:$port | xargs kill
    echo "关闭端口 $port"
    
    # 等待检测
    sleep 12
    
    # 查看状态
    curl -s http://localhost:8080/api/backends | python3 -m json.tool | grep -A5 -B5 $port
    
    # 重启服务器
    node_name="node$((port-8080))"
    ./websocket-system -service=server -mode=single -port=$port -node=$node_name &
    echo "重启端口 $port"
    
    # 等待恢复
    sleep 12
    echo "等待恢复完成"
    echo
done
```

## 📊 监控和API

### 健康检查
```bash
# 负载均衡器健康检查
curl -s http://localhost:8080/health | python3 -m json.tool

# 各服务端节点健康检查
curl -s http://localhost:8081/health | python3 -m json.tool
curl -s http://localhost:8082/health | python3 -m json.tool  
curl -s http://localhost:8083/health | python3 -m json.tool
```

### API接口
```bash
# 获取客户端列表
curl -s http://localhost:8080/api/clients | python3 -m json.tool

# 获取后端服务器状态
curl -s http://localhost:8080/api/backends | python3 -m json.tool

# 查询特定客户端
curl -s "http://localhost:8080/api/query?client_id=<CLIENT_ID>" | python3 -m json.tool
```

### Web管理界面
```bash
# 打开Web管理界面
open http://localhost:8080/web-loadbalancer.html

# 或者使用curl测试界面可访问性
curl -I http://localhost:8080/web-loadbalancer.html
```

### 端口状态检查
```bash
# 检查所有相关端口状态
echo "=== 端口使用情况 ==="
lsof -i:8080  # 负载均衡器
lsof -i:8081  # node1
lsof -i:8082  # node2
lsof -i:8083  # node3

# 简化显示
echo -e "\n=== 简化端口检查 ==="
for port in 8080 8081 8082 8083; do
    if lsof -i:$port >/dev/null 2>&1; then
        echo "✅ 端口 $port: 正在使用"
    else
        echo "❌ 端口 $port: 未使用"
    fi
done
```

## ❓ 常见问题

### 端口被占用
```bash
# 查看端口占用情况
lsof -i:8080
lsof -i:8081
lsof -i:8082
lsof -i:8083

# 强制清理所有端口
for port in 8080 8081 8082 8083; do
    lsof -ti:$port | xargs kill -9 2>/dev/null
    echo "清理端口 $port"
done
```

### 系统完全重启
```bash
# 停止所有相关进程
pkill -f websocket-system
pkill -f start-loadbalancer

# 等待进程完全退出
sleep 3

# 重新启动系统
./start-loadbalancer.sh
```

### 查看系统日志
```bash
# 查看负载均衡器日志
journalctl -f | grep loadbalancer

# 查看特定端口的连接
ss -tlnp | grep :8080
ss -tlnp | grep :8081
ss -tlnp | grep :8082
ss -tlnp | grep :8083
```

### 性能监控
```bash
# 监控连接数
watch -n 2 'curl -s http://localhost:8080/api/clients | jq ".total"'

# 监控服务器状态
watch -n 5 'curl -s http://localhost:8080/api/backends | jq ".backends[] | {id: .id, healthy: .is_healthy, connections: .connections}"'
```

## 🎯 快速参考

### 常用组合命令
```bash
# 快速重启node2
lsof -ti:8082 | xargs kill && sleep 2 && ./websocket-system -service=server -mode=single -port=8082 -node=node2 &

# 查看完整系统状态
echo "客户端:" && curl -s http://localhost:8080/api/clients | jq ".total" && echo "服务器:" && curl -s http://localhost:8080/api/backends | jq ".backends[] | select(.is_healthy == true) | .id"

# 一键测试所有API
curl -s http://localhost:8080/health && curl -s http://localhost:8080/api/clients && curl -s http://localhost:8080/api/backends
```

---

**📝 文档更新日期**: $(date "+%Y-%m-%d %H:%M:%S")  
**🔧 维护者**: WebSocket负载均衡系统团队  
**📧 支持**: 如有问题请查看系统日志或联系开发团队