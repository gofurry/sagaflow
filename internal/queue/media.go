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

// MediaClient is a separate durable lane for local FFmpeg work. Keeping it
// independent prevents a long transcode from blocking model generation jobs.
type MediaClient struct {
	store   *db.Store
	cfg     config.JobsConfig
	execute ExecuteFunc
	log     *zap.Logger
	notify  chan struct{}
	once    sync.Once
	mu      sync.Mutex
	running map[uuid.UUID]context.CancelFunc
}

func NewMediaClient(store *db.Store, cfg config.JobsConfig, execute ExecuteFunc, log *zap.Logger) *MediaClient {
	if log == nil {
		log = zap.NewNop()
	}
	// Media processing is intentionally serialized on a personal workstation:
	// multiple encoders can otherwise exhaust CPU, RAM and disk bandwidth.
	cfg.Concurrency = 1
	return &MediaClient{store: store, cfg: cfg, execute: execute, log: log, notify: make(chan struct{}, 1), running: make(map[uuid.UUID]context.CancelFunc)}
}

func (c *MediaClient) Enqueue(ctx context.Context, jobID uuid.UUID) (*TaskInfo, error) {
	if jobID == uuid.Nil {
		return nil, fmt.Errorf("media job id is required")
	}
	if _, err := c.store.GetMediaJob(ctx, jobID); err != nil {
		return nil, err
	}
	select {
	case c.notify <- struct{}{}:
	default:
	}
	return &TaskInfo{ID: jobID.String()}, nil
}

func (c *MediaClient) Cancel(jobID uuid.UUID) {
	c.mu.Lock()
	cancel := c.running[jobID]
	c.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (c *MediaClient) Run(ctx context.Context) error {
	if c == nil || c.store == nil || c.execute == nil {
		return fmt.Errorf("media queue dependencies are required")
	}
	var runErr error
	c.once.Do(func() {
		interrupted, err := c.store.InterruptRunningMediaJobs(ctx)
		if err != nil {
			runErr = fmt.Errorf("recover media queue: %w", err)
			return
		}
		if interrupted > 0 {
			c.log.Warn("marked unfinished media jobs as interrupted", zap.Int64("count", interrupted))
		}
		go c.worker(ctx)
	})
	return runErr
}

func (c *MediaClient) Close() error { return nil }

func (c *MediaClient) worker(ctx context.Context) {
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
			job, err := c.store.ClaimNextMediaJob(ctx, lease)
			if errors.Is(err, db.ErrNotFound) {
				break
			}
			if err != nil {
				if ctx.Err() == nil {
					c.log.Error("claim media job", zap.Error(err))
				}
				break
			}
			jobCtx, cancel := context.WithCancel(ctx)
			c.mu.Lock()
			c.running[job.ID] = cancel
			c.mu.Unlock()
			err = c.execute(jobCtx, job.ID)
			cancel()
			c.mu.Lock()
			delete(c.running, job.ID)
			c.mu.Unlock()
			if err != nil && ctx.Err() == nil {
				c.log.Error("execute media job", zap.String("job_id", job.ID.String()), zap.Error(err))
			}
		}
		timer.Reset(poll)
	}
}
