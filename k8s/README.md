# Kubernetes deployment

Deploy **cnpg-migrator** with Helm.

## Prerequisites

- Kubernetes cluster with the [CloudNativePG operator](https://cloudnative-pg.io/) installed
- Helm 3
- Network path from migration Job pods to your source PostgreSQL and CNPG clusters
- A `StorageClass` that supports `ReadWriteOnce` PVCs

## Install

```bash
helm install cnpg-migrator ./k8s/helm/cnpg-migrator \
  --namespace cnpg-migrator \
  --create-namespace \
  --set image.repository=ghcr.io/YOUR_ORG/cnpg-migrator \
  --set image.tag=0.1.0
```

## Upgrade

```bash
helm upgrade cnpg-migrator ./k8s/helm/cnpg-migrator \
  --namespace cnpg-migrator \
  --set image.tag=0.2.0
```

## Common values

| Value | Description |
|-------|-------------|
| `image.repository` / `image.tag` | API server image |
| `config.jobNodeSelector` | Node selector for migration Jobs (`key=value,...`) |
| `config.jobTolerations` | Tolerations for migration Jobs (`key=value:NoSchedule;...`) |
| `ingress.enabled` | Expose the UI via Ingress |
| `nodeSelector` / `tolerations` | Scheduling for the API server pod |

Example with ingress and arm64 job scheduling:

```bash
helm install cnpg-migrator ./k8s/helm/cnpg-migrator \
  --namespace cnpg-migrator \
  --create-namespace \
  --set ingress.enabled=true \
  --set ingress.className=nginx \
  --set ingress.hosts[0].host=cnpg-migrator.example.com \
  --set config.jobNodeSelector=kubernetes.io/arch=arm64 \
  --set config.jobTolerations=kubernetes.io/arch=arm64:NoSchedule
```

## Uninstall

```bash
helm uninstall cnpg-migrator --namespace cnpg-migrator
```
