package k8s

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/kaskol10/cnpg-migrator/internal/models"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (c *Client) CreateVerificationJob(ctx context.Context, migration *models.Migration, image string) error {
	script := buildVerificationScript(migration)
	ttl := c.cfg.JobTTLSeconds
	backoff := int32(0)

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      verificationJobName(migration.ID),
			Namespace: c.namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":      "cnpg-migrator",
				"app.kubernetes.io/component": "verification-job",
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
						"app.kubernetes.io/component": "verification-job",
						"cnpg-migrator/migration-id":    migration.ID,
					},
				},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					NodeSelector:  c.cfg.NodeSelector,
					Tolerations:   c.cfg.Tolerations,
					Containers: []corev1.Container{
						{
							Name:    "verify",
							Image:   image,
							Command: []string{"/bin/bash", "-c", script},
							Env: []corev1.EnvVar{
								{Name: "PGPASSWORD", Value: migration.Source.Password},
								{Name: "TARGET_PGPASSWORD", Value: migration.Target.Password},
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("100m"),
									corev1.ResourceMemory: resource.MustParse("256Mi"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("500m"),
									corev1.ResourceMemory: resource.MustParse("512Mi"),
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

func (c *Client) DeleteVerificationJob(ctx context.Context, migrationID string) error {
	propagation := metav1.DeletePropagationBackground
	return c.clientset.BatchV1().Jobs(c.namespace).Delete(ctx, verificationJobName(migrationID), metav1.DeleteOptions{
		PropagationPolicy: &propagation,
	})
}

func verificationJobName(migrationID string) string {
	return fmt.Sprintf("verify-%s", migrationID[:8])
}

func buildVerificationScript(m *models.Migration) string {
	src := m.Source
	tgt := m.Target
	opts := m.Options

	sslMode := src.SSLMode
	if sslMode == "" {
		sslMode = "require"
	}

	targetHost := resolveTargetHost(tgt)
	targetPort := resolveTargetPort(tgt)
	targetUser := tgt.Username
	if targetUser == "" {
		targetUser = "postgres"
	}

	exclude := opts.ExcludeDatabases
	if exclude == "" {
		exclude = "rdsadmin"
	}

	if opts.AllDatabases {
		return fmt.Sprintf(`set -euo pipefail

export PGPASSWORD
export TARGET_PGPASSWORD
export PGSSLMODE=%s

echo "=== Phase: verification ==="

SRC_FILE=$(mktemp)
TGT_FILE=$(mktemp)

psql -h %s -p %d -U %s -d postgres -Atc \
  "SELECT datname || '|' || pg_database_size(datname) FROM pg_database WHERE datistemplate = false ORDER BY datname" \
  > "$SRC_FILE"

PGPASSWORD="$TARGET_PGPASSWORD" psql -h %s -p %d -U %s -d postgres -Atc \
  "SELECT datname || '|' || pg_database_size(datname) FROM pg_database WHERE datistemplate = false ORDER BY datname" \
  > "$TGT_FILE"

EXCLUDE="%s"
MATCHED=0
MISSING=0
MISMATCH=0
TOTAL=0

while IFS='|' read -r db src_size; do
  if echo ",${EXCLUDE}," | grep -q ",${db},"; then
    continue
  fi

  TOTAL=$((TOTAL + 1))
  tgt_line=$(grep "^${db}|" "$TGT_FILE" || true)
  if [ -z "$tgt_line" ]; then
    tgt_size=0
    status="missing"
    MISSING=$((MISSING + 1))
    pct=100
  else
    tgt_size=$(echo "$tgt_line" | cut -d'|' -f2)
    status="ok"
    if [ "$src_size" -gt 0 ]; then
      diff=$((src_size - tgt_size))
      if [ "$diff" -lt 0 ]; then diff=$((-diff)); fi
      pct=$((diff * 100 / src_size))
      if [ "$pct" -gt 10 ]; then
        status="size_mismatch"
        MISMATCH=$((MISMATCH + 1))
      else
        MATCHED=$((MATCHED + 1))
      fi
    else
      pct=0
      MATCHED=$((MATCHED + 1))
    fi
  fi

  echo "DB|${db}|${src_size}|${tgt_size}|${status}|${pct}"
done < "$SRC_FILE"

PASSED=false
if [ "$MISSING" -eq 0 ] && [ "$MISMATCH" -eq 0 ] && [ "$TOTAL" -gt 0 ]; then
  PASSED=true
fi

echo "SUMMARY|${TOTAL}|${MATCHED}|${MISSING}|${MISMATCH}|${PASSED}"
echo "=== Verification finished ==="
`, sslMode,
			src.Host, src.Port, src.Username,
			targetHost, targetPort, targetUser,
			exclude)
	}

	db := src.Database
	if tgt.Database != "" {
		db = src.Database
	}

	return fmt.Sprintf(`set -euo pipefail

export PGPASSWORD
export TARGET_PGPASSWORD
export PGSSLMODE=%s

echo "=== Phase: verification ==="

db="%s"
src_size=$(psql -h %s -p %d -U %s -d postgres -Atc "SELECT pg_database_size('${db}')")
tgt_size=$(PGPASSWORD="$TARGET_PGPASSWORD" psql -h %s -p %d -U %s -d postgres -Atc "SELECT pg_database_size('${db}')" 2>/dev/null || echo "0")

status="ok"
if [ "$tgt_size" = "0" ] || [ -z "$tgt_size" ]; then
  status="missing"
  tgt_size=0
  pct=100
elif [ "$src_size" -gt 0 ]; then
  diff=$((src_size - tgt_size))
  if [ "$diff" -lt 0 ]; then diff=$((-diff)); fi
  pct=$((diff * 100 / src_size))
  if [ "$pct" -gt 10 ]; then
    status="size_mismatch"
  fi
else
  pct=0
fi

echo "DB|${db}|${src_size}|${tgt_size}|${status}|${pct}"

MATCHED=0
MISSING=0
MISMATCH=0
if [ "$status" = "ok" ]; then MATCHED=1
elif [ "$status" = "missing" ]; then MISSING=1
else MISMATCH=1; fi

PASSED=false
if [ "$status" = "ok" ]; then PASSED=true; fi

echo "SUMMARY|1|${MATCHED}|${MISSING}|${MISMATCH}|${PASSED}"
echo "=== Verification finished ==="
`, sslMode,
		db,
		src.Host, src.Port, src.Username,
		targetHost, targetPort, targetUser)
}

func ParseVerificationLogs(logs string) (*models.Verification, error) {
	v := &models.Verification{
		Status:    models.VerificationFailed,
		Databases: []models.DatabaseComparison{},
	}

	for _, line := range strings.Split(logs, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "DB|") {
			parts := strings.Split(line, "|")
			if len(parts) < 6 {
				continue
			}
			srcSize, _ := strconv.ParseInt(parts[2], 10, 64)
			tgtSize, _ := strconv.ParseInt(parts[3], 10, 64)
			pct, _ := strconv.ParseFloat(parts[5], 64)

			v.Databases = append(v.Databases, models.DatabaseComparison{
				Database:        parts[1],
				SourceSizeBytes: srcSize,
				TargetSizeBytes: tgtSize,
				TargetExists:    parts[4] != "missing",
				SizeMatch:       parts[4] == "ok",
				SizeDiffPercent: pct,
				Status:          parts[4],
			})
		}
		if strings.HasPrefix(line, "SUMMARY|") {
			parts := strings.Split(line, "|")
			if len(parts) < 6 {
				continue
			}
			v.Summary.TotalDatabases, _ = strconv.Atoi(parts[1])
			v.Summary.Matched, _ = strconv.Atoi(parts[2])
			v.Summary.Missing, _ = strconv.Atoi(parts[3])
			v.Summary.SizeMismatch, _ = strconv.Atoi(parts[4])
			v.Summary.Passed = parts[5] == "true"
		}
	}

	if v.Summary.TotalDatabases == 0 && len(v.Databases) == 0 {
		return nil, fmt.Errorf("no verification results in job logs")
	}

	if v.Summary.Passed {
		v.Status = models.VerificationPassed
	} else {
		v.Status = models.VerificationFailed
	}

	return v, nil
}
