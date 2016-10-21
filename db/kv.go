package db

import (
	"github.com/Workiva/go-datastructures/trie/ctrie"
)

type KV ctrie.Ctrie

type kve struct {
	key   string
	value interface{}
}

func (k *kve) Key() string {
	return k.key
}

func (k *kve) Value() interface{} {
	return k.value
}

func NewKV() *KV {
	return (*KV)(ctrie.New(nil))
}

func (kv *KV) Set(key string, value interface{}) {
	(*ctrie.Ctrie)(kv).Insert([]byte(key), value)
}

func (kv *KV) Get(key string) interface{} {
	val, ok := (*ctrie.Ctrie)(kv).Lookup([]byte(key))
	if ok {
		return val
	}
	return nil
}

func (kv *KV) Snapshot() *KV {
	return (*KV)((*ctrie.Ctrie)(kv).Snapshot())
}

func (kv *KV) Iterator(cancel <-chan struct{}) <-chan Element {
	out := make(chan Element)

	ch := (*ctrie.Ctrie)(kv).Iterator(cancel)
	go func() {
		for e := range ch {
			ent := kve{string(e.Key), e.Value}
			out <- &ent
		}
		close(out)
	}()

	return out
}
