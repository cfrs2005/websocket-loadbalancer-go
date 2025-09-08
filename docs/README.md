# 📚 WebSocket负载均衡系统 - 文档中心

## 🎯 项目概述

这是一个纯Go实现的WebSocket负载均衡系统，支持多客户端连接到多个后端服务器，具备故障转移、健康检查和Web管理界面等功能。

### 🏗️ 系统架构
```
多个客户端 ──→ 负载均衡器(8080) ──→ 多个服务端
    ↓                ↓                    ↓
  客户端A         Web管理界面          node1(8081)
  客户端B         API接口             node2(8082)  
  客户端C         健康检查             node3(8083)
```

## 📖 文档导航

### 🚀 快速开始
- **[快速参考卡片](quick-reference.md)** - 常用命令和操作的快速参考
- **[服务器管理指南](server-management.md)** - 详细的服务器管理文档
- **[API接口文档](api-reference.md)** - 完整的API接口说明

### 📋 文档列表

| 文档名称 | 描述 | 适用对象 |
|----------|------|----------|
| [quick-reference.md](quick-reference.md) | 快速参考卡片，包含常用命令 | 所有用户 |
| [server-management.md](server-management.md) | 详细的服务器管理指南 | 系统管理员 |
| [api-reference.md](api-reference.md) | API接口文档和使用示例 | 开发者 |

## 🚀 快速启动

### 1. 启动完整系统
```bash
./start-loadbalancer.sh
```

### 2. 启动客户端
```bash
./websocket-system -service=client -name=客户端A &
./websocket-system -service=client -name=客户端B &
```

### 3. 访问管理界面
```bash
open http://localhost:8080/web-loadbalancer.html
```

## 🔧 系统组件

### 核心文件
- `main.go` - 主程序入口
- `loadbalancer.go` - 负载均衡器实现
- `server.go` - 后端服务器实现
- `client.go` - 客户端实现
- `protocol.go` - WebSocket消息协议

### 启动脚本
- `start-loadbalancer.sh` - 完整系统启动脚本
- `start-full.sh` - 原有启动脚本(已废弃)

### Web界面
- `web-loadbalancer.html` - 负载均衡器管理界面
- `web-client.html` - 原有客户端界面(已废弃)

## 📊 端口配置

| 服务 | 端口 | 用途 |
|------|------|------|
| 负载均衡器 | 8080 | 客户端连接 + Web管理界面 |
| 后端服务器1 | 8081 | node1服务器 |
| 后端服务器2 | 8082 | node2服务器 |
| 后端服务器3 | 8083 | node3服务器 |

## 🎯 核心功能验证

### ✅ 已实现的需求
1. **多个客户端启动** - 支持任意数量客户端连接
2. **多个服务端启动** - 支持3个后端服务器节点
3. **1个负载服务器** - 负载均衡器统一管理
4. **负载服务器转发** - 轮询策略分配请求
5. **客户端连接到负载服务器** - 统一接入点
6. **Web界面展示客户端列表** - 实时显示连接状态
7. **客户端有独立名字** - 支持自定义客户端名称
8. **查询客户端名字功能** - API和Web界面支持
9. **故障转移机制** - 服务端下线后自动转移

### 🔄 负载均衡特性
- **轮询算法** - 默认的负载分配策略
- **健康检查** - 10秒间隔检测服务器状态
- **故障转移** - 自动绕过不健康的服务器
- **连接管理** - 实时跟踪客户端连接状态

## 🧪 测试验证

### 基础功能测试
```bash
# 1. 启动系统
./start-loadbalancer.sh

# 2. 启动客户端
./websocket-system -service=client -name=测试客户端A &

# 3. 验证连接
curl -s http://localhost:8080/api/clients | python3 -m json.tool

# 4. 测试查询
CLIENT_ID=$(curl -s http://localhost:8080/api/clients | python3 -c "import sys, json; data=json.load(sys.stdin); print(data['clients'][0]['id']) if data['clients'] else print('none')")
curl -s "http://localhost:8080/api/query?client_id=$CLIENT_ID" | python3 -m json.tool
```

### 故障转移测试
```bash
# 1. 关闭node2服务器
lsof -ti:8082 | xargs kill

# 2. 等待健康检查
sleep 12

# 3. 查看状态变化
curl -s http://localhost:8080/api/backends | python3 -m json.tool

# 4. 重启node2
./websocket-system -service=server -mode=single -port=8082 -node=node2 &
```

## 📈 监控和维护

### 实时监控
```bash
# 监控客户端数量
watch -n 2 'curl -s http://localhost:8080/api/clients | jq ".total"'

# 监控服务器健康状态
watch -n 5 'curl -s http://localhost:8080/api/backends | jq ".backends[] | {id: .id, healthy: .is_healthy}"'
```

### 日志查看
```bash
# 查看系统日志
journalctl -f | grep websocket

# 查看端口使用情况
lsof -i:8080,8081,8082,8083
```

## 🆘 故障排除

### 常见问题
1. **端口被占用** - 使用 `lsof -ti:PORT | xargs kill` 清理
2. **客户端连接失败** - 检查负载均衡器是否正常运行
3. **Web界面无法访问** - 确认8080端口可访问
4. **健康检查失败** - 检查后端服务器是否正常启动

### 紧急恢复
```bash
# 停止所有服务
pkill -f websocket-system

# 清理所有端口
for port in 8080 8081 8082 8083; do
    lsof -ti:$port | xargs kill -9 2>/dev/null
done

# 重新启动
./start-loadbalancer.sh
```

## 🔗 相关资源

### 技术栈
- **Go 1.21+** - 主要编程语言
- **gorilla/websocket** - WebSocket库
- **纯HTML/CSS/JavaScript** - Web管理界面

### 扩展功能
- 支持更多负载均衡策略(最少连接、IP哈希)
- Docker容器化部署
- Nginx反向代理集成
- 更多监控指标

## 📞 支持和反馈

### 文档问题
如果发现文档有误或需要补充，请：
1. 检查最新版本的文档
2. 查看相关的日志信息
3. 联系开发团队

### 技术支持
- **系统日志**: 查看程序输出和错误信息
- **API测试**: 使用curl命令验证接口功能
- **Web界面**: 通过管理界面查看实时状态

---

**📝 文档维护**: WebSocket负载均衡系统开发团队  
**🔄 最后更新**: 2025-09-08  
**📧 版本**: v1.0

> 💡 **提示**: 建议先阅读 [quick-reference.md](quick-reference.md) 了解基本操作，然后根据需要查看详细文档。