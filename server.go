package main

import (
	"bufio"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type ClientID string

func NewClientID() (ClientID, error) {
	str, err := generateRandomString(256)
	if err != nil {
		return "", fmt.Errorf("error generating client ID: %v", err)
	}
	return ClientID(str), nil
}

type Checksum string

func CalculateChecksum(text string) Checksum {
	checksum := getSHA512Hash(text)
	return Checksum(checksum)
}

func (c Checksum) String() string {
	return string(c)
}

type Server struct {
	addr         string
	directory    string
	clients      map[ClientID]net.Conn
	acks         map[Checksum]map[ClientID]bool
	clientsMutex *sync.Mutex
	acksMutex    *sync.Mutex
}

func NewServer(addr, directory string) Server {
	if _, err := os.Stat(directory); os.IsNotExist(err) {
		err := os.MkdirAll(directory, 0755)
		if err != nil {
			fmt.Println("[server] error creating directory:", err)
			return Server{}
		}
	}
	return Server{
		addr:         addr,
		directory:    directory,
		clients:      make(map[ClientID]net.Conn),
		acks:         make(map[Checksum]map[ClientID]bool),
		clientsMutex: &sync.Mutex{},
		acksMutex:    &sync.Mutex{},
	}
}

func (s Server) Start(listener net.Listener) {
	defer listener.Close()
	fmt.Println("[server] server started on", s.addr)
	version, err := ThisBinaryVersion()
	if err != nil {
		fmt.Println("[server] error getting server version:", err)
		return
	}
	fmt.Println("[server] version", version)
	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Println("[server] error accepting connection:", err)
			continue
		}
		go s.HandleConnection(conn)
	}
}

func (s Server) HandleConnection(conn net.Conn) {
	defer conn.Close()
	clientID, err := NewClientID()
	if err != nil {
		fmt.Println("[server] error generating client ID:", err)
		return
	}
	reader := bufio.NewReader(conn)
	s.clientsMutex.Lock()
	s.clients[clientID] = conn
	s.clientsMutex.Unlock()
	fmt.Println("[server] new client connected:", (clientID)[:10])
	defer func() {
		s.clientsMutex.Lock()
		delete(s.clients, clientID)
		s.clientsMutex.Unlock()
		for _, clientAcks := range s.acks {
			s.acksMutex.Lock()
			delete(clientAcks, clientID)
			s.acksMutex.Unlock()
		}
		fmt.Println("[server] client disconnected:", (clientID)[:10])
	}()

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimSpace(line)
		msg, err := ParseMessage(line)
		if err != nil {
			fmt.Println("[server] invalid message:", err)
			continue
		}
		if ok := msg.ValidateChecksum(); !ok {
			fmt.Println("[server] message checksum does not match")
			continue
		}
		switch msg.Type {
		case MessageTypeClientPushContent:
			s.HandlePush(msg)
		case MessageTypeClientRemoveFile:
			s.HandleRemoveFile(msg)
		case MessageTypeClientAck:
			err = s.HandleAck(msg, clientID)
			if err != nil {
				fmt.Println("[server] error processing ack:", err)
			}
		case MessageTypeClientRequestServerVersion:
			err = s.HandleRequestServerVersion(clientID)
			if err != nil {
				fmt.Println("[server] error handling server version request:", err)
			}
		case MessageTypeClientRequestServerBinary:
			err = s.HandleRequestServerBinary(clientID)
			if err != nil {
				fmt.Println("[server] error handling server binary request:", err)
			}
		default:
			fmt.Println("[server] unknown command:", msg.Type)
		}
	}
}

func (s Server) HandlePush(msg Message) {
	checksum := msg.Payload.Checksum()
	filePath := filepath.Join(s.directory, checksum.String())
	err := os.WriteFile(filePath, []byte(msg.Payload), 0644)
	if err != nil {
		fmt.Println("[server] error writing file:", err)
		return
	}
	s.acksMutex.Lock()
	s.acks[checksum] = make(map[ClientID]bool)
	s.acksMutex.Unlock()
	s.BroadcastUpdate(MessageTypeServerPushContent, msg.Payload)
}

func (s Server) HandleRemoveFile(msg Message) {
	checksum := msg.Payload.Checksum()
	s.acksMutex.Lock()
	s.acks[checksum] = make(map[ClientID]bool)
	s.acksMutex.Unlock()
	s.BroadcastUpdate(MessageTypeServerRemoveFile, msg.Payload)
}

func (s Server) HandleAck(ackMsg Message, clientID ClientID) error {
	s.acksMutex.Lock()
	defer s.acksMutex.Unlock()
	checksum := Checksum(ackMsg.Payload)
	if len(checksum) < 10 {
		return fmt.Errorf("checksum is too short: %s (len=%d)", checksum, len(checksum))
	}
	if clientAcks, exists := s.acks[checksum]; exists {
		clientAcks[clientID] = true
		if len(clientAcks) == len(s.clients) {
			delete(s.acks, checksum)
			filePath := filepath.Join(s.directory, checksum.String())
			err := os.Remove(filePath)
			if err != nil {
				if !errors.Is(err, os.ErrNotExist) {
					return fmt.Errorf("error removing file: %s", err)
				}
			}
			fmt.Println("[server] all clients acknowledged update for", checksum[:10])
		}
		return nil
	} else {
		return fmt.Errorf("no client acknowledgments exist for %s", checksum[:10])
	}
}

func (s Server) BroadcastUpdate(msgType MessageType, payload MessagePayload) {
	msg, err := NewMessageFromPayload(msgType, payload)
	if err != nil {
		fmt.Println("[server] error creating broadcastmessage:", err)
		return
	}
	for _, client := range s.clients {
		msg.Send(client)
	}
}

func (s Server) HandleRequestServerVersion(clientID ClientID) error {
	version, err := ThisBinaryVersion()
	if err != nil {
		return fmt.Errorf("error getting server version: %s", err)
	}
	msg, err := NewMessageFromPayload(MessageTypeServerSendServerVersion, MessagePayload(version))
	if err != nil {
		return fmt.Errorf("error creating message: %s", err)
	}
	s.clientsMutex.Lock()
	conn, exists := s.clients[clientID]
	s.clientsMutex.Unlock()
	if !exists {
		return fmt.Errorf("client not found: %s", clientID)
	}
	_, err = msg.Send(conn)
	if err != nil {
		return fmt.Errorf("error sending message: %s", err)
	}
	return nil
}

func (s Server) HandleRequestServerBinary(clientID ClientID) error {
	binary, err := ThisBinaryAsBytes()
	if err != nil {
		return fmt.Errorf("error getting server binary: %s", err)
	}
	data := base64.StdEncoding.EncodeToString(binary)
	msg, err := NewMessageFromPayload(MessageTypeServerSendServerBinary, MessagePayload(data))
	if err != nil {
		return fmt.Errorf("error creating message: %s", err)
	}
	s.clientsMutex.Lock()
	conn, exists := s.clients[clientID]
	s.clientsMutex.Unlock()
	if !exists {
		return fmt.Errorf("client not found: %s", clientID)
	}
	_, err = msg.Send(conn)
	if err != nil {
		return fmt.Errorf("error sending message: %s", err)
	}
	return nil
}
