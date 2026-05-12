package guard

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

type Runner struct {
	service  *Service
	interval time.Duration
	sleep    func(context.Context, time.Duration) bool

	mu      sync.Mutex
	running bool
}

func NewRunner(service *Service, interval time.Duration) *Runner {
	return &Runner{
		service:  service,
		interval: interval,
		sleep:    sleepContext,
	}
}

func (r *Runner) Run(ctx context.Context) error {
	if err := r.validate(); err != nil {
		return err
	}
	logrus.Info("account guard task started")
	r.setRunning(true)
	defer r.setRunning(false)

	delay := time.Duration(0)
	for {
		if !r.sleep(ctx, delay) {
			return nil
		}
		result, err := r.service.RunOnce(ctx)
		entry := logrus.WithFields(logrus.Fields{
			"checked":            result.Checked,
			"threshold_hits":     result.ThresholdHits,
			"disabled":           result.Disabled,
			"dry_run_disabled":   result.DryRunDisabled,
			"already_disabled":   result.AlreadyDisabled,
			"reenabled":          result.Reenabled,
			"cleanup_dry_run":    result.Cleanup.DryRun,
			"cleanup_candidates": len(result.Cleanup.Candidates),
			"cleanup_deleted":    len(result.Cleanup.Deleted),
		})
		if err != nil {
			entry.WithError(err).Error("account guard run failed")
		} else {
			entry.Info("account guard run finished")
		}
		delay = r.interval
	}
}

func (r *Runner) validate() error {
	if r == nil {
		return fmt.Errorf("account guard runner is nil")
	}
	if r.service == nil {
		return fmt.Errorf("account guard service is nil")
	}
	if r.interval <= 0 {
		return fmt.Errorf("account guard interval must be positive")
	}
	if r.sleep == nil {
		r.sleep = sleepContext
	}
	return nil
}

func (r *Runner) setRunning(running bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.running = running
}

func sleepContext(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
