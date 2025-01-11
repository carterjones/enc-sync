package main

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type MockConn struct {
	data []byte
}

func (m *MockConn) Read(b []byte) (n int, err error) {
	return 0, nil
}

func (m *MockConn) Write(b []byte) (n int, err error) {
	m.data = append(m.data, b...)
	return len(b), nil
}

func (m *MockConn) Close() error {
	return nil
}

func (m *MockConn) LocalAddr() net.Addr {
	return nil
}

func (m *MockConn) RemoteAddr() net.Addr {
	return nil
}

func (m *MockConn) SetDeadline(t time.Time) error {
	return nil
}

func (m *MockConn) SetReadDeadline(t time.Time) error {
	return nil
}

func (m *MockConn) SetWriteDeadline(t time.Time) error {
	return nil
}

func TestPushFile(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "testfile.txt")
	content := "Hello, World!"
	err := os.WriteFile(filePath, []byte(content), 0644)
	assert.NoError(t, err)

	mockConn := &MockConn{}
	client := Client{
		dir:       dir,
		conn:      mockConn,
		secretKey: "testkey",
	}

	err = client.PushFile(filePath)
	assert.ErrorContains(t, err, "crypto/aes: invalid key size")

	client.secretKey = "testkeytestkeytestkeytestkeytest"
	err = client.PushFile(filePath)
	assert.NoError(t, err)
	assert.NotEmpty(t, mockConn.data)
}

func TestSendAckMessage(t *testing.T) {
	mockConn := &MockConn{}
	client := Client{
		conn: mockConn,
	}

	checksum := Checksum("testchecksum")
	err := client.SendAckMessage(checksum)
	assert.NoError(t, err)

	sent := strings.TrimSpace(string(mockConn.data))
	parts := strings.Split(sent, " ")

	msgType, err := NewMessageType(parts[0])
	assert.NoError(t, err)
	assert.Equal(t, MessageTypeClientAck, msgType)

	payload := MessagePayload(parts[1])
	assert.Equal(t, MessagePayload(checksum), payload)

	checksumStr := Checksum(parts[2])
	assert.Equal(t, payload.Checksum(), checksumStr)

	client.conn = nil
	err = client.SendAckMessage(checksum)
	assert.ErrorContains(t, err, "connection is nil")
}
