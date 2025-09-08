# 🚀 WebSocket负载均衡系统 - 快速参考卡片

## 📋 一键启动
```bash
./start-loadbalancer.sh
```

## 🛑 关闭服务器节点

| 节点 | 端口 | 关闭命令 |
|------|------|----------|
| node1 | 8081 | `lsof -ti:8081 \| xargs kill` |
| node2 | 8082 | `lsof -ti:8082 \| xargs kill` |
| node3 | 8083 | `lsof -ti:8083 \| xargs kill` |

## ✅ 启动服务器节点

| 节点 | 端口 | 启动命令 |
|------|------|----------|
| node1 | 8081 | `./websocket-system -service=server -mode=single -port=8081 -node=node1 &` |
| node2 | 8082 | `./websocket-system -service=server -mode=single -port=8082 -node=node2 &` |
| node3 | 8083 | `./websocket-system -service=server -mode=single -port=8083 -node=node3 &` |

## 👥 客户端管理
```bash
# 启动客户端
./websocket-system -service=client -name=客户端A &
./websocket-system -service=client -name=客户端B &
./websocket-system -service=client -name=客户端C &
```

## 📊 监控命令

| 功能 | 命令 |
|------|------|
| 查看客户端列表 | `curl -s http://localhost:8080/api/clients \| python3 -m json.tool` |
| 查看服务器状态 | `curl -s http://localhost:8080/api/backends \| python3 -m json.tool` |
| 查询客户端名字 | `curl -s "http://localhost:8080/api/query?client_id=<ID>" \| python3 -m json.tool` |
| 负载均衡器健康检查 | `curl -s http://localhost:8080/health` |

## 🧪 故障转移测试 (node2为例)

### 快速测试
```bash
# 1. 关闭node2
lsof -ti:8082 | xargs kill

# 2. 等待健康检查(12秒)
sleep 12

# 3. 查看状态(应该显示node2不健康)
curl -s http://localhost:8080/api/backends | python3 -m json.tool

# 4. 重启node2
./websocket-system -service=server -mode=single -port=8082 -node=node2 &

# 5. 等待恢复(12秒)
sleep 12

# 6. 查看状态(应该显示node2恢复健康)
curl -s http://localhost:8080/api/backends | python3 -m json.tool
```

## 🌐 Web界面
```bash
# 打开管理界面
open http://localhost:8080/web-loadbalancer.html

# 或在浏览器中访问
http://localhost:8080/web-loadbalancer.html
```

## 🔧 系统端口

| 服务 | 端口 | 用途 |
|------|------|------|
| 负载均衡器 | 8080 | 客户端连接入口 + Web管理界面 |
| node1 | 8081 | 后端服务器1 |
| node2 | 8082 | 后端服务器2 |
| node3 | 8083 | 后端服务器3 |

## 🆘 紧急命令

### 清理所有进程
```bash
pkill -f websocket-system
pkill -f start-loadbalancer
```

### 清理所有端口
```bash
for port in 8080 8081 8082 8083; do
    lsof -ti:$port | xargs kill -9 2>/dev/null
done
```

### 检查端口状态
```bash
for port in 8080 8081 8082 8083; do
    if lsof -i:$port >/dev/null 2>&1; then
        echo "✅ 端口 $port: 正在使用"
    else
        echo "❌ 端口 $port: 未使用"
    fi
done
```

## 📈 实时监控
```bash
# 监控客户端数量
watch -n 2 'curl -s http://localhost:8080/api/clients | jq ".total"'

# 监控服务器健康状态  
watch -n 5 'curl -s http://localhost:8080/api/backends | jq ".backends[] | {id: .id, healthy: .is_healthy, connections: .connections}"'
```

## 💡 常用组合

### 重启特定节点
```bash
# 重启node2的完整流程
lsof -ti:8082 | xargs kill && sleep 2 && ./websocket-system -service=server -mode=single -port=8082 -node=node2 &
```

### 系统状态概览
```bash
echo "=== 系统状态概览 ==="
echo "客户端数量: $(curl -s http://localhost:8080/api/clients | jq '.total')"
echo "健康服务器: $(curl -s http://localhost:8080/api/backends | jq '.backends[] | select(.is_healthy == true) | .id' | wc -l)"
echo "总服务器数: $(curl -s http://localhost:8080/api/backends | jq '.backends | length')"
```

---
**⚡ 快速帮助**: 详细文档请查看 `docs/server-management.md`