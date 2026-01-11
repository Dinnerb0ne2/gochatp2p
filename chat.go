package main

import (
	"bufio"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net"
	"os"
	"strings"
	"sync"
	"time"
)

// Message structure
type Message struct {
	RoomID    string `json:"room_id"`
	Sender    string `json:"sender"`
	Timestamp string `json:"timestamp"`
	Content   string `json:"content"`
}

// Node info structure
type NodeInfo struct {
	ID          string `json:"id"`
	Address     string `json:"address"`
	Nickname    string `json:"nickname"`
	NoSuperNode bool   `json:"no_super_node,omitempty"` // 表示该节点不参与SuperNode选举
}

// Room info structure
type RoomInfo struct {
	ID       string     `json:"id"`
	Nodes    []NodeInfo `json:"nodes"`
	Password string     `json:"password"`
}

// P2P chat client
type P2PChat struct {
	LocalNode    NodeInfo
	Room         RoomInfo
	MessageKey   []byte
	UDPSocket    *net.UDPConn
	TCPListeners map[string]*net.TCPConn
	NodeMutex    sync.RWMutex
	Running      bool
	PublicIP     string
	PublicPort   int
	SuperNodeMgr *SuperNodeManager
}

// Create new P2P chat client
func NewP2PChat() *P2PChat {
	client := &P2PChat{
		TCPListeners: make(map[string]*net.TCPConn),
		Running:      false,
	}

	// Generate default nickname
	client.LocalNode.Nickname = generateRandomNickname()

	// Try to get public IP and port
	publicIP, publicPort, err := getPublicIPAndPort()
	if err != nil {
		fmt.Printf("Failed to get public IP, using local IP: %v\n", err)
		publicIP = getLocalIP()
		publicPort = AppConfig.TCPPort
	}

	client.PublicIP = publicIP
	client.PublicPort = publicPort
	client.LocalNode.Address = fmt.Sprintf("%s:%d", publicIP, publicPort)

	// Initialize SuperNode manager
	client.LocalNode.NoSuperNode = AppConfig.NoSuperNode
	client.SuperNodeMgr = NewSuperNodeManager(client.LocalNode, nil, AppConfig.TCPPort, AppConfig.UDPPort, AppConfig.NoSuperNode)

	return client
}

// Generate random nickname
func generateRandomNickname() string {
	// Use default nickname from config if specified
	if AppConfig.DefaultNickname != "" {
		return AppConfig.DefaultNickname
	}

	// Otherwise, generate random nickname from adjectives and nouns
	adj := AppConfig.DefaultAdjectives[time.Now().UnixNano()%int64(len(AppConfig.DefaultAdjectives))]
	noun := AppConfig.DefaultNouns[time.Now().UnixNano()%int64(len(AppConfig.DefaultNouns))]

	// Generate random number suffix
	num, _ := rand.Int(rand.Reader, big.NewInt(100))
	return fmt.Sprintf("%s%s%d", adj, noun, num)
}

// Create room
func (p *P2PChat) CreateRoom(roomID string) error {
	// Generate AES-128 key
	key := make([]byte, 16) // 128-bit key
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return err
	}

	p.MessageKey = key
	p.Room.ID = roomID
	p.Room.Password = base64.StdEncoding.EncodeToString(key)

	// Update SuperNode manager with the message key
	p.SuperNodeMgr = NewSuperNodeManager(p.LocalNode, p.MessageKey, AppConfig.TCPPort, AppConfig.UDPPort, AppConfig.NoSuperNode)

	// Add local node to room
	localNode := NodeInfo{
		ID:          p.LocalNode.Address,
		Address:     p.LocalNode.Address,
		Nickname:    p.LocalNode.Nickname,
		NoSuperNode: AppConfig.NoSuperNode,
	}
	p.Room.Nodes = append(p.Room.Nodes, localNode)

	fmt.Printf("Room created successfully! Room ID: %s\n", roomID)
	fmt.Printf("Room key: %s\n", p.Room.Password)
	fmt.Printf("Your nickname: %s\n", p.LocalNode.Nickname)

	return nil
}

// Join room
func (p *P2PChat) JoinRoom(roomID, password string) error {
	// Decode key
	key, err := base64.StdEncoding.DecodeString(password)
	if err != nil {
		return fmt.Errorf("invalid room key: %v", err)
	}

	if len(key) != 16 {
		return fmt.Errorf("room key length incorrect, should be 16 bytes")
	}

	p.Room.ID = roomID
	p.MessageKey = key
	p.Room.Password = password

	// Update SuperNode manager with the message key
	p.LocalNode.NoSuperNode = AppConfig.NoSuperNode
	p.SuperNodeMgr = NewSuperNodeManager(p.LocalNode, p.MessageKey, AppConfig.TCPPort, AppConfig.UDPPort, AppConfig.NoSuperNode)

	fmt.Printf("Successfully joined room %s!\n", roomID)
	fmt.Printf("Your nickname: %s\n", p.LocalNode.Nickname)

	return nil
}

// Send message to all nodes in room
func (p *P2PChat) SendMessage(content string) error {
	// Create message
	message := Message{
		RoomID:    p.Room.ID,
		Sender:    p.LocalNode.Nickname,
		Timestamp: time.Now().Format("2006-01-02 15:04:05"),
		Content:   content,
	}

	// Serialize message
	messageData, err := json.Marshal(message)
	if err != nil {
		return err
	}

	// Encrypt message
	encryptedData, err := encryptAES(p.MessageKey, messageData)
	if err != nil {
		return err
	}

	// Use SuperNode mode if enabled and there are enough nodes
	if p.SuperNodeMgr.ShouldEnableSuperNodeMode(len(p.Room.Nodes)) {
		// If this node is a SuperNode, send to other SuperNodes
		if p.SuperNodeMgr.IsLocalNodeSuperNode() {
			// Forward to other SuperNodes
			err = p.SuperNodeMgr.ForwardMessageToSuperNodes(message, p.MessageKey)
			if err != nil {
				return err
			}
			// Display local message
			fmt.Printf("[%s] Me: %s\n", message.Timestamp, message.Content)
		} else {
			// If not a SuperNode, send to my designated SuperNode
			superNode := p.SuperNodeMgr.GetBestSuperNodeForConnection()
			if superNode != nil {
				// Send to designated SuperNode
				go func(nodeAddr string) {
					conn, err := net.DialTimeout("tcp", nodeAddr, 5*time.Second)
					if err != nil {
						fmt.Printf("Failed to connect to SuperNode %s: %v\n", nodeAddr, err)
						return
					}
					defer conn.Close()

					_, err = conn.Write(encryptedData)
					if err != nil {
						fmt.Printf("Failed to send message to SuperNode %s: %v\n", nodeAddr, err)
					} else {
						// Display local message after successful send to SuperNode
						fmt.Printf("[%s] Me: %s\n", message.Timestamp, message.Content)
					}
				}(superNode.Address)
			} else {
				// Fallback: send directly to all nodes if no SuperNode available
				p.NodeMutex.RLock()
				defer p.NodeMutex.RUnlock()

				// Track if local message has been displayed
				localDisplayed := false
				for _, node := range p.Room.Nodes {
					if node.Address == p.LocalNode.Address {
						// Display local message
						fmt.Printf("[%s] Me: %s\n", message.Timestamp, message.Content)
						localDisplayed = true
						continue
					}

					// Connect to other nodes and send message
					go func(nodeAddr string) {
						conn, err := net.DialTimeout("tcp", nodeAddr, 5*time.Second)
						if err != nil {
							fmt.Printf("Failed to connect to node %s: %v\n", nodeAddr, err)
							return
						}
						defer conn.Close()

						_, err = conn.Write(encryptedData)
						if err != nil {
							fmt.Printf("Failed to send message to node %s: %v\n", nodeAddr, err)
						}
					}(node.Address)
				}
				if !localDisplayed {
					fmt.Printf("[%s] Me: %s\n", message.Timestamp, message.Content)
				}
			}
		}
	} else {
		// Standard mode: send to all nodes directly
		p.NodeMutex.RLock()
		defer p.NodeMutex.RUnlock()

		// Track if local message has been displayed
		localDisplayed := false
		for _, node := range p.Room.Nodes {
			if node.Address == p.LocalNode.Address {
				// Display local message
				fmt.Printf("[%s] Me: %s\n", message.Timestamp, message.Content)
				localDisplayed = true
				continue
			}

			// Connect to other nodes and send message
			go func(nodeAddr string) {
				conn, err := net.DialTimeout("tcp", nodeAddr, 5*time.Second)
				if err != nil {
					fmt.Printf("Failed to connect to node %s: %v\n", nodeAddr, err)
					return
				}
				defer conn.Close()

				_, err = conn.Write(encryptedData)
				if err != nil {
					fmt.Printf("Failed to send message to node %s: %v\n", nodeAddr, err)
				}
			}(node.Address)
		}
		if !localDisplayed {
			fmt.Printf("[%s] Me: %s\n", message.Timestamp, message.Content)
		}
	}

	return nil
}

// Run CLI interface
func (p *P2PChat) RunCLI() {
	fmt.Println("P2P chat program started!")
	fmt.Println("Available commands:")
	fmt.Println("  /create [room ID] - Create room")
	fmt.Println("  /join [room ID] [room key] - Join room")
	fmt.Println("  /list - List nodes in room")
	fmt.Println("  /save - Save chat log")
	fmt.Println("  /file [file path] - Send file")
	fmt.Println("  /help - Show this help message")
	fmt.Println("  /exit - Exit program")
	fmt.Println("  (Messages without / are sent as chat messages)")

	scanner := bufio.NewScanner(os.Stdin)

	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			break
		}

		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}

		// Check if input is a command (starts with /)
		if strings.HasPrefix(input, "/") {
			// Process as command
			parts := strings.Fields(input)
			command := strings.ToLower(parts[0])

			// Remove the leading slash to get the actual command
			command = command[1:]

			switch command {
			case "create":
				if len(parts) < 2 {
					fmt.Println("Usage: /create [room ID]")
					continue
				}

				if p.Room.ID != "" {
					fmt.Println("You are already in a room!")
					continue
				}

				roomID := parts[1]
				if err := p.CreateRoom(roomID); err != nil {
					fmt.Printf("Failed to create room: %v\n", err)
					continue
				}

				// Start UDP and TCP services
				if err := p.StartUDPBroadcast(); err != nil {
					fmt.Printf("Failed to start UDP broadcast: %v\n", err)
					continue
				}

				if err := p.StartTCPListener(); err != nil {
					fmt.Printf("Failed to start TCP listener: %v\n", err)
					continue
				}

				fmt.Printf("Room %s created, listening for connections...\n", roomID)

			case "join":
				if len(parts) < 3 {
					fmt.Println("Usage: /join [room ID] [room key]")
					continue
				}

				if p.Room.ID != "" {
					fmt.Println("You are already in a room!")
					continue
				}

				roomID := parts[1]
				password := parts[2]

				if err := p.JoinRoom(roomID, password); err != nil {
					fmt.Printf("Failed to join room: %v\n", err)
					continue
				}

				// Start UDP and TCP services
				if err := p.StartUDPBroadcast(); err != nil {
					fmt.Printf("Failed to start UDP broadcast: %v\n", err)
					continue
				}

				if err := p.StartTCPListener(); err != nil {
					fmt.Printf("Failed to start TCP listener: %v\n", err)
					continue
				}

				fmt.Printf("Successfully joined room %s, listening for connections...\n", roomID)

			case "list":
				if p.Room.ID == "" {
					fmt.Println("Please create or join a room first!")
					continue
				}

				p.NodeMutex.RLock()
				fmt.Printf("Nodes in room %s (%d nodes):\n", p.Room.ID, len(p.Room.Nodes))
				for i, node := range p.Room.Nodes {
					status := ""
					if node.Address == p.LocalNode.Address {
						status = " (you)"
					}
					fmt.Printf("  %d. %s (%s)%s\n", i+1, node.Nickname, node.Address, status)
				}
				p.NodeMutex.RUnlock()

			case "save":
				// Save chat log (simplified implementation)
				timestamp := time.Now().Format("20060102_150405")
				filename := fmt.Sprintf("chat_log_%s.txt", timestamp)
				content := fmt.Sprintf("P2P Chat Log - %s\n", time.Now().Format("2006-01-02 15:04:05"))
				err := os.WriteFile(filename, []byte(content), 0644)
				if err != nil {
					fmt.Printf("Failed to save chat log: %v\n", err)
				} else {
					fmt.Printf("Chat log saved to %s\n", filename)
				}

			case "file":
				if len(parts) < 2 {
					fmt.Println("Usage: /file [file path]")
					continue
				}

				filePath := parts[1]
				fileInfo, err := os.Stat(filePath)
				if err != nil {
					fmt.Printf("File does not exist: %s\n", filePath)
					continue
				}

				if fileInfo.IsDir() {
					fmt.Printf("Cannot send directory: %s\n", filePath)
					continue
				}

				// Simplified file sending (actual implementation should handle file chunks)
				fmt.Printf("File sending: Preparing to send file %s (size: %d bytes)\n", filePath, fileInfo.Size())

				// Read file content and encode to base64
				fileData, err := os.ReadFile(filePath)
				if err != nil {
					fmt.Printf("Failed to read file: %v\n", err)
					continue
				}

				encodedData := base64.StdEncoding.EncodeToString(fileData)
				chunkSize := 100
				if len(encodedData) < chunkSize {
					chunkSize = len(encodedData)
				}
				fileMessage := fmt.Sprintf("[File] %s: %s", fileInfo.Name(), encodedData[:chunkSize])
				if len(encodedData) > chunkSize {
					fileMessage += "..."
				}

				if err := p.SendMessage(fileMessage); err != nil {
					fmt.Printf("Failed to send file info: %v\n", err)
				} else {
					fmt.Printf("File info sent\n")
				}

			case "help":
				fmt.Println("Available commands:")
				fmt.Println("  /create [room ID] - Create room")
				fmt.Println("  /join [room ID] [room key] - Join room")
				fmt.Println("  /list - List nodes in room")
				fmt.Println("  /save - Save chat log")
				fmt.Println("  /file [file path] - Send file")
				fmt.Println("  /help - Show this help message")
				fmt.Println("  /exit - Exit program")
				fmt.Println("  (Messages without / are sent as chat messages)")

			case "exit":
				fmt.Println("Exiting program...")
				p.Running = false
				if p.UDPSocket != nil {
					p.UDPSocket.Close()
				}
				// Close all TCP connections
				p.NodeMutex.Lock()
				for addr, conn := range p.TCPListeners {
					conn.Close()
					delete(p.TCPListeners, addr)
				}
				p.NodeMutex.Unlock()
				os.Exit(0)

			default:
				fmt.Printf("Unknown command: %s\n", command)
				fmt.Println("Type '/help' for available commands")
			}
		} else {
			// Process as chat message
			if p.Room.ID == "" {
				fmt.Println("Please create or join a room first!")
				continue
			}

			// Send the entire input as a message
			if err := p.SendMessage(input); err != nil {
				fmt.Printf("Failed to send message: %v\n", err)
				continue
			}
		}
	}
}
