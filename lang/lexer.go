package lang

import (
	"errors"
	"fmt"
	"strconv"
	"text/scanner"
)

type rangeLexer struct {
	s      scanner.Scanner
	neg    bool
	binary bool
	val    string
	err    error
	nums   []int64
	incr   int
	typ    int
}

func (l *rangeLexer) String() string {
	return fmt.Sprintf("rangeLexer{neg: %v, binary: %t, val: %s, err: %s, nums: %v, incr: %d, type: %d}\n",
		l.neg, l.binary, l.val, l.err, l.nums, l.incr, l.typ)
}

func (l *rangeLexer) Range() Ranger {
	if l.typ == BasicRange && len(l.nums) > 1 {
		i := int64(1)
		if l.incr != -1 {
			i = l.nums[l.incr]
		}
		if l.nums[0] > l.nums[1] && i > 0 {
			i = -i
		}
		return basic{l.nums[0], l.nums[1], i, l.binary}
	}
	if l.typ == ListRange {
		return list{l.nums, l.binary}
	}
	return nil
}

func (l *rangeLexer) Error() error {
	return l.err
}

type stateFn func(l *rangeLexer) stateFn

func lexRangeBegin(l *rangeLexer) stateFn {
	return lexInt
}

func lexRangeEnd(l *rangeLexer) stateFn {
	return lex
}

func lexRange(l *rangeLexer) stateFn {
	tok := l.s.Scan()
	switch tok {
	case ']':
		return lexRangeEnd
	case ',':
		return lexComma
	case '-':
		return lexDash
	case '^':
		l.binary = true
		return lexRange
	case '/':
		l.incr = len(l.nums)
		return lexInt
	}
	l.err = errors.New("unrecognized symbol " + scanner.TokenString(tok))
	return lexError
}

func lexComma(l *rangeLexer) stateFn {
	if !(l.typ == NilRange || l.typ == ListRange) {
		l.err = errors.New("conflicting range types")
		return lexError
	}
	l.typ = ListRange
	return lexInt
}

func lexDash(l *rangeLexer) stateFn {
	if !(l.typ == NilRange || l.typ == BasicRange) {
		l.err = errors.New("conflicting range types")
		return lexError
	}
	l.typ = BasicRange
	return lexInt
}

func lexError(l *rangeLexer) stateFn {
	return nil
}

func lexInt(l *rangeLexer) stateFn {
	tok := l.s.Scan()
	if tok == '-' {
		if len(l.val) != 0 {
			panic("bad value length")
		}
		l.val = "-"
		tok = l.s.Scan()
	}
	if tok != scanner.Int {
		l.err = errors.New("expected integer got " + scanner.TokenString(tok))
		return lexError
	}
	l.val += l.s.TokenText()
	n, err := strconv.ParseInt(l.val, 10, 64)
	if err != nil {
		l.err = err
		return lexError
	}
	l.nums = append(l.nums, n)
	l.val = ""
	return lexRange
}

func lex(l *rangeLexer) stateFn {
	var tok rune
	for tok != scanner.EOF {
		tok = l.s.Scan()
		if tok == '[' {
			return lexRangeBegin(l)
		}
		// else parse other c4 language inputs
	}
	return nil
}
