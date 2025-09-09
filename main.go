package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	service := flag.String("service", "server", "服务类型: server(服务端), client(客户端), loadbalancer(负载均衡器)")
	port := flag.Int("port", 8081, "服务器端口")
	nodeID := flag.String("node", "node1", "节点ID")
	mode := flag.String("mode", "single", "运行模式: single(单节点) 或 multi(多节点)")
	strategy := flag.String("strategy", "round_robin", "负载均衡策略: round_robin, least_conn, ip_hash")
	clientName := flag.String("name", "", "客户端名称")
	flag.Parse()

	// 初始化全局客户端注册表
	InitGlobalRegistry("global_clients.json")

	switch *service {
	case "server":
		switch *mode {
		case "single":
			runSingleNode(*port, *nodeID)
		case "multi":
			runMultiNodes()
		default:
			fmt.Println("无效的模式。可用模式: single, multi")
			os.Exit(1)
		}
	case "client":
		// 客户端有自己的参数处理，直接调用
		if *clientName != "" {
			// 如果在main中提供了name，传递给客户端
			InteractiveClient("ws://localhost:8080/ws", "ws://localhost:8080/ws", "", *clientName)
		} else {
			runClient()
		}
	case "loadbalancer":
		runLoadBalancer(*port, LoadBalanceStrategy(*strategy))
	default:
		fmt.Println("无效的服务类型。可用类型: server, client, loadbalancer")
		fmt.Println("使用示例:")
		fmt.Println("  负载均衡器: go run . -service=loadbalancer -port=8080 -strategy=round_robin")
		fmt.Println("  服务端: go run . -service=server -mode=single -port=8081 -node=node1")
		fmt.Println("  客户端: go run . -service=client -loadbalancer=ws://localhost:8080/ws -name=我的客户端")
		os.Exit(1)
	}
}

// 运行单节点
func runSingleNode(port int, nodeID string) {
	server := NewServer(port, nodeID)

	// 优雅关闭
	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt, syscall.SIGTERM)
		<-c
		log.Printf("正在关闭服务器节点 %s...", nodeID)
		os.Exit(0)
	}()

	log.Printf("启动单节点WebSocket服务器: %s (端口 %d)", nodeID, port)
	log.Fatal(server.Start())
}

// 运行多节点（演示用）
func runMultiNodes() {
	// 启动多个节点
	nodes := []struct {
		port int
		id   string
	}{
		{8081, "node1"},
		{8082, "node2"},
		{8083, "node3"},
	}

	for _, node := range nodes {
		go func(port int, id string) {
			server := NewServer(port, id)
			log.Printf("启动多节点服务器: %s (端口 %d)", id, port)
			log.Fatal(server.Start())
		}(node.port, node.id)
	}

	// 等待中断信号
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c
	log.Println("正在关闭所有服务器节点...")
}

// 运行负载均衡器
func runLoadBalancer(port int, strategy LoadBalanceStrategy) {
	lb := NewLoadBalancer(port, strategy)
	
	// 添加后端服务器（传入端口号，不再是ws地址）
	lb.AddBackend("node1", 8081)
	lb.AddBackend("node2", 8082)
	lb.AddBackend("node3", 8083)

	// 优雅关闭
	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt, syscall.SIGTERM)
		<-c
		log.Printf("正在关闭负载均衡器...")
		os.Exit(0)
	}()

	log.Fatal(lb.Start())
}

// 使用说明：
// 单节点启动: go run . -mode=single -port=8081 -node=node1
// 多节点启动: go run . -mode=multi
//
// 测试命令:
// curl http://localhost:8081/health
// curl http://localhost:8082/health
// curl http://localhost:8083/health
