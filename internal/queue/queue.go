// Package queue implements SagaFlow's durable in-process generation workers.
// The queue state lives in SQLite, so a restart never depends on Redis and
// never loses a submitted job.
package queue

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/gofurry/sagaflow/internal/config"
	"github.com/gofurry/sagaflow/internal/store/db"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type ExecuteFunc func(context.Context, uuid.UUID) error

type TaskInfo struct {
	ID string `json:"id"`
}

type Client struct {
	store   *db.Store
	cfg     config.JobsConfig
	execute ExecuteFunc
	log     *zap.Logger
	notify  chan struct{}
	once    sync.Once
}

func NewClient(store *db.Store, cfg config.JobsConfig, execute ExecuteFunc, log *zap.Logger) *Client {
	if log == nil {
		log = zap.NewNop()
	}
	return &Client{store: store, cfg: cfg, execute: execute, log: log, notify: make(chan struct{}, 1)}
}

func (c *Client) EnqueueGeneration(ctx context.Context, jobID uuid.UUID) (*TaskInfo, error) {
	if jobID == uuid.Nil {
		return nil, fmt.Errorf("generation job id is required")
	}
	if _, err := c.store.GetGenerationJob(ctx, jobID); err != nil {
		return nil, err
	}
	select {
	case c.notify <- struct{}{}:
	default:
	}
	return &TaskInfo{ID: jobID.String()}, nil
}

func (c *Client) Run(ctx context.Context) error {
	if c == nil || c.store == nil || c.execute == nil {
		return fmt.Errorf("queue dependencies are required")
	}
	var runErr error
	c.once.Do(func() {
		interrupted, err := c.store.InterruptRunningJobs(ctx)
		if err != nil {
			runErr = fmt.Errorf("recover generation queue: %w", err)
			return
		}
		if interrupted > 0 {
			c.log.Warn("marked unfinished generation jobs as interrupted", zap.Int64("count", interrupted))
		}
		concurrency := c.cfg.Concurrency
		if concurrency < 1 {
			concurrency = 1
		}
		for i := 0; i < concurrency; i++ {
			go c.worker(ctx, i+1)
		}
	})
	return runErr
}

func (c *Client) Close() error { return nil }

func (c *Client) worker(ctx context.Context, number int) {
	poll := parseDuration(c.cfg.PollInterval, 750*time.Millisecond)
	lease := parseDuration(c.cfg.Lease, 2*time.Hour)
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.notify:
		case <-timer.C:
		}
		for {
			job, err := c.store.ClaimNextGenerationJob(ctx, lease)
			if errors.Is(err, db.ErrNotFound) {
				break
			}
			if err != nil {
				if ctx.Err() == nil {
					c.log.Error("claim generation job", zap.Int("worker", number), zap.Error(err))
				}
				break
			}
			if err := c.execute(ctx, job.ID); err != nil && ctx.Err() == nil {
				c.log.Error("execute generation job", zap.String("job_id", job.ID.String()), zap.Error(err))
			}
		}
		timer.Reset(poll)
	}
}

func parseDuration(value string, fallback time.Duration) time.Duration {
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}
