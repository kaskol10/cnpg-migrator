package store

import (
	"errors"
	"sync"

	"github.com/kaskol10/cnpg-migrator/internal/models"
)

var ErrNotFound = errors.New("migration not found")

type Store struct {
	mu         sync.RWMutex
	migrations map[string]*models.Migration
	logs       map[string][]models.MigrationLog
}

func New() *Store {
	return &Store{
		migrations: make(map[string]*models.Migration),
		logs:       make(map[string][]models.MigrationLog),
	}
}

func (s *Store) Create(m *models.Migration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.migrations[m.ID] = m
	s.logs[m.ID] = []models.MigrationLog{}
}

func (s *Store) Get(id string) (*models.Migration, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m, ok := s.migrations[id]
	if !ok {
		return nil, ErrNotFound
	}
	return m, nil
}

func (s *Store) List() []*models.Migration {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*models.Migration, 0, len(s.migrations))
	for _, m := range s.migrations {
		result = append(result, m)
	}
	return result
}

func (s *Store) Update(m *models.Migration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.migrations[m.ID]; !ok {
		return ErrNotFound
	}
	s.migrations[m.ID] = m
	return nil
}

func (s *Store) AppendLog(id string, log models.MigrationLog) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.logs[id] = append(s.logs[id], log)
}

func (s *Store) GetLogs(id string) []models.MigrationLog {
	s.mu.RLock()
	defer s.mu.RUnlock()
	logs := s.logs[id]
	if logs == nil {
		return []models.MigrationLog{}
	}
	result := make([]models.MigrationLog, len(logs))
	copy(result, logs)
	return result
}
