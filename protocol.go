package main

import (
	"time"
)

// WebSocketMessage 定义WebSocket消息格式，类似RESTful
type WebSocketMessage struct {
	ID        string            `json:"id"`                // 消息ID，用于请求响应匹配
	Method    string            `json:"method"`            // GET, POST, PUT, DELETE
	Path      string            `json:"path"`              // 类似RESTful的路径，如 /users, /users/123
	Headers   map[string]string `json:"headers,omitempty"` // 请求头
	Body      interface{}       `json:"body,omitempty"`    // 请求体
	Timestamp int64             `json:"timestamp"`         // 时间戳
}

// WebSocketResponse WebSocket响应格式
type WebSocketResponse struct {
	ID        string            `json:"id"`     // 对应请求的ID
	Status    int               `json:"status"` // HTTP风格的状态码
	Headers   map[string]string `json:"headers,omitempty"`
	Body      interface{}       `json:"body,omitempty"`
	Error     string            `json:"error,omitempty"`
	Timestamp int64             `json:"timestamp"`
}

// NewMessage 创建新消息
func NewMessage(method, path string, body interface{}) *WebSocketMessage {
	return &WebSocketMessage{
		ID:        generateID(),
		Method:    method,
		Path:      path,
		Body:      body,
		Timestamp: time.Now().UnixMilli(),
	}
}

// NewResponse 创建响应
func NewResponse(requestID string, status int, body interface{}) *WebSocketResponse {
	return &WebSocketResponse{
		ID:        requestID,
		Status:    status,
		Body:      body,
		Timestamp: time.Now().UnixMilli(),
	}
}

// 简单的ID生成器
func generateID() string {
	return time.Now().Format("20060102150405") + "-" + string(rune(time.Now().Nanosecond()%1000))
}

// 协议示例说明：
// 客户端发送：
// {
//   "id": "123456",
//   "method": "GET",
//   "path": "/users/1",
//   "timestamp": 1703123456789
// }
//
// 服务器响应：
// {
//   "id": "123456",
//   "status": 200,
//   "body": {"id": 1, "name": "张三"},
//   "timestamp": 1703123456790
// }
