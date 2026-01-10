package main

import (
	"crypto/rand"
	"fmt"
	"net"
	"time"
)

// STUN constants
const (
	STUNBindingRequest  = 0x0001
	STUNBindingResponse = 0x0101
	STUNHeaderLength    = 20
	STUNMAGIC_COOKIE    = 0x2112A442
)

// Get public IP and port using STUN
func getPublicIPAndPort() (string, int, error) {
	// Try multiple STUN servers - ordered by reliability
	stunServers := []string{
		"stun.l.google.com:19302",      // Google STUN
		"stun1.l.google.com:19302",     // Google STUN
		"stun2.l.google.com:19302",     // Google STUN
		"stun3.l.google.com:19302",     // Google STUN
		"stun4.l.google.com:19302",     // Google STUN
		"stun.qq.com:19302",            // QQ STUN
		"stun.miwifi.com:19302",        // MiWiFi STUN
		"stun.msn.com:19302",           // MSN STUN
		"stun.hot-chilli.net:19302",    // Hot-chilli STUN
		"stun.ekiga.net:3478",          // Ekiga STUN
		"stun.ideasip.com:3478",        // Ideasip STUN
		"stun.rixtelecom.se:3478",      // Rixtelecom STUN
		"stun.schlund.de:3478",         // Schlund STUN
		"stun.stunprotocol.org:3478",   // STUN protocol STUN
		"stun.voiparound.com:3478",     // VoIP Around STUN
		"stun.voipbuster.com:3478",     // VoIP Buster STUN
		"stun.voipstunt.com:3478",      // VoIP Stunt STUN
		"stun.voxgratia.org:3478",      // Vox Gratia STUN
		"stun.xten.com:3478",           // XTen STUN
	}

	for _, server := range stunServers {
		fmt.Printf("Trying STUN server: %s\n", server)
		ip, port, err := sendSTUNRequest(server)
		if err == nil {
			fmt.Printf("Successfully connected to STUN server: %s\n", server)
			return ip, port, nil
		}
		fmt.Printf("STUN server %s request failed: %v\n", server, err)
	}

	// If all STUN servers fail, return local IP and default port
	localIP := getLocalIP()
	fmt.Println("All STUN servers failed, using local IP")
	return localIP, AppConfig.TCPPort, nil
}

// Send STUN request and parse response
func sendSTUNRequest(serverAddr string) (string, int, error) {
	// Resolve server address
	udpAddr, err := net.ResolveUDPAddr("udp", serverAddr)
	if err != nil {
		return "", 0, err
	}

	// Create UDP connection
	conn, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		return "", 0, err
	}
	defer conn.Close()

	// Set timeout
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))

	// Create STUN binding request
	header := STUNHeader{
		Type:   STUNBindingRequest,
		Length: 0,
		Cookie: STUNMAGIC_COOKIE,
	}
	
	// Generate random transaction ID
	_, err = rand.Read(header.TransactionID[:])
	if err != nil {
		return "", 0, err
	}

	// Serialize STUN message
	var msg []byte
	msg = append(msg, uint16ToBytes(header.Type)...)
	msg = append(msg, uint16ToBytes(header.Length)...)
	msg = append(msg, uint32ToBytes(header.Cookie)...)
	msg = append(msg, header.TransactionID[:]...)

	// Send STUN request
	_, err = conn.Write(msg)
	if err != nil {
		return "", 0, err
	}

	// Read response
	response := make([]byte, 1024)
	n, err := conn.Read(response)
	if err != nil {
		return "", 0, err
	}

	// Parse response
	return parseSTUNResponse(response[:n])
}

// STUN header
type STUNHeader struct {
	Type          uint16
	Length        uint16
	Cookie        uint32
	TransactionID [12]byte
}

// Parse STUN response
func parseSTUNResponse(data []byte) (string, int, error) {
	if len(data) < STUNHeaderLength {
		return "", 0, fmt.Errorf("response data too short")
	}

	// Parse STUN header
	msgType := bytesToUint16(data[0:2])
	if msgType != STUNBindingResponse {
		return "", 0, fmt.Errorf("invalid STUN response type")
	}

	// Get message length
	msgLength := int(bytesToUint16(data[2:4]))

	// Check data integrity
	if len(data) < STUNHeaderLength+msgLength {
		return "", 0, fmt.Errorf("response data incomplete")
	}

	// Parse attributes
	attrsStart := STUNHeaderLength
	attrsEnd := STUNHeaderLength + msgLength

	ip := ""
	port := 0

	for attrsStart < attrsEnd {
		if attrsStart+4 > attrsEnd {
			break
		}

		attrType := bytesToUint16(data[attrsStart : attrsStart+2])
		attrLength := int(bytesToUint16(data[attrsStart+2 : attrsStart+4]))
		attrsStart += 4

		if attrsStart+attrLength > attrsEnd {
			break
		}

		attrValue := data[attrsStart : attrsStart+attrLength]

		// Handle XOR_MAPPED_ADDRESS attribute (0x0020)
		if attrType == 0x0020 {
			if len(attrValue) >= 4 {
				addrFamily := attrValue[1]
				if addrFamily == 0x01 { // IPv4
					// XOR port
					xorPort := bytesToUint16(attrValue[2:4])
					port = int(xorPort ^ (STUNMAGIC_COOKIE >> 16))

					// XOR IP
					xorIP := attrValue[4:8]
					cookieBytes := uint32ToBytes(STUNMAGIC_COOKIE)
					ipBytes := make([]byte, 4)
					for i := 0; i < 4; i++ {
						ipBytes[i] = xorIP[i] ^ cookieBytes[i]
					}
					ip = net.IP(ipBytes).String()
				}
			}
		}

		// Align to 4-byte boundary
		attrsStart += attrLength
		if attrLength%4 != 0 {
			attrsStart += 4 - (attrLength % 4)
		}
	}

	if ip == "" {
		return "", 0, fmt.Errorf("mapped address not found in STUN response")
	}

	return ip, port, nil
}

// Convert uint16 to bytes
func uint16ToBytes(n uint16) []byte {
	return []byte{byte(n >> 8), byte(n)}
}

// Convert bytes to uint16
func bytesToUint16(b []byte) uint16 {
	return uint16(b[0])<<8 | uint16(b[1])
}

// Convert uint32 to bytes
func uint32ToBytes(n uint32) []byte {
	return []byte{byte(n >> 24), byte(n >> 16), byte(n >> 8), byte(n)}
}

// Convert bytes to uint32
func bytesToUint32(b []byte) uint32 {
	return uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
}
