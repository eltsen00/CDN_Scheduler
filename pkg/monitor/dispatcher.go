package monitor

import (
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/eltsen00/CDN_Scheduler/pkg/hashring"
	"github.com/eltsen00/CDN_Scheduler/pkg/model"
)

// 定义调度器结构
type Dispatcher struct {
	ring  *hashring.HashRing
	nodes []*model.ATSNode
}

func NewDispatcher(ring *hashring.HashRing, nodes []*model.ATSNode) *Dispatcher {
	var wg sync.WaitGroup
	for _, node := range nodes {
		wg.Add(1)
		go func(n *model.ATSNode) {
			defer wg.Done()
			err := n.FetchAndUpdateStats(true) // 初始获取状态，强制更新 S
			if err != nil {
				log.Printf("警告: 初始獲取節點 %s 狀態失敗: %v", n.GetNodeName(), err)
			}
		}(node)
	}
	wg.Wait() // 等待所有初始状态获取完成

	interfaceNodes := make([]hashring.Node, len(nodes))
	for i, v := range nodes {
		interfaceNodes[i] = v
	}

	ring.Build(interfaceNodes) // 初始构建哈希环
	return &Dispatcher{
		ring:  ring,
		nodes: nodes,
	}
}

func (d *Dispatcher) StartTimeWindow(interval time.Duration) {
	ticker := time.NewTicker(interval)
	var counter int = 4 // 初始化为 4，这样第一次触发时就会更新 S
	var shouldUpdateS bool = false

	go func() {
		for range ticker.C {
			log.Println("--- 時間窗口到達，開始併發獲取 ATS 狀態 ---")
			counter++
			var wg sync.WaitGroup

			// 這裡決定是否要重新校準 S。
			if counter%5 == 0 {
				shouldUpdateS = true
				counter = 0 // 重置計數器
				log.Println("本次時間窗口將更新基準服務時間 S")
			} else {
				shouldUpdateS = false
			}

			// 併發獲取所有節點的狀態
			for _, node := range d.nodes {
				wg.Add(1)
				// 啟動 goroutine 併發請求，避免節點過多時阻塞
				go func(n *model.ATSNode) {
					defer wg.Done()
					err := n.FetchAndUpdateStats(shouldUpdateS)
					if err != nil {
						log.Printf("警告: 獲取節點 %s 狀態失敗: %v", n.GetNodeName(), err)
					}
				}(node)
			}

			// 等待所有 HTTP 請求完成
			wg.Wait()

			interfaceNodes := make([]hashring.Node, len(d.nodes))
			for i, v := range d.nodes {
				interfaceNodes[i] = v
			}

			// 狀態更新完畢，重建哈希環
			d.ring.Build(interfaceNodes)
		}
	}()
}

func (d *Dispatcher) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	urlPath := r.URL.Path
	nodeDomain := d.ring.GetNode(urlPath)
	switch nodeDomain {
	case "":
		http.Error(w, "服務不可用", http.StatusServiceUnavailable)
		log.Printf("錯誤: 哈希環為空或無可用節點，請求 %s 失敗", urlPath)
		return
	case r.Host:
		http.Error(w, "請求已經在正確的節點上", http.StatusBadRequest)
		log.Printf("警告: 請求 %s 已經在正確的節點 %s 上，無需重定向", urlPath, nodeDomain)
		return
	}
	redirectURL := "https://" + nodeDomain + r.URL.RequestURI() // 保持原始請求的 URI 不變
	http.Redirect(w, r, redirectURL, http.StatusFound)          // 302 重定向
	log.Printf("請求路徑: %s -> 重定向到: %s", urlPath, redirectURL)
}
