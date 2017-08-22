package id

import (
	"bytes"
	// "fmt"
)

func (id *ID) MarshalBinary() (data []byte, err error) {
	if id == nil {
		return nil, errNil{}
	}
	return []byte(id.Digest()), nil
}

func (id *ID) UnmarshalBinary(data []byte) error {
	d := NewDigest(data)
	if d == nil {
		return errNil{}
	}
	id = d.ID()
	return nil
}

func (id *ID) MarshalText() (text []byte, err error) {
	if id == nil {
		return nil, errNil{}
	}
	return []byte(id.String()), nil
}

func (id *ID) UnmarshalText(text []byte) error {
	i, err := Parse(string(text))
	if err != nil {
		return err
	}
	id = i
	return nil
}

func (t *Tree) MarshalBinary() (data []byte, err error) {
	if t == nil || len(t.rows[0]) != 64 {
		return nil, errNil{}
	}

	if Digest(t.rows[0]).ID().Cmp(VOID_ID) == 0 {
		return nil, errNil{}
	}

	return t.data, nil
}

func (t *Tree) UnmarshalBinary(data []byte) error {
	// We test to see if the first 192 bytes are 3 valid digests, and that
	// the ID of the second two is the first.

	if len(data) < 192 {
		return errInvalidTree{}
	}
	head := make([]Digest, 3)
	for i := range head {
		ii := i * 64
		head[i] = Digest(data[ii : ii+64])
	}
	root := head[2].Sum(head[1])

	if bytes.Compare([]byte(root), []byte(head[0])) != 0 {
		return errInvalidTree{}
	}

	// At this point we know we have a valid ID tree, but we are not sure
	// of it's row structure.
	total := len(data) / 64
	length := listSize(total)
	t.rows, t.data = allocateTree(length)
	copy(t.data, data)

	return nil
}

func (t *Tree) MarshalText() (text []byte, err error) {
	return []byte(t.String()), nil
}

// func (t *Tree) UnmarshalText(text []byte) error {
// 	i, err := Parse(string(text))
// 	if err != nil {
// 		return err
// 	}
// 	id = i
// 	return nil
// }
