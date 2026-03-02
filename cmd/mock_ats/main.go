package main

import (
	"encoding/json"
	"flag"
	"log"
	"math/rand"
	"net/http"
	"sync"
	"time"
)

// ATSStatsResponse 模拟 ATS 返回的 JSON 结构
type ATSStatsResponse struct {
	CurrentClientConnections float64 `json:"proxy.process.http.current_client_connections"`
	TotalTransactionsTime    float64 `json:"proxy.process.http.total_transactions_time"`
	IncomingRequests         float64 `json:"proxy.process.http.incoming_requests"`
}

var (
	port        = flag.String("port", "8081", "服务监听端口")
	baseLatency = flag.Float64("latency", 50, "基础延迟(ms)")
	conns       = flag.Float64("conns", 100, "当前连接数")
)

var (
	mu           sync.Mutex
	totalReqs    float64 = 0
	totalTimeSec float64 = 0
	lastTime     time.Time
)

func statsHandler(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	defer mu.Unlock()

	// 1. 计算距离上次请求的时间差 (模拟真实的请求速率)
	now := time.Now()
	if lastTime.IsZero() {
		lastTime = now
	}
	elapsed := now.Sub(lastTime).Seconds()
	lastTime = now

	// 2. 模拟每秒 100 个请求 (RPS = 100)
	// 同时也加上一点随机性
	rps := 100.0 + rand.Float64()*20.0
	newReqs := rps * elapsed
	if newReqs < 0 {
		newReqs = 0
	} // 防御性

	// 3. 累加总请求数
	totalReqs += newReqs

	// 4. 计算新增的总耗时
	// 当前的平均延迟 (ms)
	avgLat := *baseLatency + float64(rand.Intn(10)) - 5.0
	if avgLat < 1 {
		avgLat = 1
	}

	// 新增耗时 (秒) = 新增请求数 * (延迟ms / 1000)
	addedTime := newReqs * (avgLat / 1000.0)
	totalTimeSec += addedTime

	// 5. 连接数波动
	currentConns := *conns + float64(rand.Intn(20)-10)

	stats := ATSStatsResponse{
		CurrentClientConnections: currentConns,
		TotalTransactionsTime:    totalTimeSec, // 单位：秒
		IncomingRequests:         totalReqs,    // 单位：次
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)

	log.Printf("ATS Mock on :%s | Latency: %.2fms | TotalReqs: %.0f", *port, avgLat, totalReqs)
}

func main() {
	flag.Parse()
	lastTime = time.Now() // 初始化时间

	http.HandleFunc("/stats", statsHandler)
	log.Printf("启动模拟 ATS 节点，监听端口 :%s，基础延迟 %.2fms", *port, *baseLatency)
	log.Fatal(http.ListenAndServe(":"+*port, nil))
}
