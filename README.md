# status

- Make it get the latest version of all content when starting up.
  - Apply each update as a patch to the local version.
  - Save the timestamp for the last sync.
  - Save a backup of the last merge. This will be used as the base for a three-way diff.
- Only delete an update from the server once all registered clients have successfully applied the update.
- Prevent making any updates until all file differences have been resolved. Apply this at the file level.
- Make it resolve any differences between the local copy and what in pulls from the other clients.
- Add admin commands:
  - Register new clients via client TLS public key. Keep these only on admin systems.
  - Add ability to rotate keys.
- Add mTLS.
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

## sample patching strategy

Use <https://github.com/sergi/go-diff>.

```go
package main

import (
  "fmt"

  "github.com/sergi/go-diff/diffmatchpatch"
)

func main() {
  dmp := diffmatchpatch.New()
  lastSync := "hello world"
  newSync := "say hello world"
  myEdit := "hello to the world"

  // If newSync is older, then it should be used to make the patch, which would be applied to
  // myEdit. Otherwise, myEdit should be used to make the patch, which would be applied to newSync.
  patches1 := dmp.PatchMake(lastSync, newSync)
  patched1, _ := dmp.PatchApply(patches1, myEdit)
  fmt.Println(patched1)
}
```
