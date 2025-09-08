package main

import (
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// 负载均衡策略
type LoadBalanceStrategy string

const (
	RoundRobin    LoadBalanceStrategy = "round_robin"
	LeastConn     LoadBalanceStrategy = "least_conn"
	IPHash        LoadBalanceStrategy = "ip_hash"
)

// 后端服务器信息
type BackendServer struct {
	ID           string
	Address      string  // ws://localhost:8081/ws
	Connections  int     // 当前连接数
	IsHealthy    bool    // 健康状态
	LastCheck    time.Time
	Weight       int     // 权重
}

// 客户端连接信息
type ClientConnection struct {
	ID         string
	Name       string
	BackendID  string    // 连接到哪个后端服务器
	ConnTime   time.Time
	LastSeen   time.Time
	ClientConn *websocket.Conn // 客户端连接
	ServerConn *websocket.Conn // 到后端服务器的连接
	IsActive   bool
}

// 负载均衡器
type LoadBalancer struct {
	port         int
	strategy     LoadBalanceStrategy
	backends     map[string]*BackendServer
	backendsMu   sync.RWMutex
	clients      map[string]*ClientConnection
	clientsMu    sync.RWMutex
	upgrader     websocket.Upgrader
	roundRobinIdx int
}

// 创建负载均衡器
func NewLoadBalancer(port int, strategy LoadBalanceStrategy) *LoadBalancer {
	return &LoadBalancer{
		port:     port,
		strategy: strategy,
		backends: make(map[string]*BackendServer),
		clients:  make(map[string]*ClientConnection),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
	}
}

// 添加后端服务器
func (lb *LoadBalancer) AddBackend(id, address string) {
	lb.backendsMu.Lock()
	defer lb.backendsMu.Unlock()
	
	lb.backends[id] = &BackendServer{
		ID:        id,
		Address:   address,
		IsHealthy: true,
		LastCheck: time.Now(),
		Weight:    1,
	}
	
	log.Printf("添加后端服务器: %s -> %s", id, address)
}

// 启动负载均衡器
func (lb *LoadBalancer) Start() error {
	// WebSocket连接处理（客户端连接）
	http.HandleFunc("/ws", lb.handleWebSocket)
	
	// Web API接口
	http.HandleFunc("/api/clients", lb.handleClientList)
	http.HandleFunc("/api/query", lb.handleQuery)
	http.HandleFunc("/api/backends", lb.handleBackends)
	http.HandleFunc("/health", lb.handleHealth)
	
	// 静态文件服务
	http.Handle("/", http.FileServer(http.Dir("./")))
	
	// 启动健康检查
	go lb.healthChecker()
	
	log.Printf("负载均衡器启动在端口 %d", lb.port)
	log.Printf("负载均衡策略: %s", lb.strategy)
	log.Printf("Web界面: http://localhost:%d/web-client.html", lb.port)
	
	return http.ListenAndServe(":"+strconv.Itoa(lb.port), nil)
}

// 处理客户端WebSocket连接
func (lb *LoadBalancer) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	clientConn, err := lb.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket升级失败: %v", err)
		return
	}
	defer clientConn.Close()

	// 等待客户端注册消息
	var regMsg map[string]interface{}
	err = clientConn.ReadJSON(&regMsg)
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

	log.Printf("客户端注册: %s (%s)", clientName, clientID)

	// 选择后端服务器
	backend := lb.selectBackend(r.RemoteAddr)
	if backend == nil {
		log.Printf("没有可用的后端服务器")
		clientConn.WriteJSON(map[string]interface{}{
			"error": "没有可用的服务器",
		})
		return
	}

	// 连接到后端服务器
	serverConn, err := lb.connectToBackend(backend)
	if err != nil {
		log.Printf("连接后端服务器失败: %v", err)
		clientConn.WriteJSON(map[string]interface{}{
			"error": "服务器连接失败",
		})
		return
	}
	defer serverConn.Close()

	// 创建客户端连接记录
	client := &ClientConnection{
		ID:         clientID,
		Name:       clientName,
		BackendID:  backend.ID,
		ConnTime:   time.Now(),
		LastSeen:   time.Now(),
		ClientConn: clientConn,
		ServerConn: serverConn,
		IsActive:   true,
	}

	lb.clientsMu.Lock()
	lb.clients[clientID] = client
	backend.Connections++
	lb.clientsMu.Unlock()

	log.Printf("客户端 %s 连接到后端服务器 %s", clientName, backend.ID)

	// 向后端服务器转发注册消息
	serverConn.WriteJSON(regMsg)

	// 清理函数
	defer func() {
		lb.clientsMu.Lock()
		delete(lb.clients, clientID)
		if backend.Connections > 0 {
			backend.Connections--
		}
		lb.clientsMu.Unlock()
		log.Printf("客户端 %s 断开连接", clientName)
	}()

	// 启动消息转发
	lb.forwardMessages(client)
}

// 选择后端服务器
func (lb *LoadBalancer) selectBackend(_ string) *BackendServer {
	lb.backendsMu.RLock()
	defer lb.backendsMu.RUnlock()

	// 过滤健康的服务器
	healthyBackends := make([]*BackendServer, 0)
	for _, backend := range lb.backends {
		if backend.IsHealthy {
			healthyBackends = append(healthyBackends, backend)
		}
	}

	if len(healthyBackends) == 0 {
		return nil
	}

	switch lb.strategy {
	case RoundRobin:
		backend := healthyBackends[lb.roundRobinIdx%len(healthyBackends)]
		lb.roundRobinIdx++
		return backend
		
	case LeastConn:
		var selected *BackendServer
		minConn := int(^uint(0) >> 1) // max int
		for _, backend := range healthyBackends {
			if backend.Connections < minConn {
				minConn = backend.Connections
				selected = backend
			}
		}
		return selected
		
	default:
		return healthyBackends[0]
	}
}

// 连接到后端服务器
func (lb *LoadBalancer) connectToBackend(backend *BackendServer) (*websocket.Conn, error) {
	u, err := url.Parse(backend.Address)
	if err != nil {
		return nil, err
	}

	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		// 标记服务器为不健康
		backend.IsHealthy = false
		return nil, err
	}

	return conn, nil
}

// 消息转发
func (lb *LoadBalancer) forwardMessages(client *ClientConnection) {
	defer func() {
		// 清理连接
		lb.cleanupConnection(client)
	}()

	// 使用通道来协调两个goroutine的退出
	done := make(chan struct{})
	clientError := make(chan error, 1)
	serverError := make(chan error, 1)

	// 客户端 -> 服务端
	go func() {
		defer close(clientError)
		for {
			select {
			case <-done:
				return
			default:
			}

			var msg map[string]interface{}
			err := client.ClientConn.ReadJSON(&msg)
			if err != nil {
				log.Printf("读取客户端消息失败: %v", err)
				clientError <- err
				return
			}
			
			client.LastSeen = time.Now()
			
			// 转发到后端服务器
			err = client.ServerConn.WriteJSON(msg)
			if err != nil {
				log.Printf("转发到服务端失败: %v", err)
				clientError <- err
				return
			}
		}
	}()

	// 服务端 -> 客户端
	go func() {
		defer close(serverError)
		for {
			select {
			case <-done:
				return
			default:
			}

			var msg map[string]interface{}
			err := client.ServerConn.ReadJSON(&msg)
			if err != nil {
				log.Printf("读取服务端消息失败: %v", err)
				serverError <- err
				return
			}
			
			// 转发到客户端
			err = client.ClientConn.WriteJSON(msg)
			if err != nil {
				log.Printf("转发到客户端失败: %v", err)
				serverError <- err
				return
			}
		}
	}()

	// 等待任一方向的连接断开
	select {
	case err := <-clientError:
		if err != nil {
			log.Printf("客户端连接错误: %v", err)
		}
	case err := <-serverError:
		if err != nil {
			log.Printf("服务端连接错误: %v", err)
		}
	}

	// 通知所有goroutine退出
	close(done)
	log.Printf("客户端 %s 的消息转发已停止", client.ID)
}

// 清理连接
func (lb *LoadBalancer) cleanupConnection(client *ClientConnection) {
	log.Printf("清理客户端连接: %s", client.ID)

	// 关闭WebSocket连接
	if client.ClientConn != nil {
		client.ClientConn.Close()
	}
	if client.ServerConn != nil {
		client.ServerConn.Close()
	}

	// 从客户端映射中移除
	lb.clientsMu.Lock()
	delete(lb.clients, client.ID)
	lb.clientsMu.Unlock()

	// 减少后端服务器的连接计数
	if client.BackendID != "" {
		lb.backendsMu.Lock()
		for _, backend := range lb.backends {
			if backend.ID == client.BackendID {
				if backend.Connections > 0 {
					backend.Connections--
				}
				log.Printf("后端服务器 %s 连接数减少: %d", backend.ID, backend.Connections)
				break
			}
		}
		lb.backendsMu.Unlock()
	}

	log.Printf("客户端 %s 已从系统中清理", client.ID)
}

// 处理客户端列表查询
func (lb *LoadBalancer) handleClientList(w http.ResponseWriter, r *http.Request) {
	lb.clientsMu.RLock()
	defer lb.clientsMu.RUnlock()

	clients := make([]map[string]interface{}, 0)
	for _, client := range lb.clients {
		clients = append(clients, map[string]interface{}{
			"id":         client.ID,
			"name":       client.Name,
			"backend_id": client.BackendID,
			"conn_time":  client.ConnTime.Format("15:04:05"),
			"last_seen":  client.LastSeen.Format("15:04:05"),
			"is_active":  client.IsActive,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"clients": clients,
		"total":   len(clients),
	})
}

// 处理查询客户端名字
func (lb *LoadBalancer) handleQuery(w http.ResponseWriter, r *http.Request) {
	clientID := r.URL.Query().Get("client_id")
	if clientID == "" {
		http.Error(w, "missing client_id", http.StatusBadRequest)
		return
	}

	lb.clientsMu.RLock()
	client, exists := lb.clients[clientID]
	lb.clientsMu.RUnlock()

	if !exists {
		http.Error(w, "client not found", http.StatusNotFound)
		return
	}

	// 通过负载均衡器查询客户端名字
	queryMsg := map[string]interface{}{
		"type": "query_name",
		"id":   "query_" + strconv.FormatInt(time.Now().UnixNano(), 36),
	}

	// 发送查询到客户端
	err := client.ClientConn.WriteJSON(queryMsg)
	if err != nil {
		http.Error(w, "failed to query client", http.StatusInternalServerError)
		return
	}

	// 等待响应（简化处理）
	time.Sleep(100 * time.Millisecond)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"client_id":   client.ID,
		"client_name": client.Name,
		"backend_id":  client.BackendID,
		"status":      "ok",
	})
}

// 处理后端服务器状态
func (lb *LoadBalancer) handleBackends(w http.ResponseWriter, r *http.Request) {
	lb.backendsMu.RLock()
	defer lb.backendsMu.RUnlock()

	backends := make([]map[string]interface{}, 0)
	for _, backend := range lb.backends {
		backends = append(backends, map[string]interface{}{
			"id":          backend.ID,
			"address":     backend.Address,
			"connections": backend.Connections,
			"is_healthy":  backend.IsHealthy,
			"last_check":  backend.LastCheck.Format("15:04:05"),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"backends": backends,
		"strategy": lb.strategy,
	})
}

// 健康检查
func (lb *LoadBalancer) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":   "healthy",
		"service":  "loadbalancer",
		"clients":  len(lb.clients),
		"backends": len(lb.backends),
		"time":     time.Now().Format(time.RFC3339),
	})
}

// 后端健康检查器
func (lb *LoadBalancer) healthChecker() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		lb.checkBackendsHealth()
	}
}

// 检查后端服务器健康状态
func (lb *LoadBalancer) checkBackendsHealth() {
	lb.backendsMu.Lock()
	defer lb.backendsMu.Unlock()

	for _, backend := range lb.backends {
		// 简单的健康检查：尝试HTTP请求
		healthURL := backend.Address
		if healthURL[0:2] == "ws" {
			healthURL = "http" + healthURL[2:] // ws://... -> http://...
			if healthURL[len(healthURL)-3:] == "/ws" {
				healthURL = healthURL[:len(healthURL)-3] + "/health"
			}
		}

		// 创建带超时的HTTP客户端
		client := &http.Client{
			Timeout: 5 * time.Second,
		}
		
		resp, err := client.Get(healthURL)
		if err != nil {
			if backend.IsHealthy {
				log.Printf("后端服务器 %s 变为不健康: %v", backend.ID, err)
			}
			backend.IsHealthy = false
		} else {
			resp.Body.Close()
			if !backend.IsHealthy {
				log.Printf("后端服务器 %s 恢复健康", backend.ID)
			}
			backend.IsHealthy = true
		}
		
		backend.LastCheck = time.Now()
	}
}