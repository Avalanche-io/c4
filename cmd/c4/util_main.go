package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"

	"github.com/etcenter/c4/asset"
	flag "github.com/ogier/pflag"
)

func util_main(f *flag.FlagSet) {
	list := f.Args()

	if cmd_util_string == "oldnewid" {
		if len(list) == 0 {
			oldnewid_pipe()
			return
		}
		oldnewid_args(list)
	}

}

func oldnewid_pipe() {
	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		ids := find_ids(bufio.NewReader(os.Stdin))
		oldnewid_output(ids)
		// for old := range ids {
		// 	fmt.Println(oldnewid(old))
		// }
	} else {
		flag.Usage()
	}
}

func oldnewid_output(ids <-chan string) {
	fmt.Printf("{")
	first := true
	for old_id := range ids {
		if !first {
			fmt.Printf(",")
		}
		first = false
		new_id := oldnewid(old_id)
		fmt.Printf("\"%s\":\"%s\"", old_id, new_id)
	}
	fmt.Printf("}")
}

func oldnewid_args(list []string) {
	for _, item := range list {
		ids := find_ids(bytes.NewReader([]byte(item)))
		oldnewid_output(ids)
	}
}

const (
	oldcharset string = "123456789abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ"
	newcharset string = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"
)

var (
	oldnewlut map[byte]byte
	newoldlut map[byte]byte
)

func init() {
	oldnewlut = make(map[byte]byte)
	newoldlut = make(map[byte]byte)
	for i := 0; i < len(oldcharset); i++ {
		oldnewlut[oldcharset[i]] = newcharset[i]
		newoldlut[newcharset[i]] = oldcharset[i]
	}
}

func oldnewid(old string) string {
	newid := "c4"

	for i := 2; i < 90; i++ {
		newid = newid + string(oldnewlut[old[i]])
	}

	return newid
}

func find_ids(in io.Reader) <-chan string {
	out := make(chan string)

	go func() {
		defer close(out)
		cnt := 0
		for {
			cnt++
			input_bytes := make([]byte, 90)
			n, err := in.Read(input_bytes)
			if err != nil {
				if err == io.EOF {
					return
				}
				panic(err)
			}
			offset := 0
			found := false
			for i := 0; i < n-1; i++ {
				if string(input_bytes[i:i+2]) == "c4" {
					input_bytes = input_bytes[i:]
					offset = i
					found = true
					break
				}
			}
			if !found {
				continue
			}
			more_bytes := make([]byte, offset+(90-n))
			n2 := 0
			for n2 < offset+(90-n) {
				n2, err = in.Read(more_bytes)
				if err != nil {
					if err == io.EOF {
						return
					}
					panic(err)
				}
			}
			id_out := string(input_bytes) + string(more_bytes)
			_, err = asset.ParseBytesID([]byte(id_out))
			if err == nil {
				out <- id_out
			}
		}
	}()

	return out
}
