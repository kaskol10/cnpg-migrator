package config

import (
	"os"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
)

type PostgresVersion struct {
	Version string `json:"version"`
	Image   string `json:"image"`
}

type Config struct {
	Addr                 string
	Namespace            string
	InCluster            bool
	Kubeconfig           string
	PostgresVersions     []PostgresVersion
	DefaultSourceVersion string
	DefaultTargetVersion string
	NodeSelector         map[string]string
	Tolerations          []corev1.Toleration
	PollIntervalSec      int
	JobTTLSeconds        int32
	DefaultStorageSize   string
}

func Load() Config {
	inCluster := os.Getenv("IN_CLUSTER") == "true"
	pollInterval := 5
	if v := os.Getenv("POLL_INTERVAL_SEC"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			pollInterval = n
		}
	}

	versions := parsePostgresVersions(
		os.Getenv("POSTGRES_VERSIONS"),
		os.Getenv("POSTGRES_IMAGES"),
		envOr("POSTGRES_IMAGE_PREFIX", "postgres:"),
	)
	defaultVersion := "16"
	if len(versions) > 0 {
		defaultVersion = versions[len(versions)-1].Version
	}

	return Config{
		Addr:                 envOr("ADDR", ":8080"),
		Namespace:            envOr("NAMESPACE", "cnpg-migrator"),
		InCluster:            inCluster,
		Kubeconfig:           os.Getenv("KUBECONFIG"),
		PostgresVersions:     versions,
		DefaultSourceVersion: envOr("DEFAULT_SOURCE_VERSION", defaultVersion),
		DefaultTargetVersion: envOr("DEFAULT_TARGET_VERSION", defaultVersion),
		NodeSelector:         parseNodeSelector(os.Getenv("NODE_SELECTOR")),
		Tolerations:          parseTolerations(os.Getenv("TOLERATIONS")),
		PollIntervalSec:      pollInterval,
		JobTTLSeconds:        3600,
		DefaultStorageSize:   envOr("DEFAULT_STORAGE_SIZE", "50Gi"),
	}
}

func (c Config) ImageForVersion(version string) (string, bool) {
	for _, v := range c.PostgresVersions {
		if v.Version == version {
			return v.Image, true
		}
	}
	return "", false
}

func (c Config) PublicConfig() map[string]any {
	return map[string]any{
		"postgres_versions":      c.PostgresVersions,
		"default_source_version": c.DefaultSourceVersion,
		"default_target_version": c.DefaultTargetVersion,
	}
}

// parsePostgresVersions builds the version list from:
//   - POSTGRES_IMAGES: "13=postgres:13,14=postgres:14" (explicit version=image pairs)
//   - POSTGRES_VERSIONS: "13,14,15,16,17" combined with POSTGRES_IMAGE_PREFIX (default "postgres:")
func parsePostgresVersions(versionsEnv, imagesEnv, prefix string) []PostgresVersion {
	if imagesEnv != "" {
		return parseImagePairs(imagesEnv)
	}

	raw := versionsEnv
	if raw == "" {
		raw = "13,14,15,16,17"
	}

	var versions []PostgresVersion
	for _, v := range strings.Split(raw, ",") {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		versions = append(versions, PostgresVersion{
			Version: v,
			Image:   prefix + v,
		})
	}
	return versions
}

func parseImagePairs(raw string) []PostgresVersion {
	var versions []PostgresVersion
	for _, pair := range strings.Split(raw, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) != 2 {
			continue
		}
		versions = append(versions, PostgresVersion{
			Version: strings.TrimSpace(parts[0]),
			Image:   strings.TrimSpace(parts[1]),
		})
	}
	return versions
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// parseNodeSelector parses "key=value,key2=value2" into a node selector map.
func parseNodeSelector(raw string) map[string]string {
	if raw == "" {
		return nil
	}
	selector := make(map[string]string)
	for _, pair := range strings.Split(raw, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) != 2 {
			continue
		}
		selector[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
	}
	if len(selector) == 0 {
		return nil
	}
	return selector
}

// parseTolerations parses "key=value:effect;key2=value2:effect2" into tolerations.
// Operator is always Equal. Example: kubernetes.io/arch=arm64:NoSchedule
func parseTolerations(raw string) []corev1.Toleration {
	if raw == "" {
		return nil
	}
	var tolerations []corev1.Toleration
	for _, entry := range strings.Split(raw, ";") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		parts := strings.Split(entry, ":")
		if len(parts) != 2 {
			continue
		}
		kv := strings.SplitN(parts[0], "=", 2)
		if len(kv) != 2 {
			continue
		}
		tolerations = append(tolerations, corev1.Toleration{
			Key:      strings.TrimSpace(kv[0]),
			Operator: corev1.TolerationOpEqual,
			Value:    strings.TrimSpace(kv[1]),
			Effect:   corev1.TaintEffect(strings.TrimSpace(parts[1])),
		})
	}
	return tolerations
}
