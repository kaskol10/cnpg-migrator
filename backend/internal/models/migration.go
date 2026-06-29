package models

import "time"

type MigrationStatus string

const (
	StatusPending   MigrationStatus = "pending"
	StatusDumping   MigrationStatus = "dumping"
	StatusRestoring MigrationStatus = "restoring"
	StatusCompleted MigrationStatus = "completed"
	StatusFailed    MigrationStatus = "failed"
	StatusCancelled MigrationStatus = "cancelled"
)

type SourceConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Database string `json:"database"`
	Username string `json:"username"`
	Password string `json:"password"`
	SSLMode  string `json:"ssl_mode"`
}

type TargetConfig struct {
	Namespace string `json:"namespace"`
	Cluster   string `json:"cluster"`            // CNPG cluster resource name
	Host      string `json:"host,omitempty"`     // optional override, e.g. backstage-postgresql-rw.backstage.svc.cluster.local
	Port      int    `json:"port"`               // default 5432
	Database  string `json:"database"`
	Username  string `json:"username"`
	Password  string `json:"password"`
}

type MigrationOptions struct {
	Format             string `json:"format"` // custom, directory, plain
	Jobs               int    `json:"jobs"`   // parallel restore jobs
	SchemaOnly         bool   `json:"schema_only"`
	DataOnly           bool   `json:"data_only"`
	CleanBeforeRestore bool   `json:"clean_before_restore"`
	PreserveOwnership  bool   `json:"preserve_ownership"`  // keep object owners and ACLs from source
	MigrateRoles       bool   `json:"migrate_roles"`       // dump/apply roles before restore (requires preserve_ownership)
	AllDatabases       bool   `json:"all_databases"`       // dump/restore all user databases
	ExcludeDatabases   string `json:"exclude_databases"`   // comma-separated names to skip
	StorageSize          string `json:"storage_size"`                    // PVC size, e.g. "50Gi"
	SourceVersion        string `json:"source_version"`                  // PostgreSQL major version for pg_dump
	TargetVersion        string `json:"target_version"`                  // PostgreSQL major version on CNPG server
	RestoreClientVersion string `json:"restore_client_version,omitempty"` // pg_restore client version (computed)
}

type CreateMigrationRequest struct {
	Name    string           `json:"name"`
	Source  SourceConfig     `json:"source"`
	Target  TargetConfig     `json:"target"`
	Options MigrationOptions `json:"options"`
}

type Migration struct {
	ID        string           `json:"id"`
	Name      string           `json:"name"`
	Status    MigrationStatus  `json:"status"`
	Source    SourceConfig     `json:"source"`
	Target    TargetConfig     `json:"target"`
	Options   MigrationOptions `json:"options"`
	JobName   string           `json:"job_name,omitempty"`
	PVCName   string           `json:"pvc_name,omitempty"`
	Error     string           `json:"error,omitempty"`
	Phase     string           `json:"phase,omitempty"`
	CreatedAt time.Time        `json:"created_at"`
	UpdatedAt time.Time        `json:"updated_at"`
	StartedAt *time.Time       `json:"started_at,omitempty"`
	CompletedAt *time.Time     `json:"completed_at,omitempty"`
	Verification *Verification `json:"verification,omitempty"`
}

type MigrationLog struct {
	Timestamp time.Time `json:"timestamp"`
	Message   string    `json:"message"`
}
