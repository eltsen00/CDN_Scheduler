package model

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"
)

// 定义 ATS 节点
type ATSNode struct {
	Name     string
	Domain   string
	StatsURL string  // 获取 ATS 统计数据的 URL
	MaxConns float64 // 最大连接数

	nodeMu  sync.RWMutex
	S       float64 // 当前的平均服务时间
	Util    float64 // 当前的利用率
	IsAlive bool    // 节点是否存活（可用）

	// 用於保存上一次的 Counter 狀態，以便計算時間窗口內的差值 (Delta)
	lastTotalTime float64
	lastRequests  float64
	isFirstFetch  bool
}

type ATSStatsResponse struct {
	CurrentClientConnections float64 `json:"proxy.process.http.current_client_connections"`
	TotalTransactionsTime    float64 `json:"proxy.process.http.total_transactions_time"`
	IncomingRequests         float64 `json:"proxy.process.http.incoming_requests"`
}

func NewATSNode(name, domain, url string, maxConns float64) *ATSNode {
	return &ATSNode{
		Name:         name,
		Domain:       domain,
		StatsURL:     url,
		MaxConns:     maxConns,
		isFirstFetch: true,
		IsAlive:      true,
	}
}

func (n *ATSNode) FetchAndUpdateStats(updateS bool) error {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(n.StatsURL)
	if err != nil {
		n.IsAlive = false // 如果请求失败，标记节点为不可用
		return err
	}
	defer resp.Body.Close()
	var stats ATSStatsResponse
	if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil { // 使用 json.NewDecoder 直接解码响应体（串流解析）
		n.IsAlive = false // 如果解析失败，标记节点为不可用
		return err
	}
	n.IsAlive = true // 成功获取并解析数据，标记节点为可用
	n.nodeMu.Lock()
	defer n.nodeMu.Unlock()

	// 计算当前的利用率
	n.Util = stats.CurrentClientConnections / n.MaxConns
	if n.Util > 0.99 {
		n.Util = 0.99 // 避免除以零或过高的利用率导致不合理的 S 计算
	}

	var currentResponseTime float64
	if !n.isFirstFetch {
		deltaTime := stats.TotalTransactionsTime - n.lastTotalTime
		deltaRequests := stats.IncomingRequests - n.lastRequests
		if deltaRequests >= 0 {
			currentResponseTime = (deltaTime / deltaRequests) * 1000.0 // 转换为毫秒
			log.Printf("節點 %s 當前平均響應時間: %.2f ms (deltaTime=%.2f ms, deltaRequests=%.0f)", n.Name, currentResponseTime, deltaTime*1000.0, deltaRequests)
		} else {
			log.Printf("警告: 节点 %s 计数器发生回退(ATS可能重启)，跳过本次计算", n.Name)
			// 仅更新 last 值，不计算 currentResponseTime
			n.lastTotalTime = stats.TotalTransactionsTime
			n.lastRequests = stats.IncomingRequests
			return nil // 或者不做任何后续 S 更新
		}
	} else {
		n.isFirstFetch = false
	}

	// 更新 Counter 狀態供下個時間窗口使用
	n.lastTotalTime = stats.TotalTransactionsTime
	n.lastRequests = stats.IncomingRequests

	// 利用公式 S = T * (1 - rho) 反推並鎖定基準服務時間 S，當前 T 是從 Counter 計算出來的平均響應時間，rho 是當前的利用率
	if updateS && currentResponseTime > 0 {
		n.S = currentResponseTime * (1.0 - n.Util)
		log.Printf("節點 %s 反推基準服務時間 S 更新為: %.2f ms (當前 T=%.2f ms, 活躍連接=%.0f, 利用率=%.2f)",
			n.Name, n.S, currentResponseTime, stats.CurrentClientConnections, n.Util)
	}
	return nil
}

func (n *ATSNode) CalculateWeight() int {
	n.nodeMu.RLock()
	defer n.nodeMu.RUnlock()

	if !n.IsAlive || n.S <= 0 {
		return 0 // 不可用或服务时间无效的节点权重为 0
	}

	if n.Util >= 0.99 {
		return 1 // 避免除以零或过高的利用率导致权重为零，至少返回 1
	}

	expectedTime := n.S / (1.0 - n.Util)       // 计算当前的预期响应时间
	weight := int((1.0 / expectedTime) * 1000) // 将权重放大以获得更明显的差异
	if weight < 1 {
		weight = 1 // 确保权重至少为 1
	}
	if weight > 1000 {
		weight = 1000
	}
	return weight
}

func (n *ATSNode) GetNodeName() string {
	return n.Name
}

func (n *ATSNode) GetNodeDomain() string {
	return n.Domain
}
