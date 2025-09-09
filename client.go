package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
)

// WebSocketClient WebSocket客户端
type WebSocketClient struct {
	conn       *websocket.Conn
	clientID   string
	clientName string
	proxyURL   string
	serverURL  string
}

// NewClient 创建客户端
func NewClient(proxyURL, serverURL, clientID, clientName string) (*WebSocketClient, error) {
	return &WebSocketClient{
		clientID:   clientID,
		clientName: clientName,
		proxyURL:   proxyURL,
		serverURL:  serverURL,
	}, nil
}

// 连接到负载均衡器
func (c *WebSocketClient) ConnectToLoadBalancer() error {
	u, err := url.Parse(c.proxyURL)
	if err != nil {
		return err
	}

	log.Printf("连接到负载均衡器: %s", c.proxyURL)
	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return err
	}

	c.conn = conn

	// 发送注册消息
	registerMsg := map[string]interface{}{
		"client_id":   c.clientID,
		"client_name": c.clientName,
		"timestamp":   time.Now().Unix(),
	}

	if err := conn.WriteJSON(registerMsg); err != nil {
		conn.Close()
		return err
	}

	log.Printf("✅ 客户端注册成功: %s (%s)", c.clientName, c.clientID)
	return nil
}

// SendMessage 发送消息
func (c *WebSocketClient) SendMessage(method, path string, body interface{}) error {
	msg := NewMessage(method, path, body)

	if err := c.conn.WriteJSON(msg); err != nil {
		return err
	}

	fmt.Printf("发送: %s %s\n", method, path)
	return nil
}

// ReceiveResponse 接收响应
func (c *WebSocketClient) ReceiveResponse() (*WebSocketResponse, error) {
	var resp WebSocketResponse
	err := c.conn.ReadJSON(&resp)
	if err != nil {
		return nil, err
	}

	fmt.Printf("接收: 状态%d\n", resp.Status)
	if resp.Body != nil {
		bodyJSON, _ := json.MarshalIndent(resp.Body, "", "  ")
		fmt.Printf("响应体: %s\n", string(bodyJSON))
	}
	if resp.Error != "" {
		fmt.Printf("错误: %s\n", resp.Error)
	}

	return &resp, nil
}

// HandleServerMessages 处理服务器消息
func (c *WebSocketClient) HandleServerMessages() {
	for {
		var msg map[string]interface{}
		err := c.conn.ReadJSON(&msg)
		if err != nil {
			log.Printf("读取服务器消息失败: %v", err)
			break
		}

		c.handleServerMessage(msg)
	}
}

// 处理服务器消息
func (c *WebSocketClient) handleServerMessage(msg map[string]interface{}) {
	msgType, ok := msg["type"].(string)
	if !ok {
		log.Printf("收到无效消息: %v", msg)
		return
	}

	switch msgType {
	case "command":
		// 处理服务器发送的指令
		c.handleCommand(msg)

	case "query_name":
		// 服务器查询客户端名字
		log.Printf("📨 收到服务器查询名字请求")

		// 回复客户端名字
		replyMsg := map[string]interface{}{
			"type":        "name_response",
			"client_id":   c.clientID,
			"client_name": c.clientName,
			"timestamp":   time.Now().Unix(),
		}

		if err := c.conn.WriteJSON(replyMsg); err != nil {
			log.Printf("回复客户端名字失败: %v", err)
		} else {
			log.Printf("✅ 已回复客户端名字: %s", c.clientName)
		}

	case "ping":
		// 心跳检测
		pongMsg := map[string]interface{}{
			"type":      "pong",
			"timestamp": time.Now().Unix(),
		}
		c.conn.WriteJSON(pongMsg)

	default:
		log.Printf("收到消息: %s", msgType)
	}
}

// handleCommand 处理服务器发送的指令
func (c *WebSocketClient) handleCommand(msg map[string]interface{}) {
	command, ok := msg["command"].(string)
	if !ok {
		log.Printf("❌ 收到无效指令: %v", msg)
		c.sendCommandResponse("error", "无效的指令格式", nil)
		return
	}

	log.Printf("📨 收到指令: %s", command)

	var responseType, responseMessage string
	var responseData interface{}

	switch command {
	case "ping":
		// Ping测试
		responseType = "success"
		responseMessage = "pong"
		responseData = map[string]interface{}{
			"client_info": map[string]interface{}{
				"id":   c.clientID,
				"name": c.clientName,
			},
			"server_time": time.Now().Unix(),
			"latency":     "< 1ms",
		}

	case "status":
		// 获取客户端状态
		responseType = "success"
		responseMessage = "客户端状态正常"
		responseData = map[string]interface{}{
			"client_id":   c.clientID,
			"client_name": c.clientName,
			"status":      "online",
			"uptime":      time.Now().Unix(), // 简化版，实际应该记录启动时间
			"version":     "1.0.0",
			"platform":    "Go WebSocket Client",
		}

	case "restart":
		// 重启连接
		responseType = "success"
		responseMessage = "即将重启连接"
		responseData = map[string]interface{}{
			"restart_in": "3 seconds",
		}

		// 发送响应后重启连接
		c.sendCommandResponse(responseType, responseMessage, responseData)
		
		log.Printf("🔄 3秒后重启连接...")
		go func() {
			time.Sleep(3 * time.Second)
			c.conn.Close() // 关闭连接，触发重连机制
		}()
		return

	case "info":
		// 获取详细信息
		responseType = "success"
		responseMessage = "客户端信息"
		responseData = map[string]interface{}{
			"client_id":     c.clientID,
			"client_name":   c.clientName,
			"proxy_url":     c.proxyURL,
			"server_url":    c.serverURL,
			"connected":     c.conn != nil,
			"timestamp":     time.Now().Unix(),
			"capabilities": []string{"ping", "status", "restart", "info", "echo"},
		}

	default:
		// 处理自定义指令
		if command == "echo" {
			// Echo指令 - 回显数据
			data := msg["data"]
			responseType = "success"
			responseMessage = "echo响应"
			responseData = map[string]interface{}{
				"original_data": data,
				"echo_time":     time.Now().Unix(),
			}
		} else {
			// 未知指令
			responseType = "error"
			responseMessage = fmt.Sprintf("未知指令: %s", command)
			responseData = map[string]interface{}{
				"supported_commands": []string{"ping", "status", "restart", "info", "echo"},
			}
		}
	}

	// 发送响应
	c.sendCommandResponse(responseType, responseMessage, responseData)
}

// sendCommandResponse 发送指令响应
func (c *WebSocketClient) sendCommandResponse(responseType, message string, data interface{}) {
	response := map[string]interface{}{
		"type":      "command_response",
		"result":    responseType,
		"message":   message,
		"data":      data,
		"client_id": c.clientID,
		"timestamp": time.Now().Unix(),
	}

	if err := c.conn.WriteJSON(response); err != nil {
		log.Printf("❌ 发送指令响应失败: %v", err)
	} else {
		log.Printf("✅ 已发送指令响应: %s - %s", responseType, message)
	}
}

// Close 关闭连接
func (c *WebSocketClient) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// StartWithAutoReconnect 启动客户端并支持自动重连
func (c *WebSocketClient) StartWithAutoReconnect() {
	// 创建上下文用于优雅关闭
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 监听系统信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// 启动重连循环
	go c.reconnectLoop(ctx)

	// 等待系统信号
	<-sigChan
	log.Printf("收到关闭信号，正在优雅关闭客户端...")
	cancel()
	
	// 给一些时间清理资源
	time.Sleep(1 * time.Second)
	c.Close()
	log.Printf("客户端已关闭")
}

// reconnectLoop 自动重连循环
func (c *WebSocketClient) reconnectLoop(ctx context.Context) {
	retryCount := 0
	baseDelay := 2 * time.Second
	maxDelay := 30 * time.Second

	for {
		select {
		case <-ctx.Done():
			log.Printf("重连循环收到关闭信号，退出")
			return
		default:
		}

		// 尝试连接
		err := c.ConnectToLoadBalancer()
		if err != nil {
			retryCount++
			// 计算退避延迟
			delay := time.Duration(retryCount) * baseDelay
			if delay > maxDelay {
				delay = maxDelay
			}
			
			log.Printf("❌ 连接失败 (第%d次重试): %v", retryCount, err)
			log.Printf("⏳ %v 后重试连接...", delay)
			
			// 等待重试或接收关闭信号
			select {
			case <-ctx.Done():
				return
			case <-time.After(delay):
				continue
			}
		}

		// 连接成功，重置重试计数
		if retryCount > 0 {
			log.Printf("✅ 重连成功! (共重试%d次)", retryCount)
		} else {
			log.Printf("✅ 首次连接成功")
		}
		retryCount = 0

		// 启动消息处理
		messageErrChan := make(chan error, 1)
		go func() {
			messageErrChan <- c.handleMessagesWithReconnect(ctx)
		}()

		// 等待连接断开或关闭信号
		select {
		case <-ctx.Done():
			c.Close()
			return
		case err := <-messageErrChan:
			if err != nil {
				log.Printf("🔗 连接中断: %v", err)
			}
			c.Close()
			log.Printf("🔄 准备重连...")
			time.Sleep(1 * time.Second) // 短暂等待后重连
		}
	}
}

// handleMessagesWithReconnect 处理消息并支持重连
func (c *WebSocketClient) handleMessagesWithReconnect(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		var msg map[string]interface{}
		err := c.conn.ReadJSON(&msg)
		if err != nil {
			// 检查是否是正常关闭
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				log.Printf("服务器正常关闭连接")
				return err
			}
			// 其他错误表示异常断开，需要重连
			return fmt.Errorf("读取消息失败: %v", err)
		}

		// 处理收到的消息
		c.handleServerMessage(msg)
	}
}

// InteractiveClient 交互式客户端（带自动重连）
func InteractiveClient(loadbalancerURL, serverURL, clientID, clientName string) {
	client, err := NewClient(loadbalancerURL, serverURL, clientID, clientName)
	if err != nil {
		log.Fatal("创建客户端失败:", err)
	}

	fmt.Println("WebSocket客户端启动")
	fmt.Println("负载均衡器:", loadbalancerURL) 
	fmt.Println("客户端ID:", clientID)
	fmt.Println("客户端名称:", clientName)
	fmt.Println("自动重连: 已启用")
	fmt.Println()
	fmt.Println("按 Ctrl+C 退出")
	fmt.Println()

	// 启动自动重连循环
	client.StartWithAutoReconnect()
}

// main 函数用于独立运行客户端
func runClient() {
	loadbalancerURL := flag.String("loadbalancer", "ws://localhost:8080/ws", "负载均衡器地址")
	serverURL := flag.String("server", "ws://localhost:8080/ws", "服务端地址") 
	clientID := flag.String("id", "", "客户端ID (可选)")
	clientName := flag.String("name", "", "客户端名称 (可选)")
	flag.Parse()

	// 生成默认的客户端ID和名称
	if *clientID == "" {
		*clientID = fmt.Sprintf("client_%d_%s", time.Now().Unix(), generateRandomString(6))
	}
	if *clientName == "" {
		*clientName = fmt.Sprintf("客户端_%s", (*clientID)[len(*clientID)-6:])
	}

	fmt.Println("启动Go WebSocket客户端")
	fmt.Println("负载均衡器:", *loadbalancerURL)
	fmt.Println("服务端:", *serverURL)
	fmt.Println("客户端ID:", *clientID)
	fmt.Println("客户端名称:", *clientName)
	fmt.Println()

	InteractiveClient(*loadbalancerURL, *serverURL, *clientID, *clientName)
}

// 生成随机字符串
func generateRandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[time.Now().UnixNano()%int64(len(charset))]
	}
	return string(b)
}

// 测试用例示例:
// GET /info - 获取服务器信息
// GET /health - 健康检查
// POST /users {"name": "张三", "age": 25}
// PUT /users/1 {"name": "李四", "age": 30}
// DELETE /users/1
