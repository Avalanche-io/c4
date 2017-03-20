package db

import (
	"bytes"
	"fmt"
	"math/big"
	"testing"
)

func TestIncr(t *testing.T) {

	tests := []struct {
		In  []byte
		Exp []byte
	}{
		// {
		// 	In:  nil,
		// 	Exp: []byte{0x01},
		// },
		{
			In:  []byte{0x01},
			Exp: []byte{0x02},
		},
		{
			In:  []byte{0x01},
			Exp: []byte{0x02},
		},
		{
			In:  []byte{0xfe},
			Exp: []byte{0xff},
		},
		{
			In:  []byte{0xff},
			Exp: []byte{0x01, 0x00},
		},
		{
			In:  []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
			Exp: []byte{0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
		},
		{
			In:  []byte{0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
			Exp: []byte{0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01},
		},
		{
			In:  []byte{0x01, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
			Exp: []byte{0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
		},
	}

	cases := []struct {
		Name string
		F    func([]byte) []byte
	}{
		{"incr", incr},
	}
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			// f := reflect.ValueOf(c.F)
			for i, test := range tests {
				x := big.NewInt(0)
				if test.In == nil {
					x.SetUint64(0)
				} else {
					x.SetBytes(test.In)
				}
				out := c.F(x.Bytes())
				y := big.NewInt(0)
				z := big.NewInt(0)
				y.SetBytes(out)
				z.Sub(y, x)
				if z.Uint64() != 1 {
					t.Fatalf("%d: Incorrect increment %s (%s, %s)\n", i, z.String(), x.String(), y.String())
				}
				if bytes.Compare(y.Bytes(), test.Exp) != 0 {
					fmt.Printf("%d: Incorrect bytes %d (%v, %v)\n", i, z.Bytes(), x.Bytes(), y.Bytes())
					t.Fatalf("%d: in: %v, out: %v, exp: %v\n", i, x.Bytes(), out, test.Exp)
				}
			}
		})
	}
}

func TestDecr(t *testing.T) {

	tests := []struct {
		In  []byte
		Exp []byte
	}{
		// {
		// 	In:  nil,
		// 	Exp: []byte{0x01},
		// },
		{
			In:  []byte{0x02},
			Exp: []byte{0x01},
		},
		{
			In:  []byte{0x02},
			Exp: []byte{0x01},
		},
		{
			In:  []byte{0xff},
			Exp: []byte{0xfe},
		},
		{
			In:  []byte{0x01, 0x00},
			Exp: []byte{0xff},
		},
		{
			In:  []byte{0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
			Exp: []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
		},
		{
			In:  []byte{0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01},
			Exp: []byte{0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
		},
		{
			In:  []byte{0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
			Exp: []byte{0x01, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
		},
		{
			In:  []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
			Exp: []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xfe},
		},
	}

	cases := []struct {
		Name string
		F    func([]byte) []byte
	}{
		{"decr", decr},
	}
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			// f := reflect.ValueOf(c.F)
			for i, test := range tests {
				x := big.NewInt(0)
				if test.In == nil {
					x.SetUint64(0)
				} else {
					x.SetBytes(test.In)
				}
				out := c.F(x.Bytes())
				y := big.NewInt(0)
				z := big.NewInt(0)
				y.SetBytes(out)
				z.Sub(x, y)
				if z.Uint64() != 1 {
					t.Fatalf("%d: Incorrect decrement %s (%s, %s)\n", i, z.String(), x.String(), y.String())
				}
				if bytes.Compare(y.Bytes(), test.Exp) != 0 {
					fmt.Printf("%d: Incorrect bytes %d (%v, %v)\n", i, z.Bytes(), x.Bytes(), y.Bytes())
					t.Fatalf("%d: in: %v, out: %v, exp: %v\n", i, x.Bytes(), out, test.Exp)
				}
			}
		})
	}
}

func BenchmarkUint64Increment(b *testing.B) {
	var i uint64
	for n := 0; n < b.N; n++ {
		i++
	}
}

func BenchmarkUnBoundIncrement(b *testing.B) {
	var i []byte
	for n := 0; n < b.N; n++ {
		i = incr(i)
	}
}

func BenchmarkUnBoundDecrement(b *testing.B) {
	bint := big.NewInt(int64(b.N) * 2)
	i := bint.Bytes()
	for n := 0; n < b.N; n++ {
		i = decr(i)
	}
}
