# 📡 WebSocket负载均衡系统 - API接口文档

## 🌐 基础信息

**负载均衡器地址**: http://localhost:8080  
**WebSocket连接地址**: ws://localhost:8080/ws  
**Web管理界面**: http://localhost:8080/web-loadbalancer.html

## 📋 API接口列表

### 1. 健康检查
**GET** `/health`

获取负载均衡器的健康状态

#### 请求示例
```bash
curl -s http://localhost:8080/health
```

#### 响应示例
```json
{
    "status": "healthy",
    "service": "loadbalancer", 
    "clients": 2,
    "backends": 3,
    "time": "2025-09-08T15:55:25Z"
}
```

### 2. 客户端列表
**GET** `/api/clients`

获取所有连接的客户端列表

#### 请求示例
```bash
curl -s http://localhost:8080/api/clients | python3 -m json.tool
```

#### 响应示例
```json
{
    "clients": [
        {
            "id": "client_dcn9aa2ahze0",
            "name": "客户端A",
            "backend_id": "node1",
            "conn_time": "15:55:25",
            "last_seen": "15:55:25", 
            "is_active": true
        },
        {
            "id": "client_dcn9agtd05pk",
            "name": "客户端B",
            "backend_id": "node2",
            "conn_time": "15:55:40",
            "last_seen": "15:55:40",
            "is_active": true
        }
    ],
    "total": 2
}
```

#### 字段说明
- `id`: 客户端唯一标识符
- `name`: 客户端显示名称
- `backend_id`: 连接的后端服务器ID
- `conn_time`: 连接时间
- `last_seen`: 最后活跃时间
- `is_active`: 是否活跃状态
- `total`: 客户端总数

### 3. 后端服务器状态
**GET** `/api/backends`

获取所有后端服务器的状态信息

#### 请求示例
```bash
curl -s http://localhost:8080/api/backends | python3 -m json.tool
```

#### 响应示例
```json
{
    "backends": [
        {
            "id": "node1",
            "address": "ws://localhost:8081/ws",
            "connections": 1,
            "is_healthy": true,
            "last_check": "15:59:54"
        },
        {
            "id": "node2", 
            "address": "ws://localhost:8082/ws",
            "connections": 1,
            "is_healthy": false,
            "last_check": "15:59:54"
        },
        {
            "id": "node3",
            "address": "ws://localhost:8083/ws", 
            "connections": 0,
            "is_healthy": true,
            "last_check": "15:59:54"
        }
    ],
    "strategy": "round_robin"
}
```

#### 字段说明
- `id`: 后端服务器ID
- `address`: WebSocket连接地址
- `connections`: 当前连接数
- `is_healthy`: 健康状态
- `last_check`: 最后健康检查时间
- `strategy`: 负载均衡策略

### 4. 查询客户端信息
**GET** `/api/query`

查询指定客户端的详细信息

#### 请求参数
- `client_id` (必需): 客户端ID

#### 请求示例
```bash
curl -s "http://localhost:8080/api/query?client_id=client_dcn9aa2ahze0" | python3 -m json.tool
```

#### 响应示例
```json
{
    "client_id": "client_dcn9aa2ahze0",
    "client_name": "客户端A", 
    "backend_id": "node1",
    "status": "ok"
}
```

#### 错误响应
```json
{
    "error": "client not found"
}
```

## 🔌 WebSocket接口

### 连接地址
```
ws://localhost:8080/ws
```

### 消息协议

#### 客户端注册
客户端连接后需要发送注册消息：
```json
{
    "client_id": "client_1234567890_abc123",
    "client_name": "我的客户端",
    "timestamp": 1703123456789
}
```

#### 查询请求 
负载均衡器发送给客户端的查询消息：
```json
{
    "type": "query_name",
    "id": "query_1234567890"
}
```

#### 查询响应
客户端回复给负载均衡器的响应：
```json
{
    "type": "name_response", 
    "client_id": "client_1234567890_abc123",
    "client_name": "我的客户端",
    "timestamp": 1703123456789
}
```

#### 心跳检测
```json
// 负载均衡器发送
{
    "type": "ping",
    "timestamp": 1703123456789  
}

// 客户端响应
{
    "type": "pong",
    "timestamp": 1703123456789
}
```

## 🌐 Web管理界面功能

### 界面访问
```
http://localhost:8080/web-loadbalancer.html
```

### 主要功能
1. **实时客户端列表** - 显示所有连接的客户端
2. **后端服务器状态** - 显示服务器健康状态和连接数
3. **查询客户端名字** - 点击按钮查询特定客户端
4. **实时日志** - 显示操作日志和系统事件
5. **状态统计** - 连接数、策略等统计信息

### 界面功能演示
```bash
# 打开界面
open http://localhost:8080/web-loadbalancer.html

# 或者使用curl验证界面可访问
curl -I http://localhost:8080/web-loadbalancer.html
```

## 🧪 API测试用例

### 完整API测试流程
```bash
#!/bin/bash
echo "=== WebSocket负载均衡系统 API测试 ==="

# 1. 测试健康检查
echo -e "\n1. 健康检查测试:"
curl -s http://localhost:8080/health | python3 -m json.tool

# 2. 测试客户端列表
echo -e "\n2. 客户端列表测试:"
CLIENTS_RESP=$(curl -s http://localhost:8080/api/clients)
echo $CLIENTS_RESP | python3 -m json.tool

# 3. 提取第一个客户端ID进行查询测试
CLIENT_ID=$(echo $CLIENTS_RESP | python3 -c "import sys, json; data=json.load(sys.stdin); print(data['clients'][0]['id']) if data['clients'] else print('none')")

if [ "$CLIENT_ID" != "none" ]; then
    echo -e "\n3. 客户端查询测试 (ID: $CLIENT_ID):"
    curl -s "http://localhost:8080/api/query?client_id=$CLIENT_ID" | python3 -m json.tool
else
    echo -e "\n3. 客户端查询测试: 无客户端连接，跳过测试"
fi

# 4. 测试后端服务器状态
echo -e "\n4. 后端服务器状态测试:"
curl -s http://localhost:8080/api/backends | python3 -m json.tool

echo -e "\n=== API测试完成 ==="
```

### 性能测试
```bash
# 并发客户端连接测试
for i in {1..5}; do
    ./websocket-system -service=client -name="压力测试客户端$i" &
done

# 等待连接建立
sleep 3

# 查看客户端数量
curl -s http://localhost:8080/api/clients | jq '.total'

# 查看负载分布
curl -s http://localhost:8080/api/backends | jq '.backends[] | {id: .id, connections: .connections}'
```

## 📊 错误代码

| HTTP状态码 | 错误类型 | 描述 |
|------------|----------|------|
| 200 | 成功 | 请求成功处理 |
| 400 | 请求错误 | 缺少必需参数或参数格式错误 |
| 404 | 资源不存在 | 客户端ID不存在或资源未找到 |
| 500 | 服务器错误 | 内部服务器错误 |

## 🔧 开发者工具

### curl便捷脚本
创建 `api-test.sh` 脚本：
```bash
#!/bin/bash
API_BASE="http://localhost:8080"

case $1 in
    health)
        curl -s $API_BASE/health | python3 -m json.tool
        ;;
    clients)
        curl -s $API_BASE/api/clients | python3 -m json.tool
        ;;
    backends)  
        curl -s $API_BASE/api/backends | python3 -m json.tool
        ;;
    query)
        if [ -z "$2" ]; then
            echo "用法: $0 query <client_id>"
            exit 1
        fi
        curl -s "$API_BASE/api/query?client_id=$2" | python3 -m json.tool
        ;;
    *)
        echo "用法: $0 {health|clients|backends|query <client_id>}"
        ;;
esac
```

使用方式：
```bash
chmod +x api-test.sh
./api-test.sh health
./api-test.sh clients  
./api-test.sh backends
./api-test.sh query client_dcn9aa2ahze0
```

---

**📝 最后更新**: 2025-09-08  
**🔧 API版本**: v1.0  
**📧 技术支持**: 查看 `docs/server-management.md` 获取更多帮助