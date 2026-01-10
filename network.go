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
		n, _, err := p.UDPSocket.ReadFromUDP(buffer)  // Changed: ignore addr variable
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
			p.Room.Nodes = append(p.Room.Nodes, nodeInfo)
			p.NodeMutex.Unlock()
			
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
	
	buffer := make([]byte, 4096)
	for p.Running {
		n, err := conn.Read(buffer)
		if err != nil {
			if err != io.EOF {
				fmt.Printf("Error reading TCP connection: %v\n", err)
			}
			break
		}
		
		// Decrypt message
		decryptedData, err := decryptAES(p.MessageKey, buffer[:n])
		if err != nil {
			fmt.Printf("Failed to decrypt message: %v\n", err)
			continue
		}
		
		// Parse message
		var message Message
		if err := json.Unmarshal(decryptedData, &message); err != nil {
			fmt.Printf("Invalid message format: %v\n", err)
			continue
		}
		
		// Check if message is for current room
		if message.RoomID != p.Room.ID {
			continue
		}
		
		// Display message
		fmt.Printf("[%s] %s: %s\n", message.Timestamp, message.Sender, message.Content)
	}
}