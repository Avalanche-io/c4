package fs

import (
	// "errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/etcenter/c4/asset"
)

// type Task string

// type TaskFunc func(src string, b *Buffer)
type TaskFunc func(i *Item, b *Buffer) error
type StartFunc func(i *Item, mtb *MultiTaskBuffer) (*Buffer, error)

type TaskBuffer struct {
	Engine   *TaskEngine
	Source   *Item
	Data     *Buffer
	Complete []bool
}

func (b *TaskBuffer) Done(qId int) {
	b.Complete[qId] = true
	done := true
	for _, c := range b.Complete {
		if c == false {
			done = false
			break
		}
	}
	if done == true {
		if b.Data != nil {
			b.Data.Release()
		}
	}
}

type TaskQueue struct {
	Key     string
	Ch      chan *TaskBuffer
	Do      TaskFunc
	wg      sync.WaitGroup
	Timelog []time.Duration
}

func (q *TaskQueue) Close() {
	q.wg.Wait()
	close(q.Ch)
}

func (q *TaskQueue) AddWait() {
	q.wg.Add(1)
}

func (q *TaskQueue) DoneWait() {
	q.wg.Done()
}

func (q *TaskQueue) LogTime(d time.Duration) {
	q.Timelog = append(q.Timelog, d)
}

func (q *TaskQueue) AvgTime() time.Duration {
	total := time.Duration(0)
	for _, t := range q.Timelog {
		total += t
	}
	return total / time.Duration(len(q.Timelog))
}

type TaskEngine struct {
	Threads int
	Queues  []*TaskQueue
	Index   map[string]int
	Ich     chan *Item
	errCh   chan error
	MTB     *MultiTaskBuffer
	wg      sync.WaitGroup
	DoStart StartFunc
}

func NewTaskEngine(names []string, buffers uint64) *TaskEngine {
	qs := make([]*TaskQueue, len(names))
	idx := make(map[string]int)
	for i, n := range names {
		q := TaskQueue{
			Key:     n,
			Ch:      make(chan *TaskBuffer),
			Timelog: make([]time.Duration, 0, 200),
		}
		qs[i] = &q
		idx[n] = i
	}
	e := TaskEngine{
		Threads: 0,
		Queues:  qs,
		Index:   idx,
		Ich:     make(chan *Item),
		DoStart: DefaultStartTask,
		MTB:     NewMTB(buffers),
	}
	return &e
}

func (e *TaskEngine) EnqueueFS(fs *FileSystem) {
	ch := fs.Walk()
	for item := range ch {
		e.Ich <- item
	}
	close(e.Ich)
}

func (e *TaskEngine) Start() <-chan error {
	ech := make(chan error)
	e.errCh = ech
	go func() {
		for item := range e.Ich {
			d, err := e.DoStart(item, e.MTB)
			if err != nil {
				ech <- err
			}
			if !item.IsDir() && d == nil {
				fmt.Printf("err = %s\n", Red(err.Error()))
				panic("Start(): ")
			}
			b := TaskBuffer{
				Engine:   e,
				Source:   item,
				Data:     d,
				Complete: make([]bool, len(e.Queues)),
			}
			for _, q := range e.Queues {
				q.AddWait()
				go func(b *TaskBuffer, q *TaskQueue) {
					q.Ch <- b
					q.DoneWait()
				}(&b, q)
			}
		}
		for _, q := range e.Queues {
			go func(q *TaskQueue) {
				q.Close()
			}(q)
		}
	}()
	e.do(ech)
	return ech
}

func (e *TaskEngine) StartTask(f StartFunc) {
	e.DoStart = f
}

func (e *TaskEngine) TaskHandler(queue_name string, f TaskFunc) {
	i := e.Index[queue_name]
	e.Queues[i].Do = f
}

func (e *TaskEngine) do(ech chan<- error) {
	threads := len(e.Queues)
	if threads <= 0 {
		panic("Cannot run tasks, queues not initialized")
	} else if e.Threads > 0 && e.Threads <= threads {
		threads = e.Threads
	}
	batch := len(e.Queues) / threads
	for j := 0; j < threads; j++ {
		e.AddWait()
		go func(j int) {
			for k := 0; k < batch; k++ {
				qID := j*batch + k
				q := e.Queues[qID]
				for b := range q.Ch {
					start := time.Now()
					err := q.Do(b.Source, b.Data)
					if err != nil {
						ech <- err
					}
					end := time.Now()
					q.LogTime(end.Sub(start))
					b.Done(qID)
				}
			}
			e.DoneWait()
		}(j)
	}
}

func (e *TaskEngine) Close() {
	e.wg.Wait()
	close(e.errCh)
}

func (e *TaskEngine) AddWait() {
	e.wg.Add(1)
}

func (e *TaskEngine) DoneWait() {
	e.wg.Done()
}

func DefaultStartTask(item *Item, mtb *MultiTaskBuffer) (*Buffer, error) {
	if item.IsDir() {
		return nil, nil
	}

	f, err := os.OpenFile(item.Path(), os.O_RDONLY, 0777)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b := mtb.Get(uint64(item.Get("size").(int64)))
	f.Read(b.Bytes())
	return b, nil
}

func IdTask(item *Item, b *Buffer) error {
	var id *asset.ID
	var err error
	if item.IsDir() {
		item.Set("id", id)
		return nil
	}
	if b == nil {
		item.Print()
		panic(Bold(Blue("Buffer is nil")))
	}
	id, err = asset.Identify(b.Reader())
	if err != nil {
		return err
	}
	item.SetAttribute("id", id)
	return nil
}
