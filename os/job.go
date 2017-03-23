package os

import (
	"sync"

	c4 "github.com/Avalanche-io/c4/id"
)

var Worker chan *job

func finish(job *job) {
	jobPool.Put(job)
}

type job struct {
	path *string
	Ids  *c4.DigestSlice
	done chan *c4.Digest
}

var jobPool = sync.Pool{
	New: func() interface{} {
		return new(job)
	},
}

func NewJob(path *string, ids *c4.DigestSlice, done chan *c4.Digest) *job {
	job := jobPool.Get().(*job)
	job.path = path
	job.Ids = ids
	job.done = done
	return job
}
