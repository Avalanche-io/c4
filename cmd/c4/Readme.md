# c4 - Command line interface

This version of the c4 cli generates ids for individual files and folders, file system hierarchies, and piped data.

## Examples 

Id a file:

```bash
$ c4 c4.go
c41YXr8u3uZC5kkwUr27TZkoYRYDprZmr8YCBJ13quTggGvjGrxMJzzF9qcoFhyGr5rxP2dMtySJevJqQbC3R3hzyE
```

Id all files and folders recursively:

```bash
$ c4 -R ./cmd
c43XLLxd2sNhqjUPWL8xYqoq7gmU1SZA675m1zS7jFVPRKTx7hTANQEu6RaY286uqEpbqBZKpoPNfzNSNQEVthQ7H7:  cmd/c4/Readme.md
c44m2stxSN9Wz7sAoNgPttVQ8xFzzEW4xP3huPuTXbT93MpwYPK5AmDPR4wANjFWZHBE7kNsLm73hH1YopZawtRfRK:  cmd/c4/main.go
c45agVTfUYSMumK4ohzsnTo7X8QZtytNDTzo62tAzw8dnCMwe9GhHWwwotWBUdHwoXY9qqoSUqWrtmQJb1AQ1cTdaR:  cmd/c4
c44AvENJsDgamsSoBh7yCZHRkyn9NMbckZCY7VEVPxqfLfxNLeVmDNJ9nTugLYcyi2JYP492exkNS7KTAhoDwVms3z:  cmd/c4id/README.md
c45wRLpVXHr8FYD2GP8ZzKnGJ3GiwmWFDJCwhQ5sh8zdznp5tFXfszCe1wqrcFBZ8b3nPkYNfLqt4Y48WbbUeT7pqr:  cmd/c4id/main.go
c45p5snKG8z3vU6QcM3buW8r3LAQUeKQLUFcvqTCBJZr3Uzn8PMZrBbvF5dRvmaR4A9AEZLEwHnSFkixBkyE8dRoRU:  cmd/c4id
c436eg2b8W9Rguo6B9Arve1Lmh9sgkRJtXBvxvZjTdwmPMByTtjL7WourHQkb1HZz9mKBxEB9F5i4hq1S24N1ijwSB:  cmd
```

### Help output:

```bash
Usage: c4 [flags] [file]

  c4 generates c4ids for all files and folders spacified.
  If no file is given c4 will read piped data.
  Output is in YAML format.

flags:
  -a, --absolute=false: Output absolute paths, instead of relative paths.
  -d, --depth=0: Only output ids for files and folders 'depth' directories deep.
  -f, --formatting="id": Output formatting options.
          "id": c4id oriented.
          "path": path oriented.
  -L, --links=false: All symbolic links are followed.
  -m, --metadata=false: Include filesystem metadata.
          "url" is always included unless data is piped, or only a single file is specified.
  -R, --recursive=false: Recursively identify all files for the given url.
  -v, --version=false: Show version information.
```



