package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds the application configuration
type Config struct {
	TCPPort          int
	UDPPort          int
	BroadcastTimeout time.Duration
	DefaultNickname  string
	DefaultAdjectives []string
	DefaultNouns     []string
	MaxNodes         int
	FileChunkSize    int
}

// AppConfig holds the application-wide configuration instance
var AppConfig *Config

// LoadConfig loads configuration from the config file
func LoadConfig() *Config {
	config := &Config{
		// Set default values
		TCPPort:          8080,
		UDPPort:          8081,
		BroadcastTimeout: 5 * time.Second,
		DefaultAdjectives: []string{
			"Cool", "Smart", "Fast", "Lucky", "Brave", 
			"Clever", "Quick", "Sharp", "Bright", "Wise",
		},
		DefaultNouns: []string{
			"Tiger", "Eagle", "Wolf", "Fox", "Bear", 
			"Hawk", "Lion", "Shark", "Horse", "Owl",
		},
		MaxNodes:      100,
		FileChunkSize: 1024,
	}

	// Try to read config from file
	file, err := os.Open("config")
	if err != nil {
		fmt.Printf("Could not open config file, using defaults: %v\n", err)
		return config
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		switch key {
		case "TCPPORT":
			if port, err := strconv.Atoi(value); err == nil {
				config.TCPPort = port
			}
		case "UDPPORT":
			if port, err := strconv.Atoi(value); err == nil {
				config.UDPPort = port
			}
		case "BROADCAST_TIMEOUT":
			if dur, err := time.ParseDuration(value); err == nil {
				config.BroadcastTimeout = dur
			}
		case "DEFAULT_NICKNAME":
			config.DefaultNickname = value
		case "DEFAULT_ADJECTIVES":
			config.DefaultAdjectives = strings.Split(value, ",")
			for i := range config.DefaultAdjectives {
				config.DefaultAdjectives[i] = strings.TrimSpace(config.DefaultAdjectives[i])
			}
		case "DEFAULT_NOUNS":
			config.DefaultNouns = strings.Split(value, ",")
			for i := range config.DefaultNouns {
				config.DefaultNouns[i] = strings.TrimSpace(config.DefaultNouns[i])
			}
		case "MAX_NODES":
			if maxNodes, err := strconv.Atoi(value); err == nil {
				config.MaxNodes = maxNodes
			}
		case "FILE_CHUNK_SIZE":
			if chunkSize, err := strconv.Atoi(value); err == nil {
				config.FileChunkSize = chunkSize
			}
		}
	}

	return config
}

// Main entry point
func main() {
	AppConfig = LoadConfig()
	chat := NewP2PChat()
	chat.RunCLI()
}