# C4 - The Cinema Content Creation Cloud
[![GoDoc](https://godoc.org/github.com/etcenter/c4go?status.svg)](https://godoc.org/github.com/etcenter/c4)
[![Build Status](https://travis-ci.org/etcenter/c4.svg?branch=master)](https://travis-ci.org/etcenter/c4)
[![Coverage Status](https://coveralls.io/repos/github/etcenter/c4/badge.svg?branch=master)](https://coveralls.io/github/etcenter/c4?branch=master)

Go package and cli for c4.
### Command line tool
See [c4 command line tool](https://github.com/etcenter/c4/tree/master/cmd/c4)

### Go Package
Example usage to generate an c4 ID for a file.

```go
package main

import (
  "fmt"
  "io"
  "os"

  // import 'asset' asset identification
  "github.com/etcenter/c4/asset"
)

func main() {
  file := "main.go"
  f, err := os.Open(file)
  if err != nil {
    panic(err)
  }
  defer f.Close()

  // create a ID encoder.
  e := asset.NewIDEncoder()
  // the encoder is an io.Writer
  _, err = io.Copy(e, f)
  if err != nil {
    panic(err)
  }
  // ID will return an *asset.ID.
  // Be sure to be done writing bytes before calling ID()
  id := e.ID()
  // use the *asset.ID String method to get the c4id string
  fmt.Printf("C4id of \"%s\": %s\n", file, id.String())
  return
}

```

Output:

```bash
>go run main.go 
C4id of "main.go": c44jVTEz8y7wCiJcXvsX66BHhZEUdmtf7TNcZPy1jdM6S14qqrzsiLyoZRSvRGcAMLnKn4zVBvAFimNg14NFKp46cC
```

### Releases 

Current release: [v0.6.0](https://github.com/etcenter/c4/tree/v0.6.0)

Check the `release` branch for the latest release, or tags to find a specific release.  The `master` branch holds currently development.

### License
This software is released under the MIT license.  See LICENSE for more information.
