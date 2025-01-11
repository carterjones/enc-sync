package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strings"
)

const (
	serverAddr = ":9000"
)

// LoadSettings reads a file and returns a map of settings.
func LoadSettings(filename string) (map[string]string, error) {
	settings := make(map[string]string)

	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Ignore empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid line: %s", line)
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		settings[key] = value
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return settings, nil
}

func main() {
	name := os.Args[0]

	if len(os.Args) < 2 {
		fmt.Printf("Usage: %s [server|client] [directory]\n", name)
		return
	}

	filename := ".settings" // Replace with your filename

	settings, err := LoadSettings(filename)
	if err != nil {
		fmt.Println("error loading settings:", err)
		return
	}

	mode := os.Args[1]
	if mode == "server" {
		if len(os.Args) < 3 {
			fmt.Printf("Usage %s run main.go server [directory]\n", name)
			return
		}
		directory := os.Args[2]
		server := NewServer(serverAddr, directory)
		listener, err := net.Listen("tcp", serverAddr)
		if err != nil {
			fmt.Println("error starting server:", err)
			return
		}
		server.Start(listener)
	} else if mode == "client" {
		if len(os.Args) < 3 {
			fmt.Printf("Usage %s run main.go client [directory]\n", name)
			return
		}
		dir := os.Args[2]

		secretKey, ok := settings["secretKey"]
		if !ok {
			fmt.Println("secretKey not found in settings")
			return
		}
		client, err := NewClient(dir, secretKey)
		if err != nil {
			fmt.Println(err)
			return
		}
		client.Start()
	} else {
		fmt.Println("Invalid mode. Use 'server' or 'client'")
	}
}
