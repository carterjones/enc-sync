package main

import (
	"encoding/base64"
	"fmt"
	"net"
	"strings"
	"time"
)

type MessageData struct {
	Path      string
	Timestamp time.Time
	Content   []byte
}

func (m MessageData) String() string {
	encodedPath := base64.StdEncoding.EncodeToString([]byte(m.Path))
	encodedContent := base64.StdEncoding.EncodeToString([]byte(m.Content))
	return fmt.Sprintf("%s %s %s", encodedPath, encodedContent, m.Timestamp.Format(time.RFC3339))
}

func (m MessageData) Encrypt(secretKey string) (MessagePayload, error) {
	encrypted, err := encryptAES(m.String(), secretKey)
	if err != nil {
		return nil, fmt.Errorf("error encrypting message data: %v", err)
	}
	return MessagePayload(encrypted), nil
}

func DecryptMessageData(payload MessagePayload, key string) (MessageData, error) {
	decrypted, err := decryptAES(string(payload), key)
	if err != nil {
		return MessageData{}, fmt.Errorf("error decrypting message data: %v", err)
	}
	return ParseMessageData(string(decrypted))
}

func ParseMessageData(line string) (MessageData, error) {
	parts := strings.SplitN(line, " ", 3)
	if len(parts) < 3 {
		return MessageData{}, fmt.Errorf("invalid message data format")
	}
	decodedPath, err := base64.StdEncoding.DecodeString(parts[0])
	if err != nil {
		return MessageData{}, fmt.Errorf("invalid base64-encoded path")
	}
	decodedContent, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return MessageData{}, fmt.Errorf("invalid base64-encoded content")
	}
	timestamp, err := time.Parse(time.RFC3339, parts[2])
	if err != nil {
		fmt.Println(line)
		return MessageData{}, fmt.Errorf("invalid timestamp format: %s", parts[2])
	}
	data := MessageData{
		Path:      string(decodedPath),
		Content:   decodedContent,
		Timestamp: timestamp,
	}
	return data, nil
}

type MessageType string

const (
	MessageTypeInvalid                    MessageType = "INVALID"
	MessageTypeClientAck                  MessageType = "CLIENT_ACK"
	MessageTypeClientPushContent          MessageType = "CLIENT_PUSH_CONTENT"
	MessageTypeServerPushContent          MessageType = "SERVER_PUSH_CONTENT"
	MessageTypeClientRemoveFile           MessageType = "CLIENT_REMOVE_FILE"
	MessageTypeServerRemoveFile           MessageType = "SERVER_REMOVE_FILE"
	MessageTypeClientRequestServerVersion MessageType = "CLIENT_REQUEST_SERVER_VERSION"
	MessageTypeClientRequestServerBinary  MessageType = "CLIENT_REQUEST_SERVER_BINARY"
	MessageTypeServerSendServerVersion    MessageType = "SERVER_SEND_SERVER_VERSION"
	MessageTypeServerSendServerBinary     MessageType = "SERVER_SEND_SERVER_BINARY"
)

func NewMessageType(s string) (MessageType, error) {
	switch s {
	case string(MessageTypeClientAck):
		return MessageTypeClientAck, nil
	case string(MessageTypeClientPushContent):
		return MessageTypeClientPushContent, nil
	case string(MessageTypeServerPushContent):
		return MessageTypeServerPushContent, nil
	case string(MessageTypeClientRemoveFile):
		return MessageTypeClientRemoveFile, nil
	case string(MessageTypeServerRemoveFile):
		return MessageTypeServerRemoveFile, nil
	case string(MessageTypeClientRequestServerVersion):
		return MessageTypeClientRequestServerVersion, nil
	case string(MessageTypeClientRequestServerBinary):
		return MessageTypeClientRequestServerBinary, nil
	case string(MessageTypeServerSendServerVersion):
		return MessageTypeServerSendServerVersion, nil
	case string(MessageTypeServerSendServerBinary):
		return MessageTypeServerSendServerBinary, nil
	default:
		return MessageTypeInvalid, fmt.Errorf("invalid message type: %s", s)
	}
}

type MessagePayload []byte

func (p MessagePayload) Checksum() Checksum {
	return CalculateChecksum(string(p))
}

type Message struct {
	Type            MessageType
	Payload         MessagePayload
	ClaimedChecksum Checksum
}

func NewMessageFromDecrypted(msgType MessageType, msgData MessageData, secretKey string) (Message, error) {
	if msgType == MessageTypeInvalid {
		return Message{}, fmt.Errorf("invalid message type")
	}
	encrypted, err := msgData.Encrypt(secretKey)
	if err != nil {
		return Message{}, fmt.Errorf("error encrypting message data: %v", err)
	}
	msg := Message{
		Type:    msgType,
		Payload: encrypted,
	}
	return msg, nil
}

func NewMessageFromPayload(msgType MessageType, payload MessagePayload) (Message, error) {
	if msgType == MessageTypeInvalid {
		return Message{}, fmt.Errorf("invalid message type")
	}
	msg := Message{
		Type:    msgType,
		Payload: payload,
	}
	return msg, nil
}

func NewAckMessage(checksum Checksum) Message {
	msg := Message{
		Type:    MessageTypeClientAck,
		Payload: MessagePayload(checksum),
	}
	return msg
}

func ParseMessage(line string) (Message, error) {
	parts := strings.SplitN(line, " ", 3)
	if len(parts) < 3 {
		return Message{}, fmt.Errorf("invalid message format: %s", line)
	}
	msgType, err := NewMessageType(parts[0])
	if err != nil {
		return Message{}, fmt.Errorf("invalid message type: %v", err)
	}
	msg := Message{
		Type:            msgType,
		Payload:         MessagePayload(parts[1]),
		ClaimedChecksum: Checksum(parts[2]),
	}
	return msg, nil
}

func (m Message) Send(conn net.Conn) (string, error) {
	if conn == nil {
		return "", fmt.Errorf("error: connection is nil")
	}
	m.ClaimedChecksum = m.Payload.Checksum()
	msg := fmt.Sprintf("%s %s %s\n", m.Type, m.Payload, m.ClaimedChecksum)
	nBytes, err := conn.Write([]byte(msg))
	if err != nil {
		return "", fmt.Errorf("error sending message: %v", err)
	}
	if nBytes != len(msg) {
		return "", fmt.Errorf("error: not all bytes sent")
	}
	return msg, nil
}

func (m Message) Decrypt(key string) (MessageData, error) {
	decrypted, err := DecryptMessageData(m.Payload, key)
	if err != nil {
		return MessageData{}, fmt.Errorf("error decrypting message data: %v", err)
	}
	return decrypted, nil
}

func (m Message) ValidateChecksum() bool {
	return m.ClaimedChecksum == m.Payload.Checksum()
}
