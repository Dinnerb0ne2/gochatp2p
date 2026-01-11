package main

import (
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"
)

// SuperNodeInfo 保存SuperNode的信息
type SuperNodeInfo struct {
	NodeInfo
	IsSuperNode bool      `json:"is_super_node"`
	LastActive  time.Time `json:"last_active"`
}

// SuperNodeManager SuperNode管理器
type SuperNodeManager struct {
	mu            sync.RWMutex
	supernodes    []SuperNodeInfo
	localNodeInfo NodeInfo
	messageKey    []byte
	tcpPort       int
	udpPort       int
	isSuperNode   bool
	noSuperNode   bool
	superNodeMode bool // 是否启用SuperNode模式
}

// NewSuperNodeManager 创建新的SuperNode管理器
func NewSuperNodeManager(localNode NodeInfo, messageKey []byte, tcpPort, udpPort int, noSuperNode bool) *SuperNodeManager {
	return &SuperNodeManager{
		localNodeInfo: localNode,
		messageKey:    messageKey,
		tcpPort:       tcpPort,
		udpPort:       udpPort,
		noSuperNode:   noSuperNode,
		superNodeMode: true, // 默认启用SuperNode模式
	}
}

// IsSuperNodeModeEnabled 检查是否启用SuperNode模式
func (sm *SuperNodeManager) IsSuperNodeModeEnabled() bool {
	return sm.superNodeMode
}

// ShouldEnableSuperNodeMode 根据节点数量判断是否应该启用SuperNode模式
func (sm *SuperNodeManager) ShouldEnableSuperNodeMode(nodeCount int) bool {
	return sm.superNodeMode && nodeCount > 5
}

// SetSuperNodeMode 设置SuperNode模式
func (sm *SuperNodeManager) SetSuperNodeMode(enabled bool) {
	sm.superNodeMode = enabled
}

// IsLocalNodeSuperNode 检查本地节点是否为SuperNode
func (sm *SuperNodeManager) IsLocalNodeSuperNode() bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.isSuperNode
}

// SetLocalNodeAsSuperNode 设置本地节点为SuperNode
func (sm *SuperNodeManager) SetLocalNodeAsSuperNode(isSuperNode bool) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.isSuperNode = isSuperNode
}

// IsNoSuperNode 检查是否配置为不成为SuperNode
func (sm *SuperNodeManager) IsNoSuperNode() bool {
	return sm.noSuperNode
}

// AddNode 添加节点到SuperNode列表
func (sm *SuperNodeManager) AddNode(nodeInfo NodeInfo) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// 检查节点是否已存在
	for i, sn := range sm.supernodes {
		if sn.ID == nodeInfo.ID {
			// 更新最后活跃时间和其他信息
			sm.supernodes[i].Address = nodeInfo.Address
			sm.supernodes[i].Nickname = nodeInfo.Nickname
			sm.supernodes[i].NoSuperNode = nodeInfo.NoSuperNode
			sm.supernodes[i].LastActive = time.Now()
			return
		}
	}

	// 添加新节点
	superNodeInfo := SuperNodeInfo{
		NodeInfo:    nodeInfo,
		IsSuperNode: false, // 默认不是SuperNode
		LastActive:  time.Now(),
	}
	sm.supernodes = append(sm.supernodes, superNodeInfo)
}

// SetAsSuperNode 将指定节点设为SuperNode
func (sm *SuperNodeManager) SetAsSuperNode(nodeID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	for i, sn := range sm.supernodes {
		if sn.ID == nodeID {
			sm.supernodes[i].IsSuperNode = true
			sm.supernodes[i].LastActive = time.Now()
			return
		}
	}
}

// GetSuperNodes 获取所有活跃的SuperNodes
func (sm *SuperNodeManager) GetSuperNodes() []SuperNodeInfo {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	// 过滤掉超时的节点
	var activeSuperNodes []SuperNodeInfo
	timeout := 30 * time.Second // 超时30秒
	for _, sn := range sm.supernodes {
		if time.Since(sn.LastActive) < timeout && sn.IsSuperNode {
			activeSuperNodes = append(activeSuperNodes, sn)
		}
	}

	return activeSuperNodes
}

// GetRegularNodes 获取所有普通节点
func (sm *SuperNodeManager) GetRegularNodes() []NodeInfo {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	var regularNodes []NodeInfo
	timeout := 30 * time.Second // 超时30秒
	for _, sn := range sm.supernodes {
		if time.Since(sn.LastActive) < timeout && !sn.IsSuperNode {
			regularNodes = append(regularNodes, sn.NodeInfo)
		}
	}

	return regularNodes
}

// GetNode 获取指定节点信息
func (sm *SuperNodeManager) GetNode(nodeID string) *SuperNodeInfo {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	for _, sn := range sm.supernodes {
		if sn.ID == nodeID {
			return &sn
		}
	}

	return nil
}

// RemoveNode 移除节点
func (sm *SuperNodeManager) RemoveNode(nodeID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	for i, sn := range sm.supernodes {
		if sn.ID == nodeID {
			sm.supernodes = append(sm.supernodes[:i], sm.supernodes[i+1:]...)
			return
		}
	}
}

// UpdateNodeActivity 更新节点活跃时间
func (sm *SuperNodeManager) UpdateNodeActivity(nodeID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	for i, sn := range sm.supernodes {
		if sn.ID == nodeID {
			sm.supernodes[i].LastActive = time.Now()
			return
		}
	}
}

// SelectInitialSuperNode 从前5个节点中随机选择一个作为初始SuperNode
func (sm *SuperNodeManager) SelectInitialSuperNode() string {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// 从前5个节点中选择
	var candidates []SuperNodeInfo
	count := 0
	for _, sn := range sm.supernodes {
		if count >= 5 {
			break
		}
		// 不选择配置为noSuperNode的节点
		if sn.ID != sm.localNodeInfo.Address {
			nodeIsNoSuperNode := sm.checkIfNodeIsNoSuperNode(sn.ID)
			if !nodeIsNoSuperNode {
				candidates = append(candidates, sn)
				count++
			}
		}
	}

	if len(candidates) == 0 {
		return ""
	}

	// 随机选择一个（使用当前时间作为随机源）
	index := int(time.Now().Unix()) % len(candidates)
	selectedID := candidates[index].ID

	// 设置为SuperNode
	for i, sn := range sm.supernodes {
		if sn.ID == selectedID {
			sm.supernodes[i].IsSuperNode = true
			sm.supernodes[i].LastActive = time.Now()
			if selectedID == sm.localNodeInfo.Address {
				sm.isSuperNode = true
			}
			break
		}
	}

	return selectedID
}

// 检查节点是否配置为noSuperNode
func (sm *SuperNodeManager) checkIfNodeIsNoSuperNode(nodeID string) bool {
	nodeInfo := sm.GetNode(nodeID)
	if nodeInfo != nil {
		return nodeInfo.NodeInfo.NoSuperNode
	}
	return false
}

// HandleNodeLeave 处理节点离开事件
func (sm *SuperNodeManager) HandleNodeLeave(nodeID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// 移除节点
	for i, sn := range sm.supernodes {
		if sn.ID == nodeID {
			// 如果离开的是SuperNode，需要选举新的SuperNode
			if sn.IsSuperNode {
				sm.handleSuperNodeLeave()
			}

			sm.supernodes = append(sm.supernodes[:i], sm.supernodes[i+1:]...)
			return
		}
	}
}

// handleSuperNodeLeave 处理SuperNode离开事件
func (sm *SuperNodeManager) handleSuperNodeLeave() {
	// 检查是否还有其他活跃的SuperNode
	activeSuperNodes := 0
	for _, sn := range sm.supernodes {
		if sn.IsSuperNode && time.Since(sn.LastActive) < 30*time.Second {
			activeSuperNodes++
		}
	}

	// 如果没有其他SuperNode了，需要选举一个新的
	if activeSuperNodes <= 1 { // 当前离开的SuperNode也计算在内
		sm.selectNewSuperNode()
	}
}

// selectNewSuperNode 选择新的SuperNode
func (sm *SuperNodeManager) selectNewSuperNode() {
	// 从活跃的普通节点中选择一个新的SuperNode
	timeout := 30 * time.Second
	for i, sn := range sm.supernodes {
		if !sn.IsSuperNode &&
			time.Since(sn.LastActive) < timeout &&
			sn.ID != sm.localNodeInfo.Address {
			nodeIsNoSuperNode := sm.checkIfNodeIsNoSuperNode(sn.ID)
			if !nodeIsNoSuperNode {
				// 设置为SuperNode
				sm.supernodes[i].IsSuperNode = true
				sm.supernodes[i].LastActive = time.Now()
				return
			}
		}
	}

	// 如果没有合适的普通节点，且本地节点不设置为noSuperNode，则本地节点成为SuperNode
	if !sm.noSuperNode && !sm.isSuperNode {
		sm.isSuperNode = true
	}
}

// ForwardMessageToSuperNodes 将消息转发给SuperNodes
func (sm *SuperNodeManager) ForwardMessageToSuperNodes(message Message, messageKey []byte) error {
	superNodes := sm.GetSuperNodes()

	for _, superNode := range superNodes {
		if superNode.Address == sm.localNodeInfo.Address {
			continue // 不发送给自己
		}

		go func(nodeAddr string) {
			conn, err := net.DialTimeout("tcp", nodeAddr, 5*time.Second)
			if err != nil {
				fmt.Printf("Failed to connect to SuperNode %s: %v\n", nodeAddr, err)
				return
			}
			defer conn.Close()

			// 序列化消息
			messageData, err := json.Marshal(message)
			if err != nil {
				fmt.Printf("Failed to serialize message: %v\n", err)
				return
			}

			// 加密消息
			encryptedData, err := encryptAES(messageKey, messageData)
			if err != nil {
				fmt.Printf("Failed to encrypt message: %v\n", err)
				return
			}

			_, err = conn.Write(encryptedData)
			if err != nil {
				fmt.Printf("Failed to send message to SuperNode %s: %v\n", nodeAddr, err)
			}
		}(superNode.Address)
	}

	return nil
}

// GetBestSuperNodeForConnection 获取最佳的SuperNode进行连接
func (sm *SuperNodeManager) GetBestSuperNodeForConnection() *SuperNodeInfo {
	superNodes := sm.GetSuperNodes()
	if len(superNodes) == 0 {
		return nil
	}

	// 返回第一个SuperNode（可以实现更复杂的负载均衡算法）
	return &superNodes[0]
}
