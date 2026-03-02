package hashring

import (
	"hash/fnv"
	"log"
	"slices"
	"strconv"
	"sync"
)

// 定义哈希环结构
type HashRing struct {
	mu       sync.RWMutex
	hashKeys []uint32          // 哈希环上的键（节点位置，排序后）
	ring     map[uint32]string // 哈希键 -> 物理节点地址 的映射
}

type Node interface {
	CalculateWeight() int
	GetNodeName() string
	GetNodeDomain() string
}

func NewHashRing() *HashRing {
	return &HashRing{
		ring: make(map[uint32]string),
	}
}

func hash(key string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(key))
	return h.Sum32()
}

func (hr *HashRing) Build(nodes []Node) {
	newRing := make(map[uint32]string)
	var newKeys []uint32

	for _, node := range nodes {
		weight := node.CalculateWeight()
		for i := range weight {
			virtualNodeKey := hash(node.GetNodeName() + "#" + strconv.Itoa(i)) // 生成虚拟节点的哈希键
			newRing[virtualNodeKey] = node.GetNodeDomain()
			newKeys = append(newKeys, virtualNodeKey)
		}
	}

	slices.Sort(newKeys) // 对哈希键进行排序(从小到大)

	hr.mu.Lock()
	hr.ring = newRing
	hr.hashKeys = newKeys
	hr.mu.Unlock()

	log.Printf("哈希環重建完成，總虛擬節點數: %d", len(newKeys))
}

func (hr *HashRing) GetNode(urlPath string) string {
	hr.mu.RLock()
	defer hr.mu.RUnlock()

	if len(hr.hashKeys) == 0 {
		return ""
	}

	hashKey := hash(urlPath)

	// 查找位置
	idx, _ := slices.BinarySearch(hr.hashKeys, hashKey)

	// BinarySearch 返回的是插入位置
	if idx == len(hr.hashKeys) {
		idx = 0 // 回环到第一个节点
	}

	return hr.ring[hr.hashKeys[idx]]
}
