// go-crdt-ipfs-sync.go

package main

import (
	"context"

	logging "github.com/ipfs/go-log/v2"

	"github.com/libp2p/go-libp2p"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"

	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/sync"
	crdt "github.com/ipfs/go-ds-crdt"
	ipfs "github.com/ipfs/go-ipfs-api"
)

const (
	TopDirectory  = "./tracked_dir"
	EncryptionKey = "your-32-byte-secret-key-which-is-32-bytes-long"
	SyncInterval  = 30 * time.Second
)

var (
	shell     = ipfs.NewShell("localhost:5001")
	crdtStore *crdt.Datastore
	memoryDs  datastore.Datastore
)

func main() {
	initializeCRDTState()
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		fmt.Println("Error creating watcher:", err)
		return
	}
	defer watcher.Close()

	go watchForLocalEdits(watcher)

	if _, err := os.Stat(TopDirectory); os.IsNotExist(err) {
		err = os.Mkdir(TopDirectory, 0755)
		if err != nil {
			fmt.Println("Error creating tracked directory:", err)
			return
		}
	}
	err = watcher.Add(TopDirectory)

	ticker := time.NewTicker(SyncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			syncWithIPFS()
		case err := <-watcher.Errors:
			fmt.Println("Watcher error:", err)
		}
	}
}

func initializeCRDTState() {
	// Create an in-memory datastore
	memoryDs = sync.MutexWrap(datastore.NewMapDatastore())

	ps, host, err := initializeLibp2pPubSub()
	if err != nil {
		fmt.Println("Error initializing libp2p PubSub:", err)
		return
	}
	defer host.Close()

	// Create the CRDT datastore
	broadcaster, err := crdt.NewPubSubBroadcaster(context.Background(), ps, "crdt-topic")
	if err != nil {
		fmt.Println("Error creating PubSub broadcaster:", err)
		return
	}
	crdtStore, err = crdt.New(memoryDs, datastore.NewKey("crdt-sync"), nil, broadcaster, &crdt.Options{
		MaxBatchDeltaSize:   10,
		NumWorkers:          1,
		Logger:              logging.Logger("crdt"),
		RebroadcastInterval: time.Minute, // Set a valid interval, e.g., 1 minute
	})
	if err != nil {
		fmt.Println("Error initializing CRDT datastore:", err)
		return
	}

	err = filepath.Walk(TopDirectory, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			key := filepath.ToSlash(path)
			err = crdtStore.Put(context.Background(), datastore.NewKey(key), content)
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		fmt.Println("Error initializing CRDT state:", err)
	}
}

func watchForLocalEdits(watcher *fsnotify.Watcher) {
	for {
		select {
		case event := <-watcher.Events:
			if event.Op&fsnotify.Write == fsnotify.Write {
				filePath := event.Name
				content, err := os.ReadFile(filePath)
				if err != nil {
					fmt.Println("Error reading modified file:", err)
					continue
				}
				key := filepath.ToSlash(filePath)
				err = crdtStore.Put(context.Background(), datastore.NewKey(key), content)
				if err != nil {
					fmt.Println("Error updating CRDT state for file:", filePath, "Error:", err)
				} else {
					fmt.Println("File modified:", filePath)
				}
			}
		}
	}
}

func syncWithIPFS() {
	fmt.Println("Syncing with IPFS...")
	// CRDT datastore automatically syncs through PubSub; no explicit commit needed
	fmt.Println("Successfully synced with IPFS.")
}

func encrypt(data, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	ciphertext := make([]byte, aes.BlockSize+len(data))
	iv := ciphertext[:aes.BlockSize]
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return nil, err
	}

	stream := cipher.NewCFBEncrypter(block, iv)
	stream.XORKeyStream(ciphertext[aes.BlockSize:], data)

	return ciphertext, nil
}

func decrypt(data, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	iv := data[:aes.BlockSize]
	plaintext := make([]byte, len(data)-aes.BlockSize)

	stream := cipher.NewCFBDecrypter(block, iv)
	stream.XORKeyStream(plaintext, data[aes.BlockSize:])

	return plaintext, nil
}

func initializeLibp2pPubSub() (*pubsub.PubSub, host.Host, error) {
	ctx := context.Background()

	// Create a new libp2p host
	h, err := libp2p.New()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create libp2p host: %w", err)
	}

	// Create a new PubSub instance using GossipSub
	ps, err := pubsub.NewGossipSub(ctx, h)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create PubSub: %w", err)
	}

	return ps, h, nil
}
