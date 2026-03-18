package main

import (
	"log"
	"net/http"
	"time"

	"github.com/eltsen00/CDN_Scheduler/config"
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

func main() {
	cfg := config.Load()
	httpAddr := cfg.HTTPAddr()
	httpsAddr := cfg.HTTPSAddr()
	certFile := cfg.Server.HTTPS.CertFile
	keyFile := cfg.Server.HTTPS.KeyFile

	// 1. 初始化节点
	nodes := make([]*model.ATSNode, 0, len(cfg.ATSNodes))
	for _, nodeCfg := range cfg.ATSNodes {
		nodes = append(nodes, model.NewATSNode(nodeCfg.Name, nodeCfg.Domain, nodeCfg.StatsURL, nodeCfg.MaxConns))
	}

	// 2. 初始化哈希环
	ring := hashring.NewHashRing()

	// 3. 初始化调度器
	dispatcher := monitor.NewDispatcher(ring, nodes)

	// 4. 启动调度器
	dispatcher.StartTimeWindow(5 * time.Second)

	// 5. 启动 HTTP 服务（仅用于 HTTPS 重定向）
	go func() {
		log.Printf("啟動 HTTP 服務，監聽 %s (僅用於 HTTPS 重定向)", httpAddr)

		err := http.ListenAndServe(httpAddr, http.HandlerFunc(redirectToHTTPS))
		if err != nil {
			log.Fatalf("啟動 HTTP 服務失敗: %v", err)
		}
	}()

	// 6. 启动 HTTPS 服务（处理实际调度请求）
	log.Printf("CDN 302 調度器啟動，監聽 %s", httpsAddr)
	err := http.ListenAndServeTLS(httpsAddr, certFile, keyFile, dispatcher)
	if err != nil {
		log.Fatalf("啟動 HTTPS 服務失敗: %v", err)
	}
}
