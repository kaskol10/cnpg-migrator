package k8s

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/kaskol10/cnpg-migrator/internal/config"
	"github.com/kaskol10/cnpg-migrator/internal/models"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

type Client struct {
	clientset *kubernetes.Clientset
	namespace string
	cfg       config.Config
}

func NewClient(cfg config.Config) (*Client, error) {
	var restCfg *rest.Config
	var err error

	if cfg.InCluster {
		restCfg, err = rest.InClusterConfig()
	} else {
		kubeconfig := cfg.Kubeconfig
		if kubeconfig == "" {
			if home := homedir.HomeDir(); home != "" {
				kubeconfig = filepath.Join(home, ".kube", "config")
			}
		}
		restCfg, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
	}
	if err != nil {
		return nil, fmt.Errorf("build kubeconfig: %w", err)
	}

	cs, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return nil, fmt.Errorf("create clientset: %w", err)
	}

	return &Client{
		clientset: cs,
		namespace: cfg.Namespace,
		cfg:       cfg,
	}, nil
}

func (c *Client) Namespace() string {
	return c.namespace
}

func (c *Client) CreatePVC(ctx context.Context, name, size string) error {
	qty, err := resource.ParseQuantity(size)
	if err != nil {
		return fmt.Errorf("parse storage size: %w", err)
	}

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: c.namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":      "cnpg-migrator",
				"app.kubernetes.io/component": "migration-storage",
				"cnpg-migrator/migration":       name,
			},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: qty,
				},
			},
		},
	}

	_, err = c.clientset.CoreV1().PersistentVolumeClaims(c.namespace).Create(ctx, pvc, metav1.CreateOptions{})
	return err
}

func (c *Client) CreateMigrationJob(ctx context.Context, migration *models.Migration, sourceImage, targetImage string) error {
	dumpScript := buildDumpScript(migration)
	restoreScript := buildRestoreScript(migration)
	ttl := c.cfg.JobTTLSeconds
	backoff := int32(0)

	volumeMount := corev1.VolumeMount{
		Name:      "dump-storage",
		MountPath: "/dump",
	}

	resourceReqs := corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("500m"),
			corev1.ResourceMemory: resource.MustParse("1Gi"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("2"),
			corev1.ResourceMemory: resource.MustParse("4Gi"),
		},
	}

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      migration.JobName,
			Namespace: c.namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":      "cnpg-migrator",
				"app.kubernetes.io/component": "migration-job",
				"cnpg-migrator/migration-id":    migration.ID,
			},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:            &backoff,
			TTLSecondsAfterFinished: &ttl,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app.kubernetes.io/name":      "cnpg-migrator",
						"app.kubernetes.io/component": "migration-job",
						"cnpg-migrator/migration-id":    migration.ID,
					},
				},
				Spec: corev1.PodSpec{
					RestartPolicy:  corev1.RestartPolicyNever,
					NodeSelector: c.cfg.NodeSelector,
					Tolerations:  c.cfg.Tolerations,
					InitContainers: []corev1.Container{
						{
							Name:    "dump",
							Image:   sourceImage,
							Command: []string{"/bin/bash", "-c", dumpScript},
							Env: []corev1.EnvVar{
								{Name: "PGPASSWORD", Value: migration.Source.Password},
							},
							VolumeMounts: []corev1.VolumeMount{volumeMount},
							Resources:    resourceReqs,
						},
					},
					Containers: []corev1.Container{
						{
							Name:    "restore",
							Image:   targetImage,
							Command: []string{"/bin/bash", "-c", restoreScript},
							Env: []corev1.EnvVar{
								{Name: "TARGET_PGPASSWORD", Value: migration.Target.Password},
							},
							VolumeMounts: []corev1.VolumeMount{volumeMount},
							Resources:    resourceReqs,
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "dump-storage",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: migration.PVCName,
								},
							},
						},
					},
				},
			},
		},
	}

	_, err := c.clientset.BatchV1().Jobs(c.namespace).Create(ctx, job, metav1.CreateOptions{})
	return err
}

func (c *Client) GetJob(ctx context.Context, name string) (*batchv1.Job, error) {
	return c.clientset.BatchV1().Jobs(c.namespace).Get(ctx, name, metav1.GetOptions{})
}

func (c *Client) DeleteJob(ctx context.Context, name string) error {
	propagation := metav1.DeletePropagationBackground
	return c.clientset.BatchV1().Jobs(c.namespace).Delete(ctx, name, metav1.DeleteOptions{
		PropagationPolicy: &propagation,
	})
}

func (c *Client) DeletePVC(ctx context.Context, name string) error {
	return c.clientset.CoreV1().PersistentVolumeClaims(c.namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

func (c *Client) GetPodLogs(ctx context.Context, jobName string) (string, error) {
	pods, err := c.clientset.CoreV1().Pods(c.namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("job-name=%s", jobName),
	})
	if err != nil {
		return "", err
	}
	if len(pods.Items) == 0 {
		return "", nil
	}

	pod := pods.Items[0]
	var buf strings.Builder

	for _, ic := range pod.Spec.InitContainers {
		logs, err := c.getContainerLogs(ctx, pod.Name, ic.Name)
		if err != nil {
			continue
		}
		if logs != "" {
			buf.WriteString(fmt.Sprintf("=== %s ===\n%s\n", ic.Name, logs))
		}
	}

	for _, container := range pod.Spec.Containers {
		logs, err := c.getContainerLogs(ctx, pod.Name, container.Name)
		if err != nil {
			continue
		}
		if logs != "" {
			buf.WriteString(fmt.Sprintf("=== %s ===\n%s\n", container.Name, logs))
		}
	}

	return buf.String(), nil
}

func (c *Client) getContainerLogs(ctx context.Context, podName, containerName string) (string, error) {
	req := c.clientset.CoreV1().Pods(c.namespace).GetLogs(podName, &corev1.PodLogOptions{
		Container: containerName,
	})
	stream, err := req.Stream(ctx)
	if err != nil {
		return "", err
	}
	defer stream.Close()

	data, err := io.ReadAll(stream)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func buildDumpScript(m *models.Migration) string {
	if m.Options.AllDatabases {
		return buildAllDatabasesDumpScript(m)
	}
	return buildSingleDatabaseDumpScript(m)
}

func buildRestoreScript(m *models.Migration) string {
	if m.Options.AllDatabases {
		return buildAllDatabasesRestoreScript(m)
	}
	return buildSingleDatabaseRestoreScript(m)
}

func buildSingleDatabaseDumpScript(m *models.Migration) string {
	src := m.Source
	opts := m.Options

	format := opts.Format
	if format == "" {
		format = "custom"
	}
	sslMode := src.SSLMode
	if sslMode == "" {
		sslMode = "require"
	}

	dumpFlags := fmt.Sprintf("--format=%s --no-owner --no-acl -h %s -p %d -U %s -d %s",
		format, src.Host, src.Port, src.Username, src.Database)
	if opts.SchemaOnly {
		dumpFlags += " --schema-only"
	}
	if opts.DataOnly {
		dumpFlags += " --data-only"
	}

	dumpFile := dumpFilePath(format, "")

	return fmt.Sprintf(`set -euo pipefail

export PGPASSWORD
export PGSSLMODE=%s

echo "=== Phase: dump ==="
echo "Dumping from RDS %s:%d/%s (PostgreSQL %s) ..."
pg_dump %s -f %s
echo "Dump completed."
`, sslMode, src.Host, src.Port, src.Database, opts.SourceVersion, dumpFlags, dumpFile)
}

func buildAllDatabasesDumpScript(m *models.Migration) string {
	src := m.Source
	opts := m.Options

	format := opts.Format
	if format == "" {
		format = "custom"
	}
	sslMode := src.SSLMode
	if sslMode == "" {
		sslMode = "require"
	}

	exclude := opts.ExcludeDatabases
	if exclude == "" {
		exclude = "rdsadmin"
	}

	extraFlags := ""
	if opts.SchemaOnly {
		extraFlags += " --schema-only"
	}
	if opts.DataOnly {
		extraFlags += " --data-only"
	}

	dumpPathScript := allDatabasesDumpPathScript(format)

	return fmt.Sprintf(`set -euo pipefail

export PGPASSWORD
export PGSSLMODE=%s

mkdir -p /dump/databases

echo "=== Phase: dump ==="
echo "Listing databases on RDS %s:%d (PostgreSQL %s) ..."

DATABASES=$(psql -h %s -p %d -U %s -d postgres -Atc "SELECT datname FROM pg_database WHERE datistemplate = false ORDER BY datname")

EXCLUDE="%s"
DUMPED=0

for db in $DATABASES; do
  if echo ",${EXCLUDE}," | grep -q ",${db},"; then
    echo "Skipping excluded database: $db"
    continue
  fi

  dump_file=%s
  echo "Dumping database: $db -> $dump_file"
  pg_dump --format=%s --no-owner --no-acl -h %s -p %d -U %s -d "$db"%s -f "$dump_file"
  DUMPED=$((DUMPED + 1))
done

if [ "$DUMPED" -eq 0 ]; then
  echo "ERROR: no databases were dumped"
  exit 1
fi

echo "Dump completed ($DUMPED databases)."
`, sslMode,
		src.Host, src.Port, opts.SourceVersion,
		src.Host, src.Port, src.Username,
		exclude,
		dumpPathScript,
		format, src.Host, src.Port, src.Username, extraFlags)
}

func buildSingleDatabaseRestoreScript(m *models.Migration) string {
	tgt := m.Target
	opts := m.Options

	format := opts.Format
	if format == "" {
		format = "custom"
	}
	jobs := opts.Jobs
	if jobs <= 0 {
		jobs = 4
	}

	dumpFile := dumpFilePath(format, "")
	targetHost := resolveTargetHost(tgt)
	targetPort := resolveTargetPort(tgt)
	username := tgt.Username
	if username == "" {
		username = "postgres"
	}

	restoreCmd := buildRestoreCommand(format, targetHost, targetPort, username, tgt.Database, dumpFile, jobs, opts.CleanBeforeRestore)
	restoreClient := resolveRestoreClientVersion(opts)

	return fmt.Sprintf(`set -euo pipefail

export PGPASSWORD="$TARGET_PGPASSWORD"

echo "=== Phase: restore ==="
echo "Restoring to CNPG %s:%d (server PostgreSQL %s, pg_restore client %s) ..."
%s
echo "Restore completed."

echo "=== Migration finished successfully ==="
`, targetHost, targetPort, opts.TargetVersion, restoreClient, restoreCmd)
}

func buildAllDatabasesRestoreScript(m *models.Migration) string {
	tgt := m.Target
	opts := m.Options

	format := opts.Format
	if format == "" {
		format = "custom"
	}
	jobs := opts.Jobs
	if jobs <= 0 {
		jobs = 4
	}

	targetHost := resolveTargetHost(tgt)
	targetPort := resolveTargetPort(tgt)
	username := tgt.Username
	if username == "" {
		username = "postgres"
	}

	cleanFlags := ""
	if opts.CleanBeforeRestore {
		cleanFlags = " --clean --if-exists"
	}

	restoreLoop := allDatabasesRestoreLoop(format, targetHost, targetPort, username, jobs, cleanFlags)
	restoreClient := resolveRestoreClientVersion(opts)

	return fmt.Sprintf(`set -euo pipefail

export PGPASSWORD="$TARGET_PGPASSWORD"

echo "=== Phase: restore ==="
echo "Restoring all databases to CNPG %s:%d (server PostgreSQL %s, pg_restore client %s) ..."

%s

echo "=== Migration finished successfully ==="
`, targetHost, targetPort, opts.TargetVersion, restoreClient, restoreLoop)
}

func resolveTargetHost(tgt models.TargetConfig) string {
	if tgt.Host != "" {
		return tgt.Host
	}
	return fmt.Sprintf("%s-rw.%s.svc.cluster.local", tgt.Cluster, tgt.Namespace)
}

func resolveTargetPort(tgt models.TargetConfig) int {
	if tgt.Port == 0 {
		return 5432
	}
	return tgt.Port
}

func resolveRestoreClientVersion(opts models.MigrationOptions) string {
	if opts.RestoreClientVersion != "" {
		return opts.RestoreClientVersion
	}
	sv, _ := strconv.Atoi(opts.SourceVersion)
	tv, _ := strconv.Atoi(opts.TargetVersion)
	if sv >= tv {
		return opts.SourceVersion
	}
	return opts.TargetVersion
}

func pgConnFlags(host string, port int, user string) string {
	return fmt.Sprintf("-h %s -p %d -U %s", host, port, user)
}

func dumpFilePath(format, database string) string {
	if database == "" {
		switch format {
		case "directory":
			return "/dump/migration_dir"
		case "plain":
			return "/dump/migration.sql"
		default:
			return "/dump/migration.dump"
		}
	}

	switch format {
	case "directory":
		return fmt.Sprintf("/dump/databases/%s", database)
	case "plain":
		return fmt.Sprintf("/dump/databases/%s.sql", database)
	default:
		return fmt.Sprintf("/dump/databases/%s.dump", database)
	}
}

func allDatabasesDumpPathScript(format string) string {
	switch format {
	case "directory":
		return `"/dump/databases/${db}"`
	case "plain":
		return `"/dump/databases/${db}.sql"`
	default:
		return `"/dump/databases/${db}.dump"`
	}
}

func allDatabasesRestoreLoop(format, host string, port int, user string, jobs int, cleanFlags string) string {
	conn := pgConnFlags(host, port, user)
	switch format {
	case "plain":
		return fmt.Sprintf(`RESTORED=0
for dump_file in /dump/databases/*.sql; do
  [ -f "$dump_file" ] || continue
  db=$(basename "$dump_file" .sql)
  echo "Restoring database: $db"
  psql %s -d postgres -tc "SELECT 1 FROM pg_database WHERE datname='$db'" | grep -q 1 || \
    psql %s -d postgres -c "CREATE DATABASE \"$db\""
  psql %s -d "$db" -f "$dump_file"
  RESTORED=$((RESTORED + 1))
done

if [ "$RESTORED" -eq 0 ]; then
  echo "ERROR: no databases were restored"
  exit 1
fi

echo "Restore completed ($RESTORED databases)."`, conn, conn, conn)
	case "directory":
		return fmt.Sprintf(`RESTORED=0
for dump_dir in /dump/databases/*/; do
  [ -d "$dump_dir" ] || continue
  db=$(basename "$dump_dir")
  echo "Restoring database: $db"
  psql %s -d postgres -tc "SELECT 1 FROM pg_database WHERE datname='$db'" | grep -q 1 || \
    psql %s -d postgres -c "CREATE DATABASE \"$db\""
  pg_restore %s -d "$db"%s --no-owner --no-acl -j %d "$dump_dir"
  RESTORED=$((RESTORED + 1))
done

if [ "$RESTORED" -eq 0 ]; then
  echo "ERROR: no databases were restored"
  exit 1
fi

echo "Restore completed ($RESTORED databases)."`, conn, conn, conn, cleanFlags, jobs)
	default:
		return fmt.Sprintf(`RESTORED=0
for dump_file in /dump/databases/*.dump; do
  [ -f "$dump_file" ] || continue
  db=$(basename "$dump_file" .dump)
  echo "Restoring database: $db"
  psql %s -d postgres -tc "SELECT 1 FROM pg_database WHERE datname='$db'" | grep -q 1 || \
    psql %s -d postgres -c "CREATE DATABASE \"$db\""
  pg_restore %s -d "$db"%s --no-owner --no-acl -j %d "$dump_file"
  RESTORED=$((RESTORED + 1))
done

if [ "$RESTORED" -eq 0 ]; then
  echo "ERROR: no databases were restored"
  exit 1
fi

echo "Restore completed ($RESTORED databases)."`, conn, conn, conn, cleanFlags, jobs)
	}
}

func buildRestoreCommand(format, host string, port int, user, database, dumpFile string, jobs int, clean bool) string {
	conn := pgConnFlags(host, port, user)
	switch format {
	case "custom", "directory":
		flags := fmt.Sprintf("%s -d %s", conn, database)
		if clean {
			flags += " --clean --if-exists"
		}
		return fmt.Sprintf(`pg_restore %s --no-owner --no-acl -j %d %s`, flags, jobs, dumpFile)
	case "plain":
		return fmt.Sprintf(`psql %s -d %s -f %s`, conn, database, dumpFile)
	default:
		return fmt.Sprintf(`pg_restore %s -d %s --no-owner --no-acl -j %d %s`,
			conn, database, jobs, dumpFile)
	}
}

