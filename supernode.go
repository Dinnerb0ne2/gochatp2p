package main

import (
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"
)

// SuperNodeInfo stores information about a SuperNode
type SuperNodeInfo struct {
	NodeInfo
	IsSuperNode bool      `json:"is_super_node"`
	LastActive  time.Time `json:"last_active"`
}

// SuperNodeManager SuperNode manager
type SuperNodeManager struct {
	mu            sync.RWMutex
	supernodes    []SuperNodeInfo
	localNodeInfo NodeInfo
	messageKey    []byte
	tcpPort       int
	udpPort       int
	isSuperNode   bool
	noSuperNode   bool
	superNodeMode bool // Whether to enable SuperNode mode
}

// NewSuperNodeManager creates a new SuperNode manager
func NewSuperNodeManager(localNode NodeInfo, messageKey []byte, tcpPort, udpPort int, noSuperNode bool) *SuperNodeManager {
	return &SuperNodeManager{
		localNodeInfo: localNode,
		messageKey:    messageKey,
		tcpPort:       tcpPort,
		udpPort:       udpPort,
		noSuperNode:   noSuperNode,
		superNodeMode: true, // Enable SuperNode mode by default
	}
}

// IsSuperNodeModeEnabled checks if SuperNode mode is enabled
func (sm *SuperNodeManager) IsSuperNodeModeEnabled() bool {
	return sm.superNodeMode
}

// ShouldEnableSuperNodeMode determines whether SuperNode mode should be enabled based on node count
func (sm *SuperNodeManager) ShouldEnableSuperNodeMode(nodeCount int) bool {
	return sm.superNodeMode && nodeCount > 5
}

// SetSuperNodeMode sets SuperNode mode
func (sm *SuperNodeManager) SetSuperNodeMode(enabled bool) {
	sm.superNodeMode = enabled
}

// IsLocalNodeSuperNode checks if the local node is a SuperNode
func (sm *SuperNodeManager) IsLocalNodeSuperNode() bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.isSuperNode
}

// SetLocalNodeAsSuperNode sets the local node as a SuperNode
func (sm *SuperNodeManager) SetLocalNodeAsSuperNode(isSuperNode bool) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.isSuperNode = isSuperNode
}

// IsNoSuperNode checks if the node is configured not to become a SuperNode
func (sm *SuperNodeManager) IsNoSuperNode() bool {
	return sm.noSuperNode
}

// AddNode adds a node to the SuperNode list
func (sm *SuperNodeManager) AddNode(nodeInfo NodeInfo) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Check if node already exists
	for i, sn := range sm.supernodes {
		if sn.ID == nodeInfo.ID {
			// Update last active time and other info
			sm.supernodes[i].Address = nodeInfo.Address
			sm.supernodes[i].Nickname = nodeInfo.Nickname
			sm.supernodes[i].NoSuperNode = nodeInfo.NoSuperNode
			sm.supernodes[i].LastActive = time.Now()
			return
		}
	}

	// Add new node
	superNodeInfo := SuperNodeInfo{
		NodeInfo:    nodeInfo,
		IsSuperNode: false, // Default is not a SuperNode
		LastActive:  time.Now(),
	}
	sm.supernodes = append(sm.supernodes, superNodeInfo)
}

// SetAsSuperNode sets the specified node as a SuperNode
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

// GetSuperNodes gets all active SuperNodes
func (sm *SuperNodeManager) GetSuperNodes() []SuperNodeInfo {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	// Filter out timed-out nodes
	var activeSuperNodes []SuperNodeInfo
	timeout := 30 * time.Second // 30 second timeout
	for _, sn := range sm.supernodes {
		if time.Since(sn.LastActive) < timeout && sn.IsSuperNode {
			activeSuperNodes = append(activeSuperNodes, sn)
		}
	}

	return activeSuperNodes
}

// GetRegularNodes gets all regular nodes
func (sm *SuperNodeManager) GetRegularNodes() []NodeInfo {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	var regularNodes []NodeInfo
	timeout := 30 * time.Second // 30 second timeout
	for _, sn := range sm.supernodes {
		if time.Since(sn.LastActive) < timeout && !sn.IsSuperNode {
			regularNodes = append(regularNodes, sn.NodeInfo)
		}
	}

	return regularNodes
}

// GetNode gets the specified node info
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

// RemoveNode removes a node
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

// UpdateNodeActivity updates node activity time
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

// SelectInitialSuperNode randomly selects one as the initial SuperNode from the first 5 nodes
func (sm *SuperNodeManager) SelectInitialSuperNode() string {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Select from first 5 nodes
	var candidates []SuperNodeInfo
	count := 0
	for _, sn := range sm.supernodes {
		if count >= 5 {
			break
		}
		// Don't select nodes configured with noSuperNode
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

	// Randomly select one (using current time as random source)
	index := int(time.Now().Unix()) % len(candidates)
	selectedID := candidates[index].ID

	// Set as SuperNode
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

// checkIfNodeIsNoSuperNode checks if the node is configured as noSuperNode
func (sm *SuperNodeManager) checkIfNodeIsNoSuperNode(nodeID string) bool {
	nodeInfo := sm.GetNode(nodeID)
	if nodeInfo != nil {
		return nodeInfo.NodeInfo.NoSuperNode
	}
	return false
}

// HandleNodeLeave handles the node leave event
func (sm *SuperNodeManager) HandleNodeLeave(nodeID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Remove node
	for i, sn := range sm.supernodes {
		if sn.ID == nodeID {
			// If the leaving node is a SuperNode, need to elect a new SuperNode
			if sn.IsSuperNode {
				sm.handleSuperNodeLeave()
			}

			sm.supernodes = append(sm.supernodes[:i], sm.supernodes[i+1:]...)
			return
		}
	}
}

// handleSuperNodeLeave handles the SuperNode leave event
func (sm *SuperNodeManager) handleSuperNodeLeave() {
	// Check if there are other active SuperNodes
	activeSuperNodes := 0
	for _, sn := range sm.supernodes {
		if sn.IsSuperNode && time.Since(sn.LastActive) < 30*time.Second {
			activeSuperNodes++
		}
	}

	// If no other SuperNodes, need to elect a new one
	if activeSuperNodes <= 1 { // The leaving SuperNode is also counted
		sm.selectNewSuperNode()
	}
}

// selectNewSuperNode selects a new SuperNode
func (sm *SuperNodeManager) selectNewSuperNode() {
	// Select a new SuperNode from active regular nodes
	timeout := 30 * time.Second
	for i, sn := range sm.supernodes {
		if !sn.IsSuperNode &&
			time.Since(sn.LastActive) < timeout &&
			sn.ID != sm.localNodeInfo.Address {
			nodeIsNoSuperNode := sm.checkIfNodeIsNoSuperNode(sn.ID)
			if !nodeIsNoSuperNode {
				// Set as SuperNode
				sm.supernodes[i].IsSuperNode = true
				sm.supernodes[i].LastActive = time.Now()
				return
			}
		}
	}

	// If no suitable regular node and local node is not set as noSuperNode, then local node becomes SuperNode
	if !sm.noSuperNode && !sm.isSuperNode {
		sm.isSuperNode = true
	}
}

// ForwardMessageToSuperNodes forwards messages to SuperNodes
func (sm *SuperNodeManager) ForwardMessageToSuperNodes(message Message, messageKey []byte) error {
	superNodes := sm.GetSuperNodes()

	for _, superNode := range superNodes {
		if superNode.Address == sm.localNodeInfo.Address {
			continue // Don't send to self
		}

		go func(nodeAddr string) {
			conn, err := net.DialTimeout("tcp", nodeAddr, 5*time.Second)
			if err != nil {
				fmt.Printf("Failed to connect to SuperNode %s: %v\n", nodeAddr, err)
				return
			}
			defer conn.Close()

			// Serialize message
			messageData, err := json.Marshal(message)
			if err != nil {
				fmt.Printf("Failed to serialize message: %v\n", err)
				return
			}

			// Encrypt message
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

// GetBestSuperNodeForConnection gets the best SuperNode for connection
func (sm *SuperNodeManager) GetBestSuperNodeForConnection() *SuperNodeInfo {
	superNodes := sm.GetSuperNodes()
	if len(superNodes) == 0 {
		return nil
	}

	// Return the first SuperNode (can implement more complex load balancing algorithm)
	return &superNodes[0]
}
