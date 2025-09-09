package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// 客户端连接信息
type ClientInfo struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	ConnTime   time.Time `json:"conn_time"`
	LastSeen   time.Time `json:"last_seen"`
	IsActive   bool      `json:"is_active"`
	Connection *websocket.Conn `json:"-"` // 不序列化连接对象
}

// Server WebSocket服务器 - 每个节点独立运行
type Server struct {
	port      int
	upgrader  websocket.Upgrader
	clients   map[string]*ClientInfo  // 使用clientID作为key
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
		clients: make(map[string]*ClientInfo),
		nodeID:  nodeID,
	}
}

// Start 启动服务器
func (s *Server) Start() error {
	// WebSocket 接口
	http.HandleFunc("/ws", s.handleWebSocket)
	
	// API 接口
	http.HandleFunc("/health", s.handleHealth)
	http.HandleFunc("/api/clients", s.handleClientList)
	http.HandleFunc("/api/global-clients", s.handleGlobalClientList)
	http.HandleFunc("/api/query", s.handleQuery)
	http.HandleFunc("/api/node-info", s.handleNodeInfo)
	http.HandleFunc("/api/send-command", s.handleSendCommand)
	
	// 静态文件服务 - 提供Web管理界面
	http.Handle("/", http.FileServer(http.Dir("./")))

	log.Printf("WebSocket服务器节点 %s 启动在端口 %d", s.nodeID, s.port)
	log.Printf("Web管理界面: http://localhost:%d/web-node.html", s.port)
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

	// 等待客户端注册消息
	var regMsg map[string]interface{}
	err = conn.ReadJSON(&regMsg)
	if err != nil {
		log.Printf("读取注册消息失败: %v", err)
		return
	}

	clientID, _ := regMsg["client_id"].(string)
	clientName, _ := regMsg["client_name"].(string)
	
	if clientID == "" {
		clientID = "client_" + strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	if clientName == "" {
		clientName = "客户端_" + clientID[len(clientID)-4:]
	}

	// 创建客户端信息
	clientInfo := &ClientInfo{
		ID:         clientID,
		Name:       clientName,
		ConnTime:   time.Now(),
		LastSeen:   time.Now(),
		IsActive:   true,
		Connection: conn,
	}

	// 添加客户端连接
	s.clientsMu.Lock()
	s.clients[clientID] = clientInfo
	s.clientsMu.Unlock()

	// 注册到全局客户端列表
	RegisterGlobalClient(clientID, clientName, s.nodeID, s.port)

	log.Printf("客户端 %s (%s) 连接到节点 %s，当前连接数: %d", 
		clientName, clientID, s.nodeID, len(s.clients))

	// 清理客户端连接
	defer func() {
		s.clientsMu.Lock()
		delete(s.clients, clientID)
		s.clientsMu.Unlock()
		
		// 从全局客户端列表注销
		UnregisterGlobalClient(clientID)
		
		log.Printf("客户端 %s 断开连接，节点 %s 剩余连接数: %d", 
			clientName, s.nodeID, len(s.clients))
	}()

	// 处理消息
	for {
		var rawMsg map[string]interface{}
		err := conn.ReadJSON(&rawMsg)
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket读取错误: %v", err)
			}
			break
		}

		// 检查消息类型
		if msgType, ok := rawMsg["type"].(string); ok {
			switch msgType {
			case "command_response":
				// 处理客户端指令响应
				s.handleCommandResponse(clientID, rawMsg)
				continue
			default:
				// 处理其他类型的消息 (如旧的WebSocketMessage格式)
				if _, hasMethod := rawMsg["method"]; hasMethod {
					// 转换为WebSocketMessage格式处理
					var msg WebSocketMessage
					if msgBytes, err := json.Marshal(rawMsg); err == nil {
						if err := json.Unmarshal(msgBytes, &msg); err == nil {
							log.Printf("节点 %s 收到消息: %s %s", s.nodeID, msg.Method, msg.Path)
							response := s.handleMessage(&msg)
							if err := conn.WriteJSON(response); err != nil {
								log.Printf("发送响应失败: %v", err)
								break
							}
						}
					}
				} else {
					log.Printf("收到未知消息类型: %v", rawMsg)
				}
			}
		} else {
			log.Printf("收到无效消息格式: %v", rawMsg)
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

// handleClientList 处理客户端列表请求
func (s *Server) handleClientList(w http.ResponseWriter, r *http.Request) {
	s.clientsMu.RLock()
	defer s.clientsMu.RUnlock()
	
	w.Header().Set("Content-Type", "application/json")
	
	var clients []ClientInfo
	for _, client := range s.clients {
		// 更新最后访问时间
		client.LastSeen = time.Now()
		clients = append(clients, *client)
	}
	
	response := map[string]interface{}{
		"node_id": s.nodeID,
		"total":   len(clients),
		"clients": clients,
	}
	
	json.NewEncoder(w).Encode(response)
}

// handleQuery 处理查询请求
func (s *Server) handleQuery(w http.ResponseWriter, r *http.Request) {
	clientID := r.URL.Query().Get("client_id")
	if clientID == "" {
		http.Error(w, "缺少client_id参数", http.StatusBadRequest)
		return
	}
	
	s.clientsMu.RLock()
	defer s.clientsMu.RUnlock()
	
	w.Header().Set("Content-Type", "application/json")
	
	if client, exists := s.clients[clientID]; exists {
		client.LastSeen = time.Now()
		response := map[string]interface{}{
			"found":   true,
			"node_id": s.nodeID,
			"client":  *client,
		}
		json.NewEncoder(w).Encode(response)
	} else {
		response := map[string]interface{}{
			"found":   false,
			"node_id": s.nodeID,
			"message": "客户端不存在",
		}
		json.NewEncoder(w).Encode(response)
	}
}

// handleNodeInfo 处理节点信息请求
func (s *Server) handleNodeInfo(w http.ResponseWriter, r *http.Request) {
	s.clientsMu.RLock()
	defer s.clientsMu.RUnlock()
	
	w.Header().Set("Content-Type", "application/json")
	
	response := map[string]interface{}{
		"node_id":     s.nodeID,
		"port":        s.port,
		"clients":     len(s.clients),
		"status":      "running",
		"start_time":  time.Now().Format(time.RFC3339), // 应该记录实际启动时间
		"web_interface": fmt.Sprintf("http://localhost:%d/web-node.html", s.port),
	}
	
	json.NewEncoder(w).Encode(response)
}

// handleGlobalClientList 处理全局客户端列表请求
func (s *Server) handleGlobalClientList(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	
	globalClients := GetAllGlobalClients()
	
	var clients []GlobalClientInfo
	for _, client := range globalClients {
		clients = append(clients, *client)
	}
	
	response := map[string]interface{}{
		"current_node": s.nodeID,
		"total":        len(clients),
		"clients":      clients,
	}
	
	json.NewEncoder(w).Encode(response)
}

// handleSendCommand 处理向客户端发送指令
func (s *Server) handleSendCommand(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "仅支持POST请求", http.StatusMethodNotAllowed)
		return
	}
	
	var req struct {
		ClientID string      `json:"client_id"`
		Command  string      `json:"command"`
		Data     interface{} `json:"data"`
	}
	
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "请求格式错误", http.StatusBadRequest)
		return
	}
	
	if req.ClientID == "" || req.Command == "" {
		http.Error(w, "client_id和command为必填字段", http.StatusBadRequest)
		return
	}
	
	w.Header().Set("Content-Type", "application/json")
	
	// 查找目标客户端
	globalClient, exists := GetGlobalClient(req.ClientID)
	if !exists {
		response := map[string]interface{}{
			"success": false,
			"error":   "客户端不存在",
		}
		json.NewEncoder(w).Encode(response)
		return
	}
	
	// 如果客户端在当前节点，直接发送
	if globalClient.NodeID == s.nodeID {
		success := s.sendCommandToLocalClient(req.ClientID, req.Command, req.Data)
		response := map[string]interface{}{
			"success": success,
			"node":    s.nodeID,
			"message": func() string {
				if success {
					return "指令已发送"
				}
				return "指令发送失败"
			}(),
		}
		json.NewEncoder(w).Encode(response)
		return
	}
	
	// 如果客户端在其他节点，转发请求
	success := s.forwardCommandToOtherNode(globalClient, req.Command, req.Data)
	response := map[string]interface{}{
		"success": success,
		"node":    globalClient.NodeID,
		"message": func() string {
			if success {
				return fmt.Sprintf("指令已转发到节点 %s", globalClient.NodeID)
			}
			return fmt.Sprintf("转发到节点 %s 失败", globalClient.NodeID)
		}(),
	}
	json.NewEncoder(w).Encode(response)
}

// sendCommandToLocalClient 向本地客户端发送指令
func (s *Server) sendCommandToLocalClient(clientID, command string, data interface{}) bool {
	s.clientsMu.RLock()
	client, exists := s.clients[clientID]
	s.clientsMu.RUnlock()
	
	if !exists || client.Connection == nil {
		return false
	}
	
	// 构造指令消息
	cmdMsg := map[string]interface{}{
		"type":    "command",
		"command": command,
		"data":    data,
		"from":    fmt.Sprintf("node-%s", s.nodeID),
	}
	
	// 发送指令
	if err := client.Connection.WriteJSON(cmdMsg); err != nil {
		log.Printf("向客户端 %s 发送指令失败: %v", clientID, err)
		return false
	}
	
	log.Printf("向客户端 %s 发送指令: %s", clientID, command)
	UpdateGlobalClientActivity(clientID)
	return true
}

// forwardCommandToOtherNode 将指令转发到其他节点
func (s *Server) forwardCommandToOtherNode(targetClient *GlobalClientInfo, command string, data interface{}) bool {
	// 构造转发请求
	forwardReq := map[string]interface{}{
		"client_id": targetClient.ID,
		"command":   command,
		"data":      data,
	}
	
	reqBody, err := json.Marshal(forwardReq)
	if err != nil {
		log.Printf("构造转发请求失败: %v", err)
		return false
	}
	
	// 发送HTTP请求到目标节点
	targetURL := fmt.Sprintf("http://localhost:%d/api/send-command", targetClient.NodePort)
	resp, err := http.Post(targetURL, "application/json", bytes.NewReader(reqBody))
	if err != nil {
		log.Printf("转发指令到节点 %s:%d 失败: %v", targetClient.NodeID, targetClient.NodePort, err)
		return false
	}
	defer resp.Body.Close()
	
	return resp.StatusCode == 200
}

// handleCommandResponse 处理客户端指令响应
func (s *Server) handleCommandResponse(clientID string, response map[string]interface{}) {
	result, _ := response["result"].(string)
	message, _ := response["message"].(string)
	data := response["data"]
	timestamp, _ := response["timestamp"].(float64)

	log.Printf("📨 收到客户端 %s 的指令响应: %s - %s", clientID, result, message)

	// 更新客户端活跃状态
	UpdateGlobalClientActivity(clientID)

	// 可以在这里添加响应的持久化存储、通知机制等
	// 例如：存储到数据库、发送到监控系统、通知Web界面等

	if result == "success" {
		log.Printf("✅ 客户端 %s 成功执行指令: %s", clientID, message)
	} else {
		log.Printf("❌ 客户端 %s 执行指令失败: %s", clientID, message)
	}

	// 如果有响应数据，也可以记录下来
	if data != nil {
		if dataJSON, err := json.MarshalIndent(data, "", "  "); err == nil {
			log.Printf("📄 响应数据: %s", string(dataJSON))
		}
	}

	log.Printf("⏰ 响应时间: %v", time.Unix(int64(timestamp), 0).Format("2006-01-02 15:04:05"))
}
