package database

import (
	"database/sql"
	"errors"
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
	maxBatchSize       = 100
	batchFlushInterval = 25 * time.Millisecond
	priorityQueueCap   = 2000
	bulkQueueCap       = 5000
	writeTimeout       = 10 * time.Second
)

type writeTask struct {
	execute func(tx *sql.Tx) error
	done    chan error
}

type writerQueue struct {
	priorityQueue chan writeTask
	bulkQueue     chan writeTask
	closeCh       chan struct{}
	wg            sync.WaitGroup
	initOnce      sync.Once
	closed        bool
	mu            sync.RWMutex
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

// enqueueWrite routes a write task through the single-writer worker.
// When wait is true, it blocks until the task is committed (or fails) within writeTimeout.
func (store *MessageStore) enqueueWrite(execute func(tx *sql.Tx) error, isBulk bool, wait bool) error {
	store.ensureWriter()

	store.writer.mu.RLock()
	if store.writer.closed {
		store.writer.mu.RUnlock()
		return ErrStoreClosed
	}
	store.writer.mu.RUnlock()

	var done chan error
	if wait {
		done = make(chan error, 1)
	}

	task := writeTask{
		execute: execute,
		done:    done,
	}

	targetQueue := store.writer.priorityQueue
	if isBulk {
		targetQueue = store.writer.bulkQueue
	}

	select {
	case targetQueue <- task:
	case <-store.writer.closeCh:
		return ErrStoreClosed
	}

	if wait {
		select {
		case err := <-done:
			return err
		case <-time.After(writeTimeout):
			return ErrWriteTimeout
		}
	}

	return nil
}

// close flushes pending tasks and terminates the writer worker.
func (wq *writerQueue) close() {
	wq.mu.Lock()
	if wq.closed {
		wq.mu.Unlock()
		return
	}
	wq.closed = true
	close(wq.closeCh)
	wq.mu.Unlock()

	wq.wg.Wait()
}

// startWriterWorker runs the single dedicated writer loop, draining queues and committing transactions in batches.
func (store *MessageStore) startWriterWorker() {
	defer store.writer.wg.Done()

	ticker := time.NewTicker(batchFlushInterval)
	defer ticker.Stop()

	var batch []writeTask

	flush := func() {
		if len(batch) == 0 {
			return
		}
		store.executeBatch(batch)
		batch = batch[:0]
	}

	for {
		select {
		case <-store.writer.closeCh:
			// Drain all remaining tasks before terminating
			for {
				select {
				case task := <-store.writer.priorityQueue:
					batch = append(batch, task)
					if len(batch) >= maxBatchSize {
						flush()
					}
				case task := <-store.writer.bulkQueue:
					batch = append(batch, task)
					if len(batch) >= maxBatchSize {
						flush()
					}
				default:
					flush()
					return
				}
			}

		case task := <-store.writer.priorityQueue:
			batch = append(batch, task)
		drainPriority:
			for len(batch) < maxBatchSize {
				select {
				case t := <-store.writer.priorityQueue:
					batch = append(batch, t)
				default:
					break drainPriority
				}
			}
			if len(batch) >= maxBatchSize {
				flush()
			}

		case task := <-store.writer.bulkQueue:
			batch = append(batch, task)
		drainBulk:
			for len(batch) < maxBatchSize {
				// Priority queue always takes precedence
				select {
				case p := <-store.writer.priorityQueue:
					batch = append(batch, p)
					continue
				default:
				}

				select {
				case b := <-store.writer.bulkQueue:
					batch = append(batch, b)
				default:
					break drainBulk
				}
			}
			if len(batch) >= maxBatchSize {
				flush()
			}

		case <-ticker.C:
			flush()
		}
	}
}

// executeBatch executes an accumulated slice of write tasks within a transaction.
// If any task fails during the batch, it rolls back and executes tasks individually to isolate poison pills.
func (store *MessageStore) executeBatch(batch []writeTask) {
	if len(batch) == 0 {
		return
	}

	tx, err := store.db.Begin()
	if err != nil {
		for _, task := range batch {
			if task.done != nil {
				task.done <- err
			}
		}
		return
	}

	var anyErr bool
	for _, task := range batch {
		if taskErr := task.execute(tx); taskErr != nil {
			anyErr = true
			break
		}
	}

	if anyErr {
		// Roll back batch transaction and fall back to sequential execution per task
		_ = tx.Rollback()

		for _, task := range batch {
			individualTx, err := store.db.Begin()
			if err != nil {
				if task.done != nil {
					task.done <- err
				}
				continue
			}

			execErr := task.execute(individualTx)
			if execErr != nil {
				_ = individualTx.Rollback()
				if task.done != nil {
					task.done <- execErr
				}
			} else {
				commitErr := individualTx.Commit()
				if task.done != nil {
					task.done <- commitErr
				}
			}
		}
		return
	}

	// Normal path: commit the whole batch in one fsync
	if commitErr := tx.Commit(); commitErr != nil {
		for _, task := range batch {
			if task.done != nil {
				task.done <- commitErr
			}
		}
		return
	}

	// Notify all tasks of successful commit
	for _, task := range batch {
		if task.done != nil {
			task.done <- nil
		}
	}
}
