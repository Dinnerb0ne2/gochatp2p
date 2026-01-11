package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"time"
)

// Get local IP
func getLocalIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		// Fallback to localhost if cannot determine
		return "127.0.0.1"
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String()
}

// UDP broadcast for node discovery
func (p *P2PChat) StartUDPBroadcast() error {
	udpAddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf(":%d", AppConfig.UDPPort))
	if err != nil {
		return err
	}

	socket, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return err
	}

	p.UDPSocket = socket
	p.Running = true

	// Start broadcast receiving goroutine
	go p.listenForBroadcasts()

	// If room creator, start broadcasting own info
	if len(p.Room.Nodes) > 0 && p.Room.Nodes[0].Address == p.LocalNode.Address {
		go p.broadcastNodeInfo()
	}

	return nil
}

// Listen for UDP broadcasts
func (p *P2PChat) listenForBroadcasts() {
	buffer := make([]byte, 1024)

	for p.Running {
		n, addr, err := p.UDPSocket.ReadFromUDP(buffer)
		if err != nil {
			if p.Running {
				fmt.Printf("Error reading UDP broadcast: %v\n", err)
			}
			continue
		}

		// Parse received data
		var nodeInfo NodeInfo
		if err := json.Unmarshal(buffer[:n], &nodeInfo); err != nil {
			continue
		}

		// Ensure NoSuperNode field has a default value if not present
		if nodeInfo.ID == "" {
			nodeInfo.NoSuperNode = false
		}

		// Update node address if not provided explicitly
		if nodeInfo.Address == "" {
			nodeInfo.Address = addr.String()
		}

		// Check if node is in room
		isRoomNode := false
		for _, node := range p.Room.Nodes {
			if node.ID == nodeInfo.ID {
				isRoomNode = true
				break
			}
		}

		// Add to room if not already present and not self
		if !isRoomNode && nodeInfo.ID != p.LocalNode.Address {
			p.NodeMutex.Lock()
			// Check if node limit is reached
			if len(p.Room.Nodes) >= AppConfig.MaxNodes {
				fmt.Printf("[System] Node limit (%d) reached, ignoring new node %s (%s)\n",
					AppConfig.MaxNodes, nodeInfo.Nickname, nodeInfo.Address)
				p.NodeMutex.Unlock()
				continue
			}
			p.Room.Nodes = append(p.Room.Nodes, nodeInfo)
			p.NodeMutex.Unlock()

			// Add node to SuperNode manager
			p.SuperNodeMgr.AddNode(nodeInfo)

			// If this is the room creator and we don't have a SuperNode yet, select one
			if len(p.Room.Nodes) == 2 && p.SuperNodeMgr.GetSuperNodes() == nil { // Local node + 1 other node
				// For the first few nodes, select one as SuperNode (not the local node if it has NoSuperNode enabled)
				if len(p.Room.Nodes) <= 5 {
					// Select initial SuperNode from first 5 nodes
					selectedSuperNodeID := p.SuperNodeMgr.SelectInitialSuperNode()
					if selectedSuperNodeID != "" {
						fmt.Printf("[System] Selected %s as initial SuperNode\n", selectedSuperNodeID)
					}
				}
			}

			fmt.Printf("[System] Node %s (%s) joined the room\n", nodeInfo.Nickname, nodeInfo.Address)
		}
	}
}

// Broadcast node info
func (p *P2PChat) broadcastNodeInfo() {
	ticker := time.NewTicker(AppConfig.BroadcastTimeout)
	defer ticker.Stop()

	nodeInfo := NodeInfo{
		ID:       p.LocalNode.Address,
		Address:  p.LocalNode.Address,
		Nickname: p.LocalNode.Nickname,
	}

	for range ticker.C {
		if !p.Running {
			break
		}

		data, err := json.Marshal(nodeInfo)
		if err != nil {
			continue
		}

		// Broadcast to local network
		broadcastAddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("255.255.255.255:%d", AppConfig.UDPPort))
		if err != nil {
			continue
		}

		// Set socket broadcast permission
		p.UDPSocket.SetWriteDeadline(time.Now().Add(time.Second))
		_, err = p.UDPSocket.WriteToUDP(data, broadcastAddr)
		if err != nil {
			// May be Windows doesn't allow broadcast, try other approaches
		}
	}
}

// Start TCP listener
func (p *P2PChat) StartTCPListener() error {
	tcpAddr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf(":%d", AppConfig.TCPPort))
	if err != nil {
		return err
	}

	listener, err := net.ListenTCP("tcp", tcpAddr)
	if err != nil {
		return err
	}

	fmt.Printf("TCP listener started on port %d\n", AppConfig.TCPPort)

	// Start accepting connections goroutine
	go func() {
		defer listener.Close()

		for p.Running {
			conn, err := listener.AcceptTCP()
			if err != nil {
				if p.Running {
					fmt.Printf("Error accepting TCP connection: %v\n", err)
				}
				continue
			}

			// Handle received message
			go p.handleTCPConnection(conn)
		}
	}()

	return nil
}

// Handle TCP connection
func (p *P2PChat) handleTCPConnection(conn *net.TCPConn) {
	defer conn.Close()

	// Get the remote address to identify sender
	remoteAddr := conn.RemoteAddr().String()

	buffer := make([]byte, 4096)
	for p.Running {
		n, err := conn.Read(buffer)
		if err != nil {
			if err != io.EOF {
				fmt.Printf("Error reading TCP connection from %s: %v\n", remoteAddr, err)
			}
			break
		}

		// Decrypt message
		decryptedData, err := decryptAES(p.MessageKey, buffer[:n])
		if err != nil {
			fmt.Printf("Failed to decrypt message from %s: %v\n", remoteAddr, err)
			continue
		}

		// Parse message
		var message Message
		if err := json.Unmarshal(decryptedData, &message); err != nil {
			fmt.Printf("Invalid message format from %s: %v\n", remoteAddr, err)
			continue
		}

		// Check if message is for current room
		if message.RoomID != p.Room.ID {
			continue
		}



		// In SuperNode mode, if this is a SuperNode, forward to other nodes
		if p.SuperNodeMgr.ShouldEnableSuperNodeMode(len(p.Room.Nodes)) {
			if p.SuperNodeMgr.IsLocalNodeSuperNode() {
				// This is a SuperNode, forward the message to appropriate nodes
				p.NodeMutex.RLock()

				// Forward to all regular nodes
				for _, node := range p.Room.Nodes {
					if node.Address == p.LocalNode.Address || node.Address == remoteAddr {
						continue // Don't send back to sender or to self
					}

					// Check if it's a regular node (not another SuperNode)
					superNodeInfo := p.SuperNodeMgr.GetNode(node.Address)
					if superNodeInfo != nil && superNodeInfo.IsSuperNode {
						continue // Skip other SuperNodes to avoid loops
					}

					// Send to regular node
					go func(nodeAddr string) {
						forwardConn, err := net.DialTimeout("tcp", nodeAddr, 5*time.Second)
						if err != nil {
							fmt.Printf("Failed to connect to node %s for message forwarding: %v\n", nodeAddr, err)
							return
						}
						defer forwardConn.Close()

						// Re-encrypt and send the message
						forwardData, err := json.Marshal(message)
						if err != nil {
							fmt.Printf("Failed to re-serialize message: %v\n", err)
							return
						}

						encryptedForwardData, err := encryptAES(p.MessageKey, forwardData)
						if err != nil {
							fmt.Printf("Failed to re-encrypt message: %v\n", err)
							return
						}

						_, err = forwardConn.Write(encryptedForwardData)
						if err != nil {
							fmt.Printf("Failed to forward message to node %s: %v\n", nodeAddr, err)
						}
					}(node.Address)
				}

				// Forward to other SuperNodes too (for redundancy)
				otherSuperNodes := p.SuperNodeMgr.GetSuperNodes()
				for _, superNode := range otherSuperNodes {
					if superNode.Address == p.LocalNode.Address || superNode.Address == remoteAddr {
						continue // Don't send to self or back to sender
					}

					go func(nodeAddr string) {
						forwardConn, err := net.DialTimeout("tcp", nodeAddr, 5*time.Second)
						if err != nil {
							fmt.Printf("Failed to connect to SuperNode %s for message forwarding: %v\n", nodeAddr, err)
							return
						}
						defer forwardConn.Close()

						// Re-encrypt and send the message
						forwardData, err := json.Marshal(message)
						if err != nil {
							fmt.Printf("Failed to re-serialize message: %v\n", err)
							return
						}

						encryptedForwardData, err := encryptAES(p.MessageKey, forwardData)
						if err != nil {
							fmt.Printf("Failed to re-encrypt message: %v\n", err)
							return
						}

						_, err = forwardConn.Write(encryptedForwardData)
						if err != nil {
							fmt.Printf("Failed to forward message to SuperNode %s: %v\n", nodeAddr, err)
						}
					}(superNode.Address)
				}

				p.NodeMutex.RUnlock()
			}
		}

		// Display message locally if it's not a duplicate
		fmt.Printf("[%s] %s: %s\n", message.Timestamp, message.Sender, message.Content)
	}
}
