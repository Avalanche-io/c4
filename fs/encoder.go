package fs

import (
	// "encoding/json"
	// "errors"
	// "fmt"
	"io"
)

type EncoderState int

const (
	EncoderStart EncoderState = iota
	EncoderBuffered
	EncoderPending
	EncoderEOF
)

type NodeEncoder struct {
	// node  *Node
	item  Item
	buf   []byte
	state EncoderState
}

func (n *NodeEncoder) Read(p []byte) (int, error) {
	return 0, nil
}

func JsonEncoder(ch <-chan Item) <-chan []byte {
	out := make(chan []byte)
	go func() {
		data := make([]byte, 512)
		stack := []*NodeEncoder{}
		for item := range ch {
			stack = append([]*NodeEncoder{NewNodeEncoder(item)}, stack...)
			done := 0
			for _, e := range stack {
				_, err := e.Read(data)
				if err == io.EOF {
					done += 1
				} else if err != nil {
					panic(err)
				}
				out <- data
			}
			stack = stack[done:]
		}
		close(out)
	}()
	return out
}

func NewNodeEncoder(item Item) *NodeEncoder {
	e := NodeEncoder{item, []byte{}, EncoderStart}
	return &e
}

// State Machine:
// EncoderStart
//  if id
//    encode n -> EncoderBuffered
//  if !id
//     if n is dir
//      encode "{" -> EncoderPending
//    if n not dir
//      encode ""  -> EncoderPending
// EncoderBuffered
//   return data
//   if buffer empty -> EncoderEOF
//   if buffer !empty -> EncoderBuffered
// EncoderPending
//  if !id
//    -> EncoderPending
//  if id
//    if n is dir
//      encode "'.c4':"
//      encode n -> EncoderBuffered
//    if n not dir
//      encode n -> EncoderBuffered
// EncoderEOF
//   return EOF
//

// func (e *NodeEncoder) Read(p []byte) (int, error) {
// 	var n int
// 	switch {
// 	case e.state == EncoderStart:
// 		if e.item["id"] == nil {
// 			e.state = EncoderPending
// 			if e.item["."] != nil {
// 				data := []byte(fmt.Sprintf("{\"%s\":", e.node.Name))
// 				n = copy(p, data)
// 				// n = copy(p, "{")
// 			} else {
// 				n = 0
// 			}
// 			return n, nil
// 		} else {
// 			data, err := json.Marshal(e.node)
// 			if err != nil {
// 				return 0, err
// 			}
// 			name := fmt.Sprintf("\"%s\":", e.node.Name)
// 			data = append([]byte(name), data...)
// 			n = copy(p, data)
// 			if n < len(data) {
// 				e.state = EncoderBuffered
// 				e.buf = data[n:]
// 				return n, nil
// 			} else {
// 				e.state = EncoderEOF
// 				return n, io.EOF
// 			}
// 		}
// 	case e.state == EncoderBuffered:
// 		n = copy(p, e.buf)
// 		if n < len(e.buf) {
// 			e.state = EncoderBuffered
// 			e.buf = e.buf[n:]
// 			return n, nil
// 		}
// 		e.state = EncoderEOF
// 		return n, io.EOF
// 	case e.state == EncoderPending:
// 		if e.node.Id == nil {
// 			// still pending
// 			return 0, nil
// 		}
// 		// Id != nil
// 		if e.node.IsDir {
// 			e.buf = append(e.buf, []byte(`".c4":`)...)
// 		}
// 		data, err := json.Marshal(e.node)
// 		if err != nil {
// 			return 0, err
// 		}
// 		e.buf = append(e.buf, data...)
// 		n = copy(p, e.buf)
// 		if n < len(e.buf) {
// 			e.state = EncoderBuffered
// 			e.buf = e.buf[n:]
// 			return n, nil
// 		} else {
// 			e.state = EncoderEOF
// 			return n, io.EOF
// 		}
// 	case e.state == EncoderEOF:
// 		return 0, io.EOF
// 	}
// 	return 0, errors.New("Reached, un-reachable code.")
// }
