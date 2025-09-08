package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Server WebSocket服务器
type Server struct {
	port      int
	upgrader  websocket.Upgrader
	clients   map[*websocket.Conn]bool
	clientsMu sync.RWMutex
	nodeID    string
}

// NewServer 创建新服务器
func NewServer(port int, nodeID string) *Server {
	return &Server{
		port: port,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // 开发环境允许所有origin
			},
		},
		clients: make(map[*websocket.Conn]bool),
		nodeID:  nodeID,
	}
}

// Start 启动服务器
func (s *Server) Start() error {
	http.HandleFunc("/ws", s.handleWebSocket)
	http.HandleFunc("/health", s.handleHealth)

	log.Printf("WebSocket服务器节点 %s 启动在端口 %d", s.nodeID, s.port)
	return http.ListenAndServe(":"+strconv.Itoa(s.port), nil)
}

// handleWebSocket 处理WebSocket连接
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket升级失败: %v", err)
		return
	}
	defer conn.Close()

	// 添加客户端连接
	s.clientsMu.Lock()
	s.clients[conn] = true
	s.clientsMu.Unlock()

	log.Printf("客户端连接到节点 %s，当前连接数: %d", s.nodeID, len(s.clients))

	// 清理客户端连接
	defer func() {
		s.clientsMu.Lock()
		delete(s.clients, conn)
		s.clientsMu.Unlock()
		log.Printf("客户端断开连接，节点 %s 剩余连接数: %d", s.nodeID, len(s.clients))
	}()

	// 处理消息
	for {
		var msg WebSocketMessage
		err := conn.ReadJSON(&msg)
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket读取错误: %v", err)
			}
			break
		}

		log.Printf("节点 %s 收到消息: %s %s", s.nodeID, msg.Method, msg.Path)

		// 处理请求
		response := s.handleMessage(&msg)

		// 发送响应
		if err := conn.WriteJSON(response); err != nil {
			log.Printf("发送响应失败: %v", err)
			break
		}
	}
}

// handleMessage 处理WebSocket消息
func (s *Server) handleMessage(msg *WebSocketMessage) *WebSocketResponse {
	switch msg.Method {
	case "GET":
		return s.handleGet(msg)
	case "POST":
		return s.handlePost(msg)
	case "PUT":
		return s.handlePut(msg)
	case "DELETE":
		return s.handleDelete(msg)
	default:
		return NewResponse(msg.ID, 405, nil)
	}
}

// handleGet 处理GET请求
func (s *Server) handleGet(msg *WebSocketMessage) *WebSocketResponse {
	path := strings.TrimPrefix(msg.Path, "/")

	switch path {
	case "info":
		return NewResponse(msg.ID, 200, map[string]interface{}{
			"node_id":   s.nodeID,
			"port":      s.port,
			"clients":   len(s.clients),
			"timestamp": time.Now().Unix(),
		})
	case "health":
		return NewResponse(msg.ID, 200, map[string]string{
			"status": "ok",
			"node":   s.nodeID,
		})
	default:
		return NewResponse(msg.ID, 404, map[string]string{
			"error": "路径不存在",
		})
	}
}

// handlePost 处理POST请求
func (s *Server) handlePost(msg *WebSocketMessage) *WebSocketResponse {
	return NewResponse(msg.ID, 201, map[string]interface{}{
		"message": "创建成功",
		"node":    s.nodeID,
		"data":    msg.Body,
	})
}

// handlePut 处理PUT请求
func (s *Server) handlePut(msg *WebSocketMessage) *WebSocketResponse {
	return NewResponse(msg.ID, 200, map[string]interface{}{
		"message": "更新成功",
		"node":    s.nodeID,
		"data":    msg.Body,
	})
}

// handleDelete 处理DELETE请求
func (s *Server) handleDelete(msg *WebSocketMessage) *WebSocketResponse {
	return NewResponse(msg.ID, 200, map[string]interface{}{
		"message": "删除成功",
		"node":    s.nodeID,
	})
}

// handleHealth 健康检查
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	response := map[string]interface{}{
		"status":  "healthy",
		"node_id": s.nodeID,
		"port":    s.port,
		"clients": len(s.clients),
		"time":    time.Now().Format(time.RFC3339),
	}
	json.NewEncoder(w).Encode(response)
}

// GetClientCount 获取客户端连接数
func (s *Server) GetClientCount() int {
	s.clientsMu.RLock()
	defer s.clientsMu.RUnlock()
	return len(s.clients)
}
