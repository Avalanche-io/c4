package lang

import (
	"io"
	"text/scanner"
)

// updated, delete me

type Ranger interface {
	Start() int64
	End() int64
	Incr() int64
	IsBinary() bool
	Iterate(stop chan struct{}) chan int64
}

func ParseRange(in io.Reader) (Ranger, error) {
	var s scanner.Scanner
	s.Filename = "Ranger"
	s.Init(in)
	s.Mode = s.Mode &^ scanner.ScanIdents
	l := rangeLexer{s: s, incr: -1}
	for state := lex; state != nil; {
		state = state(&l)
	}
	return l.Range(), l.Error()
}

type basic struct {
	start    int64
	end      int64
	incr     int64
	isBinary bool
}

func (r basic) Start() int64 {
	return r.start
}

func (r basic) End() int64 {
	return r.end
}

func (r basic) Incr() int64 {
	return r.incr
}

func (r basic) IsBinary() bool {
	return r.isBinary
}

func gt(a, b int64) bool {
	return a >= b
}

func lt(a, b int64) bool {
	return a <= b
}

func abs(x int64) int64 {
	mask := x >> 63
	return x ^ mask - mask
}

func nextpow2(x int64) int64 {
	x = abs(x)
	x--
	x |= x >> 1
	x |= x >> 2
	x |= x >> 4
	x |= x >> 8
	x |= x >> 16
	x |= x >> 32
	return x + 1
}

func (r basic) Iterate(stop chan struct{}) chan int64 {
	out := make(chan int64)
	go func() {
		defer close(out)
		incr := r.incr
		end := r.end
		start := r.start
		eval := gt
		if r.isBinary {
			steps := (end - start) / incr
			step := nextpow2(steps) / 2
			for i := int64(0); i <= steps; i += step {
				select {
				case out <- start + i*incr:
				case <-stop:
					return
				}
			}
			for step > 1 {
				s := (step / 2)
				for i := s; i <= steps; i += step {
					select {
					case out <- start + i*incr:
					case <-stop:
						return
					}
				}
				step /= 2
			}
			return
		}
		if r.incr > 0 {
			eval = lt
		}
		for i := r.start; eval(i, r.end); i += incr {
			select {
			case out <- i:
			case <-stop:
				return
			}
		}
		return
	}()
	return out
}

type list struct {
	l        []int64
	isBinary bool
}

func (r list) Start() int64 {
	return r.l[0]
}

func (r list) End() int64 {
	return r.l[len(r.l)-1]
}

func (r list) Incr() int64 {
	return 1
}

func (r list) IsBinary() bool {
	return r.isBinary
}

func (l list) Iterate(stop chan struct{}) chan int64 {
	out := make(chan int64)
	go func() {
		defer close(out)
		for _, i := range l.l {
			select {
			case out <- i:
			case <-stop:
				return
			}
		}
	}()
	return out
}

const (
	NilRange int = iota
	BasicRange
	ListRange
)
