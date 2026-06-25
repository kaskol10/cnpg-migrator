package migration

import (
	"context"
	"fmt"
	"time"

	"github.com/kaskol10/cnpg-migrator/internal/k8s"
	"github.com/kaskol10/cnpg-migrator/internal/models"
)

func (s *Service) Verification(id string) (*models.Verification, error) {
	m, err := s.store.Get(id)
	if err != nil {
		return nil, err
	}
  if m.Verification == nil {
		return &models.Verification{
			Status:  models.VerificationPending,
			Summary: models.VerificationSummary{},
		}, nil
	}
	return m.Verification, nil
}

func (s *Service) StartVerification(ctx context.Context, id string) (*models.Verification, error) {
	m, err := s.store.Get(id)
	if err != nil {
		return nil, err
	}

	if m.Status != models.StatusCompleted {
		return nil, fmt.Errorf("verification requires a completed migration")
	}

	if m.Verification != nil && m.Verification.Status == models.VerificationRunning {
		return m.Verification, nil
	}

	_ = s.k8s.DeleteVerificationJob(ctx, id)

	image, err := s.resolveImage(newerPostgresVersion(m.Options.SourceVersion, m.Options.TargetVersion))
	if err != nil {
		return nil, err
	}

	jobName := fmt.Sprintf("verify-%s", id[:8])
	now := time.Now().UTC()
	m.Verification = &models.Verification{
		Status:    models.VerificationRunning,
		JobName:   jobName,
		StartedAt: &now,
	}
	_ = s.store.Update(m)

	if err := s.k8s.CreateVerificationJob(ctx, m, image); err != nil {
		m.Verification.Status = models.VerificationFailed
		m.Verification.Error = err.Error()
		completed := time.Now().UTC()
		m.Verification.CompletedAt = &completed
		_ = s.store.Update(m)
		return m.Verification, err
	}

	s.log(id, "Verification job started")

	watchCtx, cancel := context.WithCancel(context.Background())
	s.cancel["verify:"+id] = cancel
	go s.watchVerification(watchCtx, id)

	return m.Verification, nil
}

func (s *Service) triggerVerificationIfNeeded(ctx context.Context, id string) {
	m, err := s.store.Get(id)
	if err != nil || m.Status != models.StatusCompleted {
		return
	}
	if m.Verification != nil {
		return
	}
	_, _ = s.StartVerification(ctx, id)
}

func (s *Service) watchVerification(ctx context.Context, id string) {
	ticker := time.NewTicker(time.Duration(s.cfg.PollIntervalSec) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			done, _ := s.pollVerification(ctx, id)
			if done {
				if cancel, ok := s.cancel["verify:"+id]; ok {
					cancel()
					delete(s.cancel, "verify:"+id)
				}
				return
			}
		}
	}
}

func (s *Service) pollVerification(ctx context.Context, id string) (bool, error) {
	m, err := s.store.Get(id)
	if err != nil {
		return true, err
	}
	if m.Verification == nil || m.Verification.JobName == "" {
		return true, nil
	}

	job, err := s.k8s.GetJob(ctx, m.Verification.JobName)
	if err != nil {
		return false, err
	}

	logs, _ := s.k8s.GetPodLogs(ctx, m.Verification.JobName)
	now := time.Now().UTC()

	switch {
	case job.Status.Succeeded > 0:
		result, parseErr := k8s.ParseVerificationLogs(logs)
		if parseErr != nil {
			m.Verification.Status = models.VerificationFailed
			m.Verification.Error = parseErr.Error()
			m.Verification.CompletedAt = &now
		} else {
			result.JobName = m.Verification.JobName
			result.StartedAt = m.Verification.StartedAt
			result.CompletedAt = &now
			m.Verification = result
		}
		m.UpdatedAt = now
		_ = s.store.Update(m)
		s.log(id, logs)
		return true, nil

	case job.Status.Failed > 0:
		m.Verification.Status = models.VerificationFailed
		m.Verification.Error = "verification job failed"
		m.Verification.CompletedAt = &now
		m.UpdatedAt = now
		_ = s.store.Update(m)
		if logs != "" {
			s.log(id, logs)
		}
		return true, nil
	}

	return false, nil
}
