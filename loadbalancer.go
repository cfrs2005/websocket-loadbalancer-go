package main

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
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
	ID          string
	HTTPAddress string    // http://localhost:8081 (HTTP服务地址)
	WSAddress   string    // ws://localhost:8081/ws (WebSocket地址)
	Connections int       // 当前连接数
	IsHealthy   bool      // 健康状态
	LastCheck   time.Time
	Weight      int       // 权重
	Proxy       *httputil.ReverseProxy // HTTP代理
}

// 会话信息 - 用于会话保持
type Session struct {
	SessionID  string    // 会话ID
	BackendID  string    // 绑定的后端服务器ID
	ClientIP   string    // 客户端IP
	CreateTime time.Time // 创建时间
	LastSeen   time.Time // 最后访问时间
}

// 纯七层负载均衡器 - 仅做转发和健康检查
type LoadBalancer struct {
	port         int
	strategy     LoadBalanceStrategy
	backends     map[string]*BackendServer  // 后端服务器
	backendsMu   sync.RWMutex
	sessions     map[string]*Session        // 会话保持
	sessionsMu   sync.RWMutex
	upgrader     websocket.Upgrader
	roundRobinIdx int
}

// 创建负载均衡器
func NewLoadBalancer(port int, strategy LoadBalanceStrategy) *LoadBalancer {
	lb := &LoadBalancer{
		port:     port,
		strategy: strategy,
		backends: make(map[string]*BackendServer),
		sessions: make(map[string]*Session),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
	}
	
	// 启动健康检查
	go lb.healthCheck()
	
	return lb
}

// 添加后端服务器
func (lb *LoadBalancer) AddBackend(id string, httpPort int) {
	lb.backendsMu.Lock()
	defer lb.backendsMu.Unlock()
	
	httpAddr := fmt.Sprintf("http://localhost:%d", httpPort)
	wsAddr := fmt.Sprintf("ws://localhost:%d/ws", httpPort)
	
	// 创建 HTTP 反向代理
	targetURL, _ := url.Parse(httpAddr)
	proxy := httputil.NewSingleHostReverseProxy(targetURL)
	
	lb.backends[id] = &BackendServer{
		ID:          id,
		HTTPAddress: httpAddr,
		WSAddress:   wsAddr,
		IsHealthy:   true,
		LastCheck:   time.Now(),
		Weight:      1,
		Proxy:       proxy,
	}
	
	log.Printf("添加后端服务器: %s -> HTTP:%s WS:%s", id, httpAddr, wsAddr)
}

// 获取客户端唯一标识（用于会话保持）
func (lb *LoadBalancer) getClientIdentifier(r *http.Request) string {
	// 优先使用 Session Cookie
	if cookie, err := r.Cookie("lb_session"); err == nil {
		return cookie.Value
	}
	
	// 如果没有 Cookie，使用 IP + User-Agent 生成哈希
	clientInfo := r.RemoteAddr + r.UserAgent()
	hash := md5.Sum([]byte(clientInfo))
	return fmt.Sprintf("%x", hash)
}

// 选择后端服务器（支持会话保持）
func (lb *LoadBalancer) selectBackend(clientID string) *BackendServer {
	lb.backendsMu.RLock()
	defer lb.backendsMu.RUnlock()
	
	// 检查是否有现有会话
	lb.sessionsMu.RLock()
	if session, exists := lb.sessions[clientID]; exists {
		if backend, exists := lb.backends[session.BackendID]; exists && backend.IsHealthy {
			// 更新最后访问时间
			session.LastSeen = time.Now()
			lb.sessionsMu.RUnlock()
			return backend
		}
	}
	lb.sessionsMu.RUnlock()
	
	// 没有会话或原后端不健康，选择新的后端
	var healthyBackends []*BackendServer
	for _, backend := range lb.backends {
		if backend.IsHealthy {
			healthyBackends = append(healthyBackends, backend)
		}
	}
	
	if len(healthyBackends) == 0 {
		return nil
	}
	
	var selectedBackend *BackendServer
	switch lb.strategy {
	case RoundRobin:
		selectedBackend = healthyBackends[lb.roundRobinIdx%len(healthyBackends)]
		lb.roundRobinIdx++
	case LeastConn:
		selectedBackend = healthyBackends[0]
		for _, backend := range healthyBackends[1:] {
			if backend.Connections < selectedBackend.Connections {
				selectedBackend = backend
			}
		}
	default: // IPHash 或其他
		hash := md5.Sum([]byte(clientID))
		idx := int(hash[0]) % len(healthyBackends)
		selectedBackend = healthyBackends[idx]
	}
	
	// 创建或更新会话
	lb.sessionsMu.Lock()
	lb.sessions[clientID] = &Session{
		SessionID:  clientID,
		BackendID:  selectedBackend.ID,
		CreateTime: time.Now(),
		LastSeen:   time.Now(),
	}
	lb.sessionsMu.Unlock()
	
	return selectedBackend
}

// 健康检查
func (lb *LoadBalancer) healthCheck() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	
	for range ticker.C {
		lb.backendsMu.Lock()
		for id, backend := range lb.backends {
			// 检查HTTP健康状态
			resp, err := http.Get(backend.HTTPAddress + "/health")
			if err != nil || resp.StatusCode != 200 {
				if backend.IsHealthy {
					log.Printf("后端服务器 %s (%s) 变为不健康", id, backend.HTTPAddress)
				}
				backend.IsHealthy = false
			} else {
				if !backend.IsHealthy {
					log.Printf("后端服务器 %s (%s) 恢复健康", id, backend.HTTPAddress)
				}
				backend.IsHealthy = true
				resp.Body.Close()
			}
			backend.LastCheck = time.Now()
		}
		lb.backendsMu.Unlock()
	}
}

// 处理所有请求的核心函数
func (lb *LoadBalancer) handleRequest(w http.ResponseWriter, r *http.Request) {
	// 获取客户端标识
	clientID := lb.getClientIdentifier(r)
	
	// 选择后端服务器
	backend := lb.selectBackend(clientID)
	if backend == nil {
		http.Error(w, "没有可用的后端服务器", http.StatusServiceUnavailable)
		return
	}
	
	// 设置会话 Cookie
	cookie := &http.Cookie{
		Name:     "lb_session",
		Value:    clientID,
		Path:     "/",
		MaxAge:   3600 * 24, // 24小时
		HttpOnly: false,     // 允许JS访问，方便WebSocket使用
	}
	http.SetCookie(w, cookie)
	
	// 检查是否是 WebSocket 升级请求
	if websocket.IsWebSocketUpgrade(r) {
		lb.handleWebSocketProxy(w, r, backend)
		return
	}
	
	// HTTP 请求直接代理到后端
	backend.Proxy.ServeHTTP(w, r)
}

// WebSocket 代理处理
func (lb *LoadBalancer) handleWebSocketProxy(w http.ResponseWriter, r *http.Request, backend *BackendServer) {
	// 升级客户端连接
	clientConn, err := lb.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket升级失败: %v", err)
		return
	}
	defer clientConn.Close()

	// 连接到后端 WebSocket 服务器
	backendURL := backend.WSAddress
	if r.URL.RawQuery != "" {
		backendURL += "?" + r.URL.RawQuery
	}

	backendConn, _, err := websocket.DefaultDialer.Dial(backendURL, nil)
	if err != nil {
		log.Printf("连接后端WebSocket失败: %v", err)
		clientConn.WriteMessage(websocket.CloseMessage, 
			websocket.FormatCloseMessage(websocket.CloseInternalServerErr, "后端服务器连接失败"))
		return
	}
	defer backendConn.Close()

	log.Printf("WebSocket连接已建立: 客户端 -> %s", backend.ID)

	// 增加连接计数
	lb.backendsMu.Lock()
	backend.Connections++
	lb.backendsMu.Unlock()

	// 清理函数
	defer func() {
		lb.backendsMu.Lock()
		if backend.Connections > 0 {
			backend.Connections--
		}
		lb.backendsMu.Unlock()
		log.Printf("WebSocket连接已关闭: 客户端 -> %s", backend.ID)
	}()

	// 双向消息转发
	errChan := make(chan error, 2)
	
	// 客户端 -> 后端
	go func() {
		for {
			messageType, message, err := clientConn.ReadMessage()
			if err != nil {
				errChan <- err
				return
			}
			if err := backendConn.WriteMessage(messageType, message); err != nil {
				errChan <- err
				return
			}
		}
	}()
	
	// 后端 -> 客户端
	go func() {
		for {
			messageType, message, err := backendConn.ReadMessage()
			if err != nil {
				errChan <- err
				return
			}
			if err := clientConn.WriteMessage(messageType, message); err != nil {
				errChan <- err
				return
			}
		}
	}()

	// 等待任一方向发生错误
	<-errChan
}

// 启动负载均衡器
func (lb *LoadBalancer) Start() error {
	// API 路由
	http.HandleFunc("/api/global-clients", lb.handleGlobalClients)
	http.HandleFunc("/api/all-clients", lb.handleAllClients)  // 聚合所有节点的客户端
	
	// 所有其他请求都通过转发处理器
	http.HandleFunc("/", lb.handleRequest)
	
	log.Printf("纯七层负载均衡器启动在端口 %d", lb.port)
	log.Printf("负载均衡策略: %s", lb.strategy)
	
	return http.ListenAndServe(":"+strconv.Itoa(lb.port), nil)
}

// handleGlobalClients 负载均衡器的全局客户端API（读取JSON文件）
func (lb *LoadBalancer) handleGlobalClients(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	
	// 直接读取全局JSON文件
	globalClients := GetAllGlobalClients()
	
	var clients []GlobalClientInfo
	for _, client := range globalClients {
		clients = append(clients, *client)
	}
	
	response := map[string]interface{}{
		"source":  "loadbalancer",
		"total":   len(clients),
		"clients": clients,
	}
	
	json.NewEncoder(w).Encode(response)
}

// handleAllClients 聚合所有后端节点的客户端数据
func (lb *LoadBalancer) handleAllClients(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	
	lb.backendsMu.RLock()
	defer lb.backendsMu.RUnlock()
	
	allClients := make([]GlobalClientInfo, 0)
	totalClients := 0
	
	// 从所有健康的后端节点获取客户端数据
	for _, backend := range lb.backends {
		if !backend.IsHealthy {
			continue
		}
		
		// 从后端节点获取全局客户端数据
		nodeURL := fmt.Sprintf("%s/api/global-clients", backend.HTTPAddress)
		resp, err := http.Get(nodeURL)
		if err != nil {
			log.Printf("获取节点 %s 客户端数据失败: %v", backend.ID, err)
			continue
		}
		defer resp.Body.Close()
		
		var nodeResponse struct {
			Clients []GlobalClientInfo `json:"clients"`
			Total   int               `json:"total"`
		}
		
		if err := json.NewDecoder(resp.Body).Decode(&nodeResponse); err != nil {
			log.Printf("解析节点 %s 客户端数据失败: %v", backend.ID, err)
			continue
		}
		
		allClients = append(allClients, nodeResponse.Clients...)
		totalClients += nodeResponse.Total
	}
	
	// 去重处理（按客户端ID）
	uniqueClients := make(map[string]GlobalClientInfo)
	for _, client := range allClients {
		uniqueClients[client.ID] = client
	}
	
	finalClients := make([]GlobalClientInfo, 0, len(uniqueClients))
	for _, client := range uniqueClients {
		finalClients = append(finalClients, client)
	}
	
	response := map[string]interface{}{
		"source":         "aggregated_from_all_nodes",
		"total":          len(finalClients),
		"clients":        finalClients,
		"nodes_queried":  len(lb.backends),
		"healthy_nodes":  func() int {
			count := 0
			for _, backend := range lb.backends {
				if backend.IsHealthy {
					count++
				}
			}
			return count
		}(),
	}
	
	json.NewEncoder(w).Encode(response)
}