package main

import (
	"log"
	"net/http"
	"time"

	"github.com/eltsen00/CDN_Scheduler/pkg/hashring"
	"github.com/eltsen00/CDN_Scheduler/pkg/model"
	"github.com/eltsen00/CDN_Scheduler/pkg/monitor"
)

func redirectToHTTPS(w http.ResponseWriter, r *http.Request) {
	targetURL := "https://" + r.Host + r.URL.RequestURI()

	// 執行 301 永久重定向,使用 http.StatusMovedPermanently (301)
	http.Redirect(w, r, targetURL, http.StatusMovedPermanently)

	log.Printf("升級請求: HTTP -> HTTPS [%s]", targetURL)
}

// type redirectToHTTPS_Handler func(w http.ResponseWriter, r *http.Request)

// func (h redirectToHTTPS_Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
// 	h(w, r)
// }

func main() {
	// 1. 初始化节点
	nodes := []*model.ATSNode{
		// model.NewATSNode("ats1", "ats1.example.com", "http://ats1.example.com/stats", 1000),
		// model.NewATSNode("ats2", "ats2.example.com", "http://ats2.example.com/stats", 1000),
		// model.NewATSNode("ats3", "ats3.example.com", "http://ats3.example.com/stats", 1000),
		model.NewATSNode("ats1", "127.0.0.1:8081", "http://127.0.0.1:8081/stats", 1000), // 快
		model.NewATSNode("ats2", "127.0.0.1:8082", "http://127.0.0.1:8082/stats", 1000), // 中
		model.NewATSNode("ats3", "127.0.0.1:8083", "http://127.0.0.1:8083/stats", 1000), // 慢
	}

	// 2. 初始化哈希环
	ring := hashring.NewHashRing()

	// 3. 初始化调度器
	dispatcher := monitor.NewDispatcher(ring, nodes)

	// 4. 启动调度器
	dispatcher.StartTimeWindow(5 * time.Second)

	// 5. 启动 HTTP 服务，监听 :80 埠 (仅用于 HTTPS 重定向)
	go func() {
		log.Println("啟動 HTTP 服務，監聽 :80 埠 (僅用於 HTTPS 重定向)")

		err := http.ListenAndServe(":80", http.HandlerFunc(redirectToHTTPS))
		if err != nil {
			log.Fatalf("啟動 HTTP 服務失敗: %v", err)
		}
	}()

	// 6. 启动 HTTPS 服务，监听 :443 埠 (处理实际调度请求)
	log.Println("CDN 302 調度器啟動，監聽 :443 埠")
	err := http.ListenAndServeTLS(":443", "server.crt", "server.key", dispatcher)
	if err != nil {
		log.Fatalf("啟動 HTTPS 服務失敗: %v", err)
	}
}
