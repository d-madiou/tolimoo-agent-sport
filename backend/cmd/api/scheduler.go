package main

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"
)

const schedulerTick = 5 * time.Second

// scheduler only decides when an assignment may enter the existing queue. It
// never calls providers or runs research itself.
type scheduler struct {
	repo   repository
	queue  *runQueue
	log    *slog.Logger
	now    func() time.Time
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func newScheduler(parent context.Context, repo repository, queue *runQueue, log *slog.Logger) (*scheduler, error) {
	ctx, cancel := context.WithCancel(parent)
	s := &scheduler{repo: repo, queue: queue, log: log, now: time.Now, ctx: ctx, cancel: cancel}
	if err := s.repo.ensureSchedules(s.now()); err != nil {
		cancel()
		return nil, err
	}
	queue.enableScheduling()
	s.wg.Add(1)
	go s.loop()
	return s, nil
}

func (s *scheduler) loop() {
	defer s.wg.Done()
	ticker := time.NewTicker(schedulerTick)
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.runOnce(s.now())
		}
	}
}

func (s *scheduler) runOnce(now time.Time) {
	if err := s.repo.ensureSchedules(now); err != nil {
		s.log.Error("prepare research schedules", "error", err)
		return
	}
	agentIDs, err := s.repo.dueScheduledAgents(now)
	if err != nil {
		s.log.Error("read due research schedules", "error", err)
		return
	}
	for _, agentID := range agentIDs {
		if s.ctx.Err() != nil {
			return
		}
		_, err := s.queue.enqueue(agentID)
		switch {
		case err == nil:
			s.log.Info("scheduled research run", "agent_id", agentID)
		case errors.Is(err, errActiveRun):
			s.log.Info("skipped scheduled research run with active assignment", "agent_id", agentID)
		case errors.Is(err, errQueueFull):
			s.log.Info("skipped scheduled research run with full queue", "agent_id", agentID)
		case errors.Is(err, errNotConfigured):
			s.log.Error("scheduled research is not configured", "agent_id", agentID)
		default:
			s.log.Error("schedule research run failed", "agent_id", agentID, "error", err)
		}
	}
}

func (s *scheduler) stop() {
	s.cancel()
	s.wg.Wait()
}
