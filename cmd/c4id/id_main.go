package main

import (
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/blang/vfs"

	c4 "github.com/Avalanche-io/c4/id"
	c4os "github.com/Avalanche-io/c4/os"
	flag "github.com/ogier/pflag"
)

func id_main(f *flag.FlagSet) error {
	file_list := f.Args()
	if depth < 0 {
		depth = 0
	}

	if len(file_list) == 0 {
		err := identify_pipe()
		if err != nil {
			return err
		}
		return nil
	}
	_, a := c4.Parse(file_list[0])
	_, b := shaParse(file_list[0])
	if a == nil || b == nil {
		if _, err := os.Stat(file_list[0]); os.IsNotExist(err) {
			return id_calc(file_list)
		}
	}
	err := identify_files(file_list, depth)
	if err != nil {
		return err
	}
	return nil
}

func identify_pipe() error {
	stat, err := os.Stdin.Stat()
	if err != nil {
		return err
	}
	if (stat.Mode() & os.ModeCharDevice) != 0 {
		flag.Usage()
		return errors.New("no input")
	}
	id := encode(os.Stdin)
	printID(id)
	return nil
}

func identify_files(file_list []string, depth int) error {
	for _, file := range file_list {
		path, err := filepath.Abs(file)
		if err != nil {
			return err
		}
		fs := c4os.NewFileSystem(vfs.OS(), []byte(path))
		err = fs.Walk(nil, func(key []byte, attrs c4os.Attributes) error {
			fmt.Printf("%s: %s\n", string(key), attrs.ID())
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func id_calc(list []string) error {
	var ids c4.DigestSlice
	var shaerr error
	for _, s := range list {
		id, err := c4.Parse(s)
		if err != nil {
			id, shaerr = shaParse(s)
			if shaerr != nil {
				return errors.New(fmt.Sprintf("failed to parse %s", s))
			}
			fmt.Printf("%s: %s\n", s, id)
		}
		ids.Insert(id.Digest())
	}
	for _, dg := range ids {
		h := make([]byte, hex.EncodedLen(len(dg)))
		hex.Encode(h, dg)
		fmt.Printf("%s: sha512-%s\n", dg.ID(), string(h))
	}
	fmt.Printf("%s\n", ids.Digest().ID())
	return nil
}

func shaParse(sha string) (*c4.ID, error) {
	if len(sha) != 135 {
		return nil, errors.New("wrong length to be hex encoded sha512 " + strconv.Itoa(len(sha)))
	}
	data, err := hex.DecodeString(sha[7:])
	if err != nil {
		return nil, err
	}
	dg := c4.NewDigest(data)
	return dg.ID(), nil
}
