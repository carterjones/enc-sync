// go-crdt-ipfs-sync.go

package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
	crdt "github.com/ipfs/go-ds-crdt"
	ipfs "github.com/ipfs/go-ipfs-api"
)

const (
	TopDirectory  = "./tracked_dir"
	EncryptionKey = "your-32-byte-secret-key-which-is-32-bytes-long"
	EncryptedBlob = "./encrypted_blob.aes"
	SyncInterval  = 30 * time.Second
)

var crdtState *crdt.DAGService
var shell = ipfs.NewShell("localhost:5001")

func main() {
	initializeCRDTState(TopDirectory)
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		fmt.Println("Error creating watcher:", err)
		return
	}
	defer watcher.Close()

	go watchForLocalEdits(watcher)

	err = watcher.Add(TopDirectory)
	if err != nil {
		fmt.Println("Error adding directory to watcher:", err)
		return
	}

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

func initializeCRDTState(directory string) {
	crdtState = crdt.NewDAGService(shell)
	err := filepath.Walk(directory, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			content, err := ioutil.ReadFile(path)
			if err != nil {
				return err
			}
			key := filepath.ToSlash(path)
			err = crdtState.Put(key, content)
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
				content, err := ioutil.ReadFile(filePath)
				if err != nil {
					fmt.Println("Error reading modified file:", err)
					continue
				}
				key := filepath.ToSlash(filePath)
				err = crdtState.Put(key, content)
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
	cid, err := crdtState.Commit()
	if err != nil {
		fmt.Println("Error committing CRDT state to IPFS:", err)
		return
	}

	fmt.Println("Successfully synced with IPFS. CID:", cid)
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
