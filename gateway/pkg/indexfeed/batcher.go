package indexfeed

import (
	"context"
	"sync"
	"time"

	"github.com/kumarabd/gokit/logger"
	"github.com/kumarabd/ingestion-plane/gateway/pkg/types"
)

// Batcher batches mined records for efficient processing
type Batcher struct {
	client   Client
	log      *logger.Handler
	maxBatch int
	maxWait  time.Duration
	batch    []types.MinedRecord
	mu       sync.Mutex
	flushCh  chan struct{}
	stopCh   chan struct{}
	wg       sync.WaitGroup
}

// NewBatcher creates a new batcher
func NewBatcher(client Client, log *logger.Handler, maxBatch int, maxWait time.Duration) *Batcher {
	b := &Batcher{
		client:   client,
		log:      log,
		maxBatch: maxBatch,
		maxWait:  maxWait,
		batch:    make([]types.MinedRecord, 0, maxBatch),
		flushCh:  make(chan struct{}, 1),
		stopCh:   make(chan struct{}),
	}

	// Start background processing
	b.wg.Add(1)
	go b.run()

	return b
}

// Add adds a mined record to the batch
func (b *Batcher) Add(ctx context.Context, record types.MinedRecord) error {
	b.mu.Lock()
	b.batch = append(b.batch, record)
	shouldFlush := len(b.batch) >= b.maxBatch
	b.mu.Unlock()

	if shouldFlush {
		select {
		case b.flushCh <- struct{}{}:
		default:
		}
	}

	return nil
}

// run processes batches in the background
func (b *Batcher) run() {
	defer b.wg.Done()

	ticker := time.NewTicker(b.maxWait)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			b.flush()
		case <-b.flushCh:
			b.flush()
		case <-b.stopCh:
			b.flush() // Final flush
			return
		}
	}
}

// flush processes the current batch
func (b *Batcher) flush() {
	b.mu.Lock()
	if len(b.batch) == 0 {
		b.mu.Unlock()
		return
	}

	// Copy batch and clear
	batch := make([]types.MinedRecord, len(b.batch))
	copy(batch, b.batch)
	b.batch = b.batch[:0]
	b.mu.Unlock()

	// Process each record individually
	// Note: The indexfeed service expects individual candidates, not batches
	for _, record := range batch {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := b.client.ProcessMinedRecord(ctx, record)
		cancel()

		if err != nil {
			b.log.Error().Err(err).Str("template_id", record.Primary.TemplateID).Msg("Failed to publish candidate in batch")
		} else {
			b.log.Debug().Str("template_id", record.Primary.TemplateID).Msg("Successfully published candidate in batch")
		}
	}
}

// Close gracefully shuts down the batcher
func (b *Batcher) Close(ctx context.Context) error {
	close(b.stopCh)

	done := make(chan struct{})
	go func() {
		b.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
