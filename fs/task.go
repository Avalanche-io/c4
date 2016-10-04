package fs

import (
	// "fmt"
	"sync"
	"time"
)

type Task string

type TaskFunc func(src string, b *Buffer)
type StartFunc func(src string) *Buffer

type TaskBuffer struct {
	Engine   *TaskEngine
	Source   string
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
		b.Data.Release()
	}
}

type TaskQueue struct {
	Key     string
	Ch      chan *TaskBuffer
	Do      TaskFunc
	WG      sync.WaitGroup
	Timelog []time.Duration
}

func (q *TaskQueue) Close() {
	q.WG.Wait()
	close(q.Ch)
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
	Ich     chan string
	// MTB       *MultiTaskBuffer
	WG      sync.WaitGroup
	DoStart StartFunc
}

func NewTaskEngine(names []string) *TaskEngine {
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
		Ich:     make(chan string),
		// MTB:    NewMTB(200),
	}
	return &e
}

func (e *TaskEngine) Add(s string) {
	e.Ich <- s
}

// Starts the read thread, reading and closing all inputs
// inputs are consumed, or buffer is full.
func (e *TaskEngine) InputDone() {
	close(e.Ich)
}

func (e *TaskEngine) Start() {
	go func() {
		for t := range e.Ich {
			d := e.DoStart(t)
			b := TaskBuffer{
				Engine:   e,
				Source:   t,
				Data:     d, // blocks if all buffers are in use, until buffer is released
				Complete: make([]bool, len(e.Queues)),
			}
			for _, q := range e.Queues {
				q.WG.Add(1)
				go func(b *TaskBuffer, q *TaskQueue) {
					q.Ch <- b
					q.WG.Done()
				}(&b, q)
			}
		}
		for _, q := range e.Queues {
			go func(q *TaskQueue) {
				q.Close()
			}(q)
		}
	}()
	e.do()
}

func (e *TaskEngine) StartTask(f StartFunc) {
	e.DoStart = f
}

func (e *TaskEngine) TaskHandler(queue_name string, f TaskFunc) {
	i := e.Index[queue_name]
	e.Queues[i].Do = f
}

func (e *TaskEngine) do() {
	threads := len(e.Queues)
	if e.Threads > 0 && e.Threads <= threads {
		threads = e.Threads
	}
	e.WG.Add(threads)
	batch := len(e.Queues) / threads
	for j := 0; j < threads; j++ {
		go func(j int) {
			for k := 0; k < batch; k++ {
				qID := j*batch + k
				q := e.Queues[qID]
				for b := range q.Ch {
					// outstr := fmt.Sprintf("Queue %d %s: source: %s", qID, q.Key, b.Source)
					start := time.Now()
					q.Do(b.Source, b.Data)
					end := time.Now()
					q.LogTime(end.Sub(start))
					b.Done(qID)
				}
			}
			e.WG.Done()
		}(j)
	}
}

func (e *TaskEngine) Close() {
	e.WG.Wait()
}
