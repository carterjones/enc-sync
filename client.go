package main

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
)

const LastSyncPath = ".last-sync"

type Client struct {
	dir       string
	conn      net.Conn
	secretKey string
}

func NewClient(dir string, secretKey string) (Client, error) {
	conn, err := net.Dial("tcp", serverAddr)
	if err != nil {
		return Client{}, fmt.Errorf("[client] error connecting to server: %v", err)
	}
	return Client{dir: dir, conn: conn, secretKey: secretKey}, nil
}

func (c Client) Start() {
	go c.ListenForUpdates()
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		fmt.Println("[client] error creating watcher:", err)
		return
	}
	defer watcher.Close()
	done := make(chan bool)
	go c.watchFileEvents(watcher, done)
	<-done
}

func (c Client) watchFileEvents(watcher *fsnotify.Watcher, done chan bool) {
	defer func() { done <- true }()
	mu := sync.Mutex{}
	timers := make(map[string]*time.Timer)
	err := watcher.Add(c.dir)
	if err != nil {
		fmt.Println("[client] error watching directory:", err)
		return
	}
	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			switch {
			case event.Has(fsnotify.Create):
				fallthrough
			case event.Has(fsnotify.Write):
				c.handleFileCreateOrWriteEvent(event, &mu, timers)
			case event.Has(fsnotify.Rename):
				// Renames are handled by two separate events: Remove and Create.
				fallthrough
			case event.Has(fsnotify.Remove):
				c.handleFileRemoveEvent(event)
			}
		case err := <-watcher.Errors:
			if err != nil {
				fmt.Println(err)
			}
		}
	}
}

func (c Client) handleFileCreateOrWriteEvent(event fsnotify.Event, mu *sync.Mutex, timers map[string]*time.Timer) {
	mu.Lock()
	timer, ok := timers[event.Name]
	mu.Unlock()
	if !ok {
		timer = time.AfterFunc(math.MaxInt64, func() {
			mu.Lock()
			delete(timers, event.Name)
			mu.Unlock()
			err := c.PushFile(event.Name)
			if err != nil {
				fmt.Println("[client] error pushing file to server:", err)
			}
		})
		timer.Stop()
		mu.Lock()
		timers[event.Name] = timer
		mu.Unlock()
	}
	waitFor := 100 * time.Millisecond
	timer.Reset(waitFor)
}

func (c Client) handleFileRemoveEvent(event fsnotify.Event) {
	err := c.RemoveFile(event.Name)
	if err != nil {
		fmt.Println("[client] error removing file:", err)
	}
}

func (c Client) Receive(reader *bufio.Reader) (Message, MessageData, error) {
	line, err := reader.ReadString('\n')
	if err != nil {
		return Message{}, MessageData{}, DisconnectError{}
	}
	line = strings.TrimSpace(line)
	msg, err := ParseMessage(line)
	if err != nil {
		return Message{}, MessageData{}, fmt.Errorf("invalid message: %s", err)
	}
	msgData := MessageData{}
	if msg.Type != MessageTypeServerSendServerVersion && msg.Type != MessageTypeServerSendServerBinary {
		msgData, err = msg.Decrypt(c.secretKey)
		if err != nil {
			return Message{}, MessageData{}, fmt.Errorf("error decrypting message: %s", err)
		}
	} else {
		msgData.Content = msg.Payload
	}
	return msg, msgData, nil
}

func (c *Client) ListenForUpdates() {
	for {
		reader := bufio.NewReader(c.conn)
		err := c.RequestServerVersion()
		if err != nil {
			fmt.Println("[client] error requesting server version:", err)
		}
		for {
			msg, msgData, err := c.Receive(reader)
			if err != nil {
				if _, ok := err.(DisconnectError); ok {
					c.conn = c.Reconnect()
					break
				}
				fmt.Println("[client] error receiving message:", err)
				continue
			}
			if ok := msg.ValidateChecksum(); !ok {
				fmt.Println("[client] message checksum does not match")
				continue
			}
			switch msg.Type {
			case MessageTypeServerPushContent:
				err = c.HandleUpdate(msg, msgData)
			case MessageTypeServerRemoveFile:
				err = c.HandleRemoveFile(msg, msgData)
			case MessageTypeServerSendServerVersion:
				err = c.HandleServerVersion(msgData.Content)
			case MessageTypeServerSendServerBinary:
				err = c.HandleServerBinary(msgData.Content)
			default:
				err = fmt.Errorf("unknown message type: %s", msg.Type)
			}
			if err != nil {
				fmt.Println("[client] error handling message:", err)
				continue
			}
			err = c.UpdateLastSync()
			if err != nil {
				fmt.Println("[client] failed to save last sync data:", err)
			}
		}
	}
}

func (c Client) Reconnect() net.Conn {
	for {
		conn, err := net.Dial("tcp", serverAddr)
		if err == nil {
			return conn
		}
		fmt.Println("[client] failed to reconnect, retrying in 1 second...")
		time.Sleep(1 * time.Second)
	}
}

func (c Client) SendAckMessage(checksum Checksum) error {
	msg := NewAckMessage(checksum)
	_, err := msg.Send(c.conn)
	if err != nil {
		return fmt.Errorf("error sending ack to server: %s", err)
	}
	return nil
}

func (c Client) PushFile(fullPath string) error {
	relPath, err := filepath.Rel(c.dir, fullPath)
	if err != nil {
		return fmt.Errorf("error determining relative path: %s", err)
	}
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return fmt.Errorf("[client] error reading file: %s", err)
	}
	msgData := MessageData{
		Path:      relPath,
		Content:   content,
		Timestamp: time.Now(),
	}
	msg, err := NewMessageFromDecrypted(MessageTypeClientPushContent, msgData, c.secretKey)
	if err != nil {
		return fmt.Errorf("error creating message: %s", err)
	}
	_, err = msg.Send(c.conn)
	if err != nil {
		return fmt.Errorf("error sending file to server: %s", err)
	}
	return nil
}

func (c Client) RemoveFile(fullPath string) error {
	relPath, err := filepath.Rel(c.dir, fullPath)
	if err != nil {
		return fmt.Errorf("error determining relative path: %s", err)
	}
	msgData := MessageData{
		Path:      relPath,
		Timestamp: time.Now(),
	}
	msg, err := NewMessageFromDecrypted(MessageTypeClientRemoveFile, msgData, c.secretKey)
	if err != nil {
		return fmt.Errorf("error creating message: %s", err)
	}
	_, err = msg.Send(c.conn)
	if err != nil {
		return fmt.Errorf("error sending file to server: %s", err)
	}
	return nil
}

func (c Client) HandleUpdate(msg Message, msgData MessageData) error {
	outpath := filepath.Join(c.dir, msgData.Path)
	existingContent, err := os.ReadFile(outpath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("error reading file: %s", err)
	}
	if !bytes.Equal(existingContent, msgData.Content) {
		err = os.WriteFile(outpath, []byte(msgData.Content), 0644)
		if err != nil {
			return fmt.Errorf("error writing file: %s", err)
		}
	}
	err = c.SendAckMessage(msg.Payload.Checksum())
	if err != nil {
		return fmt.Errorf("error acknowledging update: %s", err)
	}
	return nil
}

func (c Client) HandleRemoveFile(msg Message, msgData MessageData) error {
	outpath := filepath.Join(c.dir, msgData.Path)
	err := os.Remove(outpath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("error removing file: %s", err)
		}
	}
	err = c.SendAckMessage(msg.Payload.Checksum())
	if err != nil {
		return fmt.Errorf("error acknowledging update: %s", err)
	}
	return nil
}

func (c Client) RequestServerVersion() error {
	msg, err := NewMessageFromPayload(MessageTypeClientRequestServerVersion, nil)
	if err != nil {
		return fmt.Errorf("error creating message: %s", err)
	}
	_, err = msg.Send(c.conn)
	if err != nil {
		return fmt.Errorf("error sending message: %s", err)
	}
	return nil
}

func (c Client) HandleServerVersion(data []byte) error {
	serverVersion := string(data)
	clientVersion, err := ThisBinaryVersion()
	if err != nil {
		return fmt.Errorf("error getting client version: %s", err)
	}
	if serverVersion != clientVersion {
		fmt.Printf("[client] updating client to the latest version: %s\n", serverVersion[:10])
		return c.RequestServerBinary()
	}
	fmt.Println("[client] using the latest version:", clientVersion[:10])
	return nil
}

func (c Client) RequestServerBinary() error {
	msg, err := NewMessageFromPayload(MessageTypeClientRequestServerBinary, nil)
	if err != nil {
		return fmt.Errorf("error creating message: %s", err)
	}
	_, err = msg.Send(c.conn)
	if err != nil {
		return fmt.Errorf("error sending message: %s", err)
	}
	return nil
}

func (c Client) HandleServerBinary(data []byte) error {
	dir := ".tmpdir"
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("error creating temporary directory: %s", err)
	}
	defer os.RemoveAll(dir)
	tmpFile := filepath.Join(dir, "new-binary")
	out, err := os.OpenFile(tmpFile, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	defer out.Close()
	binPath, err := ThisBinaryPath()
	if err != nil {
		return fmt.Errorf("error getting binary path: %s", err)
	}
	decoded, err := base64.StdEncoding.DecodeString(string(data))
	if err != nil {
		return fmt.Errorf("error decoding binary: %s", err)
	}
	dataWriter := bufio.NewWriter(out)
	dataReader := bufio.NewReader(bytes.NewReader(decoded))
	if _, err := io.Copy(dataWriter, dataReader); err != nil {
		return err
	}
	err = dataWriter.Flush()
	if err != nil {
		return fmt.Errorf("error flushing data: %s", err)
	}
	if err := os.Rename(tmpFile, binPath); err != nil {
		return fmt.Errorf("error renaming file: %s", err)
	}
	return syscall.Exec(binPath, os.Args, os.Environ())
}

func (c Client) UpdateLastSync() error {
	err := os.WriteFile(LastSyncPath, []byte(time.Now().Format(time.RFC3339)), 0644)
	if err != nil {
		return fmt.Errorf("[client] error updating last sync time: %s", err)
	}
	return nil
}
