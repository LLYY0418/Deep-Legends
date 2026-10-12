package main

import (
	"context"
	"sync"
)

const cacheDiskQueueLimit = 64

type cacheDiskTicket struct {
	epoch, version uint64
	key            string
}
type cacheDiskTask struct {
	id     string
	ticket cacheDiskTicket
	ready  <-chan struct{}
	run    func(func() bool)
}

// Only queue metadata is protected here. No filesystem call or callback runs
// under this mutex; one bounded worker owns queued disk mutations.
type cacheDiskQueue struct {
	mu              sync.Mutex
	epoch           uint64
	versions        map[string]uint64
	invalid         map[string]bool
	tasks           []cacheDiskTask
	idle            chan struct{}
	barrier         chan struct{}
	running, closed bool
	// Observational metadata only: tests can distinguish slow progress from a
	// stalled writer. Production deadlines and queue policy do not use it.
	completed uint64
	progress  chan struct{}
}

func (q *cacheDiskQueue) ticket(key string) cacheDiskTicket {
	q.mu.Lock()
	defer q.mu.Unlock()
	return cacheDiskTicket{q.epoch, q.versions[key], key}
}
func (q *cacheDiskQueue) valid(t cacheDiskTicket) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return t.epoch == q.epoch && t.version == q.versions[t.key]
}
func (q *cacheDiskQueue) unreadable(key string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.invalid[key]
}
func (q *cacheDiskQueue) readable(t cacheDiskTicket) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if t.epoch == q.epoch && t.version == q.versions[t.key] {
		delete(q.invalid, t.key)
	}
}
func (q *cacheDiskQueue) submit(task cacheDiskTask, required bool) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed || task.ticket.epoch != q.epoch || task.ticket.version != q.versions[task.ticket.key] {
		return false
	}
	for i := range q.tasks {
		if q.tasks[i].id == task.id {
			q.tasks[i] = task
			return true
		}
	}
	if len(q.tasks) >= cacheDiskQueueLimit {
		if !required {
			return false
		}
		// Invalidations must be retained; an optional pending cache write can drop.
		q.tasks = append(q.tasks[:0], q.tasks[1:]...)
	}
	q.tasks = append(q.tasks, task)
	if !q.running {
		q.running = true
		q.idle = make(chan struct{})
		idle := q.idle
		go func() {
			defer recoverPanic("cache.diskQueue")
			q.drain(idle)
		}()
	}
	return true
}
func (q *cacheDiskQueue) drain(idle chan struct{}) {
	defer func() {
		q.mu.Lock()
		defer q.mu.Unlock()
		if q.idle == idle && q.running {
			clear(q.tasks)
			q.tasks = nil
			q.running = false
			close(idle)
		}
	}()
	completed := false
	for {
		q.mu.Lock()
		if completed {
			q.completed++
			if q.progress != nil {
				close(q.progress)
				q.progress = make(chan struct{})
			}
			completed = false
		}
		if len(q.tasks) == 0 {
			q.running = false
			close(q.idle)
			q.mu.Unlock()
			return
		}
		task := q.tasks[0]
		q.tasks[0] = cacheDiskTask{}
		q.tasks = q.tasks[1:]
		barrier := q.barrier
		q.mu.Unlock()
		if barrier != nil {
			<-barrier
		}
		if task.ready != nil {
			<-task.ready
		}
		if q.valid(task.ticket) {
			task.run(func() bool { return q.valid(task.ticket) })
		}
		completed = true
	}
}
func (q *cacheDiskQueue) clear() func() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.epoch++
	clear(q.tasks)
	q.tasks = nil
	q.invalid = nil
	if q.barrier != nil {
		close(q.barrier)
	}
	barrier := make(chan struct{})
	q.barrier = barrier
	return func() {
		q.mu.Lock()
		defer q.mu.Unlock()
		if q.barrier == barrier {
			q.barrier = nil
			close(barrier)
		}
	}
}
func (q *cacheDiskQueue) invalidate(key string) cacheDiskTicket {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.versions == nil {
		q.versions = make(map[string]uint64)
		q.invalid = make(map[string]bool)
	}
	if q.invalid == nil {
		q.invalid = make(map[string]bool)
	}
	q.versions[key]++
	q.invalid[key] = true
	return cacheDiskTicket{q.epoch, q.versions[key], key}
}
func (c *championDataCache) flushDisk(ctx context.Context) error {
	if c == nil {
		return nil
	}
	q := &c.diskQueue
	q.mu.Lock()
	idle, running := q.idle, q.running
	q.mu.Unlock()
	if !running {
		return nil
	}
	select {
	case <-idle:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (c *championDataCache) closeDisk(ctx context.Context) error {
	if c == nil {
		return nil
	}
	q := &c.diskQueue
	q.mu.Lock()
	q.closed = true
	q.mu.Unlock()
	err := c.flushDisk(ctx)
	if err != nil {
		q.mu.Lock()
		clear(q.tasks)
		q.tasks = nil
		q.epoch++ // A popped task that has not begun I/O also loses its ticket.
		q.mu.Unlock()
	}
	return err
}
func (c *championDataCache) enqueueDiskWrite(entry championCacheEnvelope) {
	if c == nil || c.dir == "" {
		return
	}
	ticket := c.diskQueue.ticket(entry.Key)
	c.diskQueue.submit(cacheDiskTask{id: "write|" + entry.Key, ticket: ticket, run: func(valid func() bool) {
		if c.writeDiskChecked(entry, valid) == nil {
			c.diskQueue.readable(ticket)
		}
	}}, false)
}
