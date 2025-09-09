package main

import (
	"encoding/json"
	"log"
	"os"
	"sync"
	"time"
)

// 全局客户端信息
type GlobalClientInfo struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	NodeID      string    `json:"node_id"`      // 连接到哪个节点
	NodePort    int       `json:"node_port"`    // 节点端口
	ConnTime    time.Time `json:"conn_time"`
	LastSeen    time.Time `json:"last_seen"`
	IsActive    bool      `json:"is_active"`
	Status      string    `json:"status"`       // online, offline, busy
}

// 全局客户端注册表
type GlobalClientRegistry struct {
	filePath string
	clients  map[string]*GlobalClientInfo
	mu       sync.RWMutex
}

var globalRegistry *GlobalClientRegistry

// 初始化全局客户端注册表
func InitGlobalRegistry(filePath string) {
	globalRegistry = &GlobalClientRegistry{
		filePath: filePath,
		clients:  make(map[string]*GlobalClientInfo),
	}
	globalRegistry.loadFromFile()
}

// 从文件加载客户端信息
func (gr *GlobalClientRegistry) loadFromFile() {
	gr.mu.Lock()
	defer gr.mu.Unlock()

	if _, err := os.Stat(gr.filePath); os.IsNotExist(err) {
		// 文件不存在，创建空的注册表
		gr.saveToFileUnsafe()
		return
	}

	data, err := os.ReadFile(gr.filePath)
	if err != nil {
		log.Printf("读取全局客户端文件失败: %v", err)
		return
	}

	var clients map[string]*GlobalClientInfo
	if err := json.Unmarshal(data, &clients); err != nil {
		log.Printf("解析全局客户端文件失败: %v", err)
		return
	}

	gr.clients = clients
	if gr.clients == nil {
		gr.clients = make(map[string]*GlobalClientInfo)
	}

	log.Printf("从文件加载了 %d 个全局客户端记录", len(gr.clients))
}

// 保存到文件（不加锁版本，内部使用）
func (gr *GlobalClientRegistry) saveToFileUnsafe() {
	data, err := json.MarshalIndent(gr.clients, "", "  ")
	if err != nil {
		log.Printf("序列化全局客户端数据失败: %v", err)
		return
	}

	if err := os.WriteFile(gr.filePath, data, 0644); err != nil {
		log.Printf("保存全局客户端文件失败: %v", err)
		return
	}
}

// 保存到文件
func (gr *GlobalClientRegistry) saveToFile() {
	gr.mu.Lock()
	defer gr.mu.Unlock()
	gr.saveToFileUnsafe()
}

// 注册客户端
func (gr *GlobalClientRegistry) RegisterClient(clientInfo *GlobalClientInfo) {
	gr.mu.Lock()
	defer gr.mu.Unlock()

	gr.clients[clientInfo.ID] = clientInfo
	gr.saveToFileUnsafe()

	log.Printf("全局注册客户端: %s (%s) -> 节点 %s:%d", 
		clientInfo.Name, clientInfo.ID, clientInfo.NodeID, clientInfo.NodePort)
}

// 注销客户端
func (gr *GlobalClientRegistry) UnregisterClient(clientID string) {
	gr.mu.Lock()
	defer gr.mu.Unlock()

	if client, exists := gr.clients[clientID]; exists {
		delete(gr.clients, clientID)
		gr.saveToFileUnsafe()
		log.Printf("全局注销客户端: %s (%s)", client.Name, clientID)
	}
}

// 更新客户端最后活跃时间
func (gr *GlobalClientRegistry) UpdateClientActivity(clientID string) {
	gr.mu.Lock()
	defer gr.mu.Unlock()

	if client, exists := gr.clients[clientID]; exists {
		client.LastSeen = time.Now()
		client.Status = "online"
		gr.saveToFileUnsafe()
	}
}

// 设置客户端状态
func (gr *GlobalClientRegistry) SetClientStatus(clientID, status string) {
	gr.mu.Lock()
	defer gr.mu.Unlock()

	if client, exists := gr.clients[clientID]; exists {
		client.Status = status
		client.LastSeen = time.Now()
		gr.saveToFileUnsafe()
	}
}

// 获取所有客户端
func (gr *GlobalClientRegistry) GetAllClients() map[string]*GlobalClientInfo {
	gr.mu.RLock()
	defer gr.mu.RUnlock()

	// 返回副本
	clients := make(map[string]*GlobalClientInfo)
	for id, client := range gr.clients {
		// 检查客户端是否超时（30秒无活动视为离线）
		if time.Since(client.LastSeen) > 30*time.Second {
			client.IsActive = false
			client.Status = "offline"
		} else {
			client.IsActive = true
			if client.Status == "" || client.Status == "offline" {
				client.Status = "online"
			}
		}
		clients[id] = client
	}

	return clients
}

// 获取指定客户端
func (gr *GlobalClientRegistry) GetClient(clientID string) (*GlobalClientInfo, bool) {
	gr.mu.RLock()
	defer gr.mu.RUnlock()

	client, exists := gr.clients[clientID]
	if exists {
		// 检查是否超时
		if time.Since(client.LastSeen) > 30*time.Second {
			client.IsActive = false
			client.Status = "offline"
		} else {
			client.IsActive = true
			if client.Status == "" || client.Status == "offline" {
				client.Status = "online"
			}
		}
	}

	return client, exists
}

// 根据节点获取客户端
func (gr *GlobalClientRegistry) GetClientsByNode(nodeID string) []*GlobalClientInfo {
	gr.mu.RLock()
	defer gr.mu.RUnlock()

	var clients []*GlobalClientInfo
	for _, client := range gr.clients {
		if client.NodeID == nodeID {
			// 检查是否超时
			if time.Since(client.LastSeen) > 30*time.Second {
				client.IsActive = false
				client.Status = "offline"
			} else {
				client.IsActive = true
				if client.Status == "" || client.Status == "offline" {
					client.Status = "online"
				}
			}
			clients = append(clients, client)
		}
	}

	return clients
}

// 清理离线客户端（超过5分钟无活动）
func (gr *GlobalClientRegistry) CleanupOfflineClients() {
	gr.mu.Lock()
	defer gr.mu.Unlock()

	cleaned := 0
	for id, client := range gr.clients {
		if time.Since(client.LastSeen) > 5*time.Minute {
			delete(gr.clients, id)
			cleaned++
		}
	}

	if cleaned > 0 {
		gr.saveToFileUnsafe()
		log.Printf("清理了 %d 个离线客户端", cleaned)
	}
}

// 启动定期清理任务
func (gr *GlobalClientRegistry) StartCleanupTask() {
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()

		for range ticker.C {
			gr.CleanupOfflineClients()
		}
	}()
}

// 全局函数接口
func RegisterGlobalClient(id, name, nodeID string, nodePort int) {
	if globalRegistry == nil {
		return
	}

	clientInfo := &GlobalClientInfo{
		ID:       id,
		Name:     name,
		NodeID:   nodeID,
		NodePort: nodePort,
		ConnTime: time.Now(),
		LastSeen: time.Now(),
		IsActive: true,
		Status:   "online",
	}

	globalRegistry.RegisterClient(clientInfo)
}

func UnregisterGlobalClient(clientID string) {
	if globalRegistry != nil {
		globalRegistry.UnregisterClient(clientID)
	}
}

func UpdateGlobalClientActivity(clientID string) {
	if globalRegistry != nil {
		globalRegistry.UpdateClientActivity(clientID)
	}
}

func SetGlobalClientStatus(clientID, status string) {
	if globalRegistry != nil {
		globalRegistry.SetClientStatus(clientID, status)
	}
}

func GetAllGlobalClients() map[string]*GlobalClientInfo {
	if globalRegistry == nil {
		return make(map[string]*GlobalClientInfo)
	}
	return globalRegistry.GetAllClients()
}

func GetGlobalClient(clientID string) (*GlobalClientInfo, bool) {
	if globalRegistry == nil {
		return nil, false
	}
	return globalRegistry.GetClient(clientID)
}