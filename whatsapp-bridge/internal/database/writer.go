package database

import (
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	// ErrStoreClosed is returned when an operation is submitted to a closed store.
	ErrStoreClosed = errors.New("message store is closed")
	// ErrWriteTimeout is returned when a synchronous write operation exceeds the deadline.
	ErrWriteTimeout = errors.New("database write operation timed out")
)

const (
	maxBatchSize     = 200
	priorityQueueCap = 2000
	bulkQueueCap     = 5000
	writeTimeout     = 30 * time.Second
)

type writeTask struct {
	execute func(tx *sql.Tx) error
	done    chan error // nil for fire-and-forget tasks
}

// writerQueue serializes every write to messages.db through one goroutine.
//
// Producers submit closures; the worker gathers whatever is queued (real-time writes first)
// and commits them as one transaction, so a burst of messages costs one fsync instead of one
// per message. Each task runs under its own savepoint: a task that fails is rolled back and
// reported to its caller without discarding the rest of the batch.
type writerQueue struct {
	priorityQueue chan writeTask
	bulkQueue     chan writeTask
	closeCh       chan struct{}
	wg            sync.WaitGroup
	initOnce      sync.Once

	mu       sync.RWMutex // guards closed; held while a producer is sending
	closed   bool
	errMu    sync.RWMutex
	onFailed func(error)
}

// ensureWriter initializes the writer queue and worker goroutine if not already running.
func (store *MessageStore) ensureWriter() {
	if store.writer == nil {
		store.writer = &writerQueue{}
	}

	store.writer.initOnce.Do(func() {
		store.writer.priorityQueue = make(chan writeTask, priorityQueueCap)
		store.writer.bulkQueue = make(chan writeTask, bulkQueueCap)
		store.writer.closeCh = make(chan struct{})

		store.writer.wg.Add(1)
		go store.startWriterWorker()
	})
}

// SetWriteErrorHandler registers a callback for failures of writes nobody waits for
// (history sync). Without one, those failures are printed.
func (store *MessageStore) SetWriteErrorHandler(fn func(error)) {
	store.ensureWriter()
	store.writer.errMu.Lock()
	store.writer.onFailed = fn
	store.writer.errMu.Unlock()
}

func (wq *writerQueue) reportFailure(err error) {
	wq.errMu.RLock()
	fn := wq.onFailed
	wq.errMu.RUnlock()
	if fn != nil {
		fn(err)
		return
	}
	fmt.Printf("Warning: queued database write failed: %v\n", err)
}

// enqueueWrite routes a write through the single-writer worker.
// When wait is true it blocks until the task is committed (or fails). Bulk tasks go to the
// low-priority queue: the worker only takes from it when no real-time write is waiting.
func (store *MessageStore) enqueueWrite(execute func(tx *sql.Tx) error, isBulk bool, wait bool) error {
	store.ensureWriter()
	wq := store.writer

	var done chan error
	if wait {
		done = make(chan error, 1)
	}
	task := writeTask{execute: execute, done: done}

	target := wq.priorityQueue
	if isBulk {
		target = wq.bulkQueue
	}

	// The read lock is held while sending so close() cannot finish (and stop the worker)
	// between the closed check and the send: a task accepted here is always executed.
	wq.mu.RLock()
	if wq.closed {
		wq.mu.RUnlock()
		return ErrStoreClosed
	}
	target <- task
	wq.mu.RUnlock()

	if !wait {
		return nil
	}

	select {
	case err := <-done:
		return err
	case <-time.After(writeTimeout):
		return ErrWriteTimeout
	}
}

// close stops accepting writes, flushes what is queued and terminates the worker.
func (wq *writerQueue) close() {
	wq.mu.Lock()
	if wq.closed {
		wq.mu.Unlock()
		return
	}
	wq.closed = true
	if wq.closeCh != nil {
		close(wq.closeCh)
	}
	wq.mu.Unlock()

	wq.wg.Wait()
}

// startWriterWorker runs the single dedicated writer loop.
func (store *MessageStore) startWriterWorker() {
	wq := store.writer
	defer wq.wg.Done()

	batch := make([]writeTask, 0, maxBatchSize)

	// fill appends queued tasks without blocking: real-time first, then bulk.
	fill := func() {
		for len(batch) < maxBatchSize {
			select {
			case t := <-wq.priorityQueue:
				batch = append(batch, t)
				continue
			default:
			}
			select {
			case t := <-wq.bulkQueue:
				batch = append(batch, t)
			default:
				return
			}
		}
	}

	flush := func() {
		if len(batch) == 0 {
			return
		}
		store.executeBatch(batch)
		batch = batch[:0]
	}

	for {
		select {
		case <-wq.closeCh:
			// close() holds the lock while setting closed, so no producer is mid-send: what is
			// in the queues now is everything that will ever arrive.
			for {
				fill()
				if len(batch) == 0 {
					return
				}
				flush()
			}

		case t := <-wq.priorityQueue:
			batch = append(batch, t)
			fill()
			flush()

		case t := <-wq.bulkQueue:
			batch = append(batch, t)
			fill()
			flush()
		}
	}
}

// executeBatch runs the tasks in one transaction, each under its own savepoint.
func (store *MessageStore) executeBatch(batch []writeTask) {
	if store.db == nil {
		store.finish(batch, func(int) error { return fmt.Errorf("database not initialized") })
		return
	}
	tx, err := store.db.Begin()
	if err != nil {
		store.finish(batch, func(int) error { return err })
		return
	}

	results := make([]error, len(batch))
	for i, task := range batch {
		if _, err := tx.Exec("SAVEPOINT task"); err != nil {
			results[i] = err
			continue
		}
		if execErr := task.execute(tx); execErr != nil {
			results[i] = execErr
			_, _ = tx.Exec("ROLLBACK TO SAVEPOINT task")
		}
		_, _ = tx.Exec("RELEASE SAVEPOINT task")
	}

	if commitErr := tx.Commit(); commitErr != nil {
		_ = tx.Rollback()
		store.finish(batch, func(int) error { return commitErr })
		return
	}
	store.finish(batch, func(i int) error { return results[i] })
}

// finish reports each task's outcome to its waiter, or to the error handler when nobody waits.
func (store *MessageStore) finish(batch []writeTask, result func(i int) error) {
	for i, task := range batch {
		err := result(i)
		if task.done != nil {
			task.done <- err
		} else if err != nil {
			store.writer.reportFailure(err)
		}
	}
}
