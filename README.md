# status

It successfully runs using the following command:

```sh
rm -rf ~/.ipfs && make && ./dist/go-crdt-ipfs-sync
```

Current output is:

```text
Building go-crdt-ipfs-sync...
mkdir -p ./dist
go build -o ./dist/go-crdt-ipfs-sync go-crdt-ipfs-sync.go
[+] IPFS node started.
[+] File watcher started.
Syncing with IPFS...
Successfully synced with IPFS.
Syncing with IPFS...
Successfully synced with IPFS.
Syncing with IPFS...
```

## next steps

- Add a way to launch this in either a client or server mode, following a hub
  and spoke model.
- Get it syncing between two clients through the server.
- Run through a number of data integrity tests, such as:
  - C1: add data, C2: receive data
  - C1: delete data, C2: delete data
  - C1: add data, C1: add data, both end up matching as expected
  - C1: add data, C2: add data, C2: remove data added by C1, C1: data is removed
  - C1: add data, C2: receive data, shut down C1, start up C1: nothing happens
  - C1: add data, C2: receive data, shut down C2, start up C2: nothing happens
  - Shut down C2, C1: add data, start up C2, C2: receive data
  - Shut down C2, C1: delete data, start up C2, c2: deletes data
  - Shut down both C1 and C2, C1: add data, start up both C1 and C2, C1 and C2: data is added
  - Shut down both C1 and C2, C1: delete data, start up both C1 and C2, C1 and C2: data is removed
