package migration

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kaskol10/cnpg-migrator/internal/config"
	"github.com/kaskol10/cnpg-migrator/internal/k8s"
	"github.com/kaskol10/cnpg-migrator/internal/models"
	"github.com/kaskol10/cnpg-migrator/internal/store"
	batchv1 "k8s.io/api/batch/v1"
)

type Service struct {
	store  *store.Store
	k8s    *k8s.Client
	cfg    config.Config
	cancel map[string]context.CancelFunc
}

func NewService(st *store.Store, k8sClient *k8s.Client, cfg config.Config) *Service {
	return &Service{
		store:  st,
		k8s:    k8sClient,
		cfg:    cfg,
		cancel: make(map[string]context.CancelFunc),
	}
}

func (s *Service) Create(ctx context.Context, req models.CreateMigrationRequest) (*models.Migration, error) {
	if req.Source.Port == 0 {
		req.Source.Port = 5432
	}
	if req.Target.Username == "" {
		req.Target.Username = "postgres"
	}
	if req.Target.Port == 0 {
		req.Target.Port = 5432
	}
	if req.Options.SourceVersion == "" {
		req.Options.SourceVersion = s.cfg.DefaultSourceVersion
	}
	if req.Options.TargetVersion == "" {
		req.Options.TargetVersion = s.cfg.DefaultTargetVersion
	}
	if err := validateRequest(req, s.cfg); err != nil {
		return nil, err
	}

	sourceImage, err := s.resolveImage(req.Options.SourceVersion)
	if err != nil {
		return nil, err
	}
	restoreClientVersion := newerPostgresVersion(req.Options.SourceVersion, req.Options.TargetVersion)
	restoreImage, err := s.resolveImage(restoreClientVersion)
	if err != nil {
		return nil, err
	}
	req.Options.RestoreClientVersion = restoreClientVersion

	id := uuid.New().String()
	now := time.Now().UTC()
	storageSize := req.Options.StorageSize
	if storageSize == "" {
		storageSize = s.cfg.DefaultStorageSize
	}

	m := &models.Migration{
		ID:        id,
		Name:      req.Name,
		Status:    models.StatusPending,
		Source:    req.Source,
		Target:    req.Target,
		Options:   req.Options,
		JobName:   fmt.Sprintf("migration-%s", id[:8]),
		PVCName:   fmt.Sprintf("migration-dump-%s", id[:8]),
		CreatedAt: now,
		UpdatedAt: now,
	}
	m.Options.StorageSize = storageSize

	s.store.Create(m)
	s.log(m.ID, "Migration created")

	if err := s.k8s.CreatePVC(ctx, m.PVCName, storageSize); err != nil {
		m.Status = models.StatusFailed
		m.Error = err.Error()
		m.UpdatedAt = time.Now().UTC()
		_ = s.store.Update(m)
		return m, fmt.Errorf("create PVC: %w", err)
	}
	s.log(m.ID, fmt.Sprintf("PVC %s created (%s)", m.PVCName, storageSize))
	s.log(m.ID, fmt.Sprintf("Using PostgreSQL %s for dump, PostgreSQL %s client for restore (CNPG server: %s)",
		m.Options.SourceVersion, restoreClientVersion, m.Options.TargetVersion))
	if postgresMajorVersion(m.Options.SourceVersion) > postgresMajorVersion(m.Options.TargetVersion) {
		s.log(m.ID, fmt.Sprintf("WARNING: RDS PostgreSQL %s is newer than CNPG %s; downgrade migrations may fail",
			m.Options.SourceVersion, m.Options.TargetVersion))
	}

	if err := s.k8s.CreateMigrationJob(ctx, m, sourceImage, restoreImage); err != nil {
		m.Status = models.StatusFailed
		m.Error = err.Error()
		m.UpdatedAt = time.Now().UTC()
		_ = s.store.Update(m)
		return m, fmt.Errorf("create job: %w", err)
	}

	started := time.Now().UTC()
	m.Status = models.StatusDumping
	m.StartedAt = &started
	m.UpdatedAt = started
	_ = s.store.Update(m)
	s.log(m.ID, fmt.Sprintf("Job %s started", m.JobName))

	watchCtx, cancel := context.WithCancel(context.Background())
	s.cancel[id] = cancel
	go s.watch(watchCtx, m.ID)

	return m, nil
}

func (s *Service) Get(id string) (*models.Migration, error) {
	return s.store.Get(id)
}

func (s *Service) List() []*models.Migration {
	return s.store.List()
}

func (s *Service) Logs(id string) []models.MigrationLog {
	return s.store.GetLogs(id)
}

func (s *Service) Cancel(ctx context.Context, id string) (*models.Migration, error) {
	m, err := s.store.Get(id)
	if err != nil {
		return nil, err
	}

	if m.Status == models.StatusCompleted || m.Status == models.StatusFailed || m.Status == models.StatusCancelled {
		return m, nil
	}

	if cancel, ok := s.cancel[id]; ok {
		cancel()
		delete(s.cancel, id)
	}

	if m.JobName != "" {
		_ = s.k8s.DeleteJob(ctx, m.JobName)
	}

	now := time.Now().UTC()
	m.Status = models.StatusCancelled
	m.UpdatedAt = now
	m.CompletedAt = &now
	_ = s.store.Update(m)
	s.log(id, "Migration cancelled")

	return m, nil
}

func (s *Service) Cleanup(ctx context.Context, id string) error {
	m, err := s.store.Get(id)
	if err != nil {
		return err
	}

	if m.JobName != "" {
		_ = s.k8s.DeleteJob(ctx, m.JobName)
	}
	_ = s.k8s.DeleteVerificationJob(ctx, id)
	if m.PVCName != "" {
		_ = s.k8s.DeletePVC(ctx, m.PVCName)
	}
	s.log(id, "Resources cleaned up")
	return nil
}

func (s *Service) watch(ctx context.Context, id string) {
	ticker := time.NewTicker(time.Duration(s.cfg.PollIntervalSec) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.poll(ctx, id); err != nil {
				continue
			}
			m, _ := s.store.Get(id)
			if m != nil && (m.Status == models.StatusCompleted || m.Status == models.StatusFailed || m.Status == models.StatusCancelled) {
				return
			}
		}
	}
}

func (s *Service) poll(ctx context.Context, id string) error {
	m, err := s.store.Get(id)
	if err != nil {
		return err
	}
	if m.JobName == "" {
		return nil
	}

	job, err := s.k8s.GetJob(ctx, m.JobName)
	if err != nil {
		return err
	}

	logs, _ := s.k8s.GetPodLogs(ctx, m.JobName)
	phase := inferPhase(logs)
	now := time.Now().UTC()

	switch {
	case job.Status.Succeeded > 0:
		wasCompleted := m.Status == models.StatusCompleted
		m.Status = models.StatusCompleted
		m.Phase = "completed"
		m.CompletedAt = &now
		m.Error = ""
		m.UpdatedAt = now
		_ = s.store.Update(m)
		if logs != "" {
			s.log(id, logs)
		}
		if !wasCompleted {
			go s.triggerVerificationIfNeeded(context.Background(), id)
		}
		return nil
	case job.Status.Failed > 0:
		m.Status = models.StatusFailed
		m.Phase = "failed"
		m.CompletedAt = &now
		if logs != "" {
			lines := strings.Split(strings.TrimSpace(logs), "\n")
			if len(lines) > 0 {
				m.Error = lines[len(lines)-1]
			}
		}
	default:
		if phase == "restore" {
			m.Status = models.StatusRestoring
		} else {
			m.Status = models.StatusDumping
		}
		m.Phase = phase
	}

	m.UpdatedAt = now
	_ = s.store.Update(m)

	if logs != "" {
		s.log(id, logs)
	}

	return nil
}

func inferPhase(logs string) string {
	if strings.Contains(logs, "Phase: restore") {
		return "restore"
	}
	if strings.Contains(logs, "Phase: dump") {
		return "dump"
	}
	return "pending"
}

func (s *Service) log(id, message string) {
	s.store.AppendLog(id, models.MigrationLog{
		Timestamp: time.Now().UTC(),
		Message:   message,
	})
}

func (s *Service) Config() map[string]any {
	return s.cfg.PublicConfig()
}

func (s *Service) resolveImage(version string) (string, error) {
	image, ok := s.cfg.ImageForVersion(version)
	if !ok {
		return "", fmt.Errorf("unsupported PostgreSQL version: %s", version)
	}
	return image, nil
}

func newerPostgresVersion(a, b string) string {
	if postgresMajorVersion(a) >= postgresMajorVersion(b) {
		return a
	}
	return b
}

func postgresMajorVersion(v string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(v))
	return n
}

func validateRequest(req models.CreateMigrationRequest, cfg config.Config) error {
	if req.Name == "" {
		return fmt.Errorf("name is required")
	}
	if req.Source.Host == "" || req.Source.Username == "" {
		return fmt.Errorf("source host and username are required")
	}
	if !req.Options.AllDatabases && req.Source.Database == "" {
		return fmt.Errorf("source database is required when not migrating all databases")
	}
	if req.Target.Cluster == "" || req.Target.Namespace == "" {
		return fmt.Errorf("target cluster and namespace are required")
	}
	if !req.Options.AllDatabases && req.Target.Database == "" {
		return fmt.Errorf("target database is required when not migrating all databases")
	}
	if _, ok := cfg.ImageForVersion(req.Options.SourceVersion); !ok {
		return fmt.Errorf("unsupported source PostgreSQL version: %s", req.Options.SourceVersion)
	}
	if _, ok := cfg.ImageForVersion(req.Options.TargetVersion); !ok {
		return fmt.Errorf("unsupported target PostgreSQL version: %s", req.Options.TargetVersion)
	}
	return nil
}

// JobStatus returns raw k8s job status for debugging.
func (s *Service) JobStatus(ctx context.Context, id string) (*batchv1.JobStatus, error) {
	m, err := s.store.Get(id)
	if err != nil {
		return nil, err
	}
	job, err := s.k8s.GetJob(ctx, m.JobName)
	if err != nil {
		return nil, err
	}
	return &job.Status, nil
}
