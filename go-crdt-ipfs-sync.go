// go-crdt-ipfs-sync.go

package main

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
	ipfslite "github.com/hsanjuan/ipfs-lite"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/sync"
	crdt "github.com/ipfs/go-ds-crdt"
	ipfs "github.com/ipfs/go-ipfs-api"
	logging "github.com/ipfs/go-log/v2"
	"github.com/ipfs/kubo/config"
	core "github.com/ipfs/kubo/core"
	"github.com/ipfs/kubo/core/coreapi"
	coreiface "github.com/ipfs/kubo/core/coreiface"
	"github.com/ipfs/kubo/plugin/loader"
	"github.com/ipfs/kubo/repo/fsrepo"
	repo "github.com/ipfs/kubo/repo/fsrepo"
	"github.com/libp2p/go-libp2p"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	ipfscrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/pnet"
)

const (
	TopDirectory  = "./tracked_dir"
	EncryptionKey = "your-32-byte-secret-key-which-is-32-bytes-long"
	SyncInterval  = 30 * time.Second
)

var (
	shell     = ipfs.NewShell("localhost:5001")
	crdtStore *crdt.Datastore
	memoryDs  *sync.MutexDatastore
)

func main() {
	// Load config.
	cfg, err := getConfig()
	if err != nil {
		fmt.Println("Error loading config:", err)
		return
	}

	// Ensure the top directory exists.
	if _, err := os.Stat(TopDirectory); os.IsNotExist(err) {
		err = os.Mkdir(TopDirectory, 0755)
		if err != nil {
			fmt.Println("Error creating tracked directory:", err)
			return
		}
	}

	_, err = startIPFSNode(cfg)
	if err != nil {
		fmt.Println("Error starting IPFS node:", err)
		return
	}
	fmt.Println("[+] IPFS node started.")

	err = initializeCRDTState(cfg)
	if err != nil {
		fmt.Println("Error initializing CRDT state:", err)
		return
	}
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		fmt.Println("Error creating watcher:", err)
		return
	}
	defer watcher.Close()

	go watchForLocalEdits(watcher)

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

func initializeCRDTState(cfg config.Config) error {
	ctx := context.Background()

	// Create an in-memory datastore
	memoryDs = sync.MutexWrap(datastore.NewMapDatastore())

	ps, host, err := initializeLibp2pPubSub()
	if err != nil {
		return fmt.Errorf("error initializing libp2p PubSub: %w", err)
	}
	defer host.Close()

	// Create the broadcaster
	broadcaster, err := crdt.NewPubSubBroadcaster(context.Background(), ps, "crdt-topic")
	if err != nil {
		return fmt.Errorf("error creating PubSub broadcaster: %w", err)
	}

	decoded, err := base64.StdEncoding.DecodeString(cfg.Identity.PrivKey)
	if err != nil {
		return fmt.Errorf("error decoding private key: %w", err)
	}
	privKey, err := ipfscrypto.UnmarshalPrivateKey([]byte(decoded))
	if err != nil {
		return fmt.Errorf("error unmarshaling private key: %w", err)
	}

	p2pHost, dht, err := ipfslite.SetupLibp2p(ctx, privKey, pnet.PSK(EncryptionKey), nil, nil, nil)
	if err != nil {
		return fmt.Errorf("error creating DHT: %w", err)
	}

	peer, err := ipfslite.New(ctx, memoryDs, nil, p2pHost, dht, nil)
	if err != nil {
		return fmt.Errorf("error creating IPFS Lite: %w", err)
	}

	// Create the CRDT datastore with the DAG service
	crdtStore, err = crdt.New(memoryDs, datastore.NewKey("crdt-sync"), peer.DAGService, broadcaster, &crdt.Options{
		MaxBatchDeltaSize:   10,
		NumWorkers:          1,
		Logger:              logging.Logger("crdt"),
		RebroadcastInterval: time.Minute,
	})
	if err != nil {
		return fmt.Errorf("error initializing CRDT datastore: %w", err)
	}

	// Walk through the tracked directory and add files to the CRDT store
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
		return fmt.Errorf("error initializing CRDT state: %w", err)
	}
	return nil
}

func watchForLocalEdits(watcher *fsnotify.Watcher) {
	fmt.Println("[+] File watcher started.")

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
		return (*pubsub.PubSub)(nil), nil, fmt.Errorf("failed to create libp2p host: %w", err)
	}

	// Create a new PubSub instance using GossipSub
	ps, err := pubsub.NewGossipSub(ctx, h)
	if err != nil {
		return (*pubsub.PubSub)(nil), nil, fmt.Errorf("failed to create PubSub: %w", err)
	}

	return ps, h, nil
}

func getConfig() (config.Config, error) {
	cfg := config.Config{}

	content, err := os.ReadFile("./ipfs-config-template.json")
	if err != nil {
		return config.Config{}, err
	}

	err = json.Unmarshal(content, &cfg)
	if err != nil {
		return config.Config{}, err
	}

	id, err := config.CreateIdentity(io.Discard, nil)
	if err != nil {
		return config.Config{}, fmt.Errorf("failed to create identity: %w", err)
	}
	cfg.Identity = id

	return cfg, nil
}

func startIPFSNode(cfg config.Config) (coreiface.CoreAPI, error) {
	ctx := context.Background()

	// Open the IPFS repository
	repoPath := filepath.Join(os.Getenv("HOME"), ".ipfs")
	if !repo.IsInitialized(repoPath) {
		plugins, err := loader.NewPluginLoader(repoPath)
		if err != nil {
			panic(fmt.Errorf("error loading plugins: %s", err))
		}

		if err := plugins.Initialize(); err != nil {
			panic(fmt.Errorf("error initializing plugins: %s", err))
		}

		if err := plugins.Inject(); err != nil {
			panic(fmt.Errorf("error initializing plugins: %s", err))
		}

		err = fsrepo.Init(repoPath, &cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize IPFS repo: %w", err)
		}
	}

	r, err := fsrepo.Open(repoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open IPFS repo: %w", err)
	}

	// Create and start the IPFS node
	node, err := core.NewNode(ctx, &core.BuildCfg{
		Repo:   r,
		Online: true,
	})

	// Create CoreAPI instance from the IPFS node
	api, err := coreapi.NewCoreAPI(node)
	if err != nil {
		return nil, fmt.Errorf("failed to create IPFS CoreAPI: %w", err)
	}

	return api, nil
}
