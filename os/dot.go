package os

import (
	"fmt"
	"math/big"
	"os"
	"time"

	c4 "github.com/Avalanche-io/c4/id"
	c4time "github.com/Avalanche-io/c4/time"
)

// The dot type is a temporary stand in for attributes.  It conforms to the os.FileInfo
// interface, but also the c4.Identifiable interface as well.
//
// Size is stored as a Big because it must be able represent a potentially very large
// value like the total size of all data in the world.
//

type dot struct {
	name     *string
	Id       *c4.ID      `json:"id"`
	SizeV    *big.Int    `json:"size"`
	ModeV    os.FileMode `json:"mode"`
	ModTimeV c4time.Time `json:"modtime"`
	isdir    bool
	sys      interface{}
}

func (d *dot) String() string {
	return fmt.Sprintf("*c4.dot=&{Name:%s, Id:%s, Size:%s, Mode:%s, Time:%s, IsDir:%t}",
		*d.name,
		d.Id,
		d.SizeV,
		d.ModeV,
		d.ModTimeV,
		d.isdir)
}

func (d *dot) Name() string {
	return *d.name
}

func (d *dot) Size() int64 {
	return d.SizeV.Int64()
}

func (d *dot) Mode() os.FileMode {
	return d.ModeV
}

func (d *dot) ModTime() time.Time {
	return d.ModTimeV.AsTime()
}

func (d *dot) IsDir() bool {
	return d.isdir
}

func (d *dot) Sys() interface{} {
	return d.sys
}

func (d *dot) Info(info os.FileInfo) *dot {
	if !info.IsDir() {
		d.SizeV = big.NewInt(info.Size())
	}
	d.ModTimeV = c4time.NewTime(info.ModTime())
	d.ModeV = info.Mode()
	d.sys = info.Sys()
	d.isdir = info.IsDir()
	name := info.Name()
	d.name = &name
	return d
}
