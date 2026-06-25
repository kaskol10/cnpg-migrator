# CNPG cluster example

Example umbrella chart that provisions a [CloudNativePG](https://cloudnative-pg.io/) PostgreSQL cluster using the official [`cluster`](https://cloudnative-pg.github.io/charts) chart. Use it as a **migration target** when testing cnpg-migrator.

## Prerequisites

- Kubernetes cluster with the CloudNativePG operator installed
- Helm 3.8+

## Install

```bash
helm dependency update
helm upgrade --install test-cnpg-cluster . \
  --namespace backstage \
  --create-namespace
```

> If your cluster enforces namespace labels (e.g. Kyverno), create the namespace with the required labels first and omit `--create-namespace`.

## Customize

Edit `values.yaml` for your environment:

| Area | Values to review |
|------|------------------|
| Cluster identity | `postgresql.fullnameOverride`, `postgresql.cluster.initdb` |
| Sizing | `postgresql.cluster.instances`, `storage.size`, `resources` |
| Scheduling | `postgresql.cluster.affinity.nodeSelector`, `tolerations` |
| Backups | `postgresql.backups.s3`, `serviceAccountTemplate` IAM role |

After install, the CNPG read-write service is typically:

```text
<fullnameOverride>-rw.<namespace>.svc.cluster.local
```

For the default values: `test-cnpg-cluster-rw.backstage.svc.cluster.local`

Use that host (or the cluster name `test-cnpg-cluster`) as the migration target in cnpg-migrator.

## Uninstall

```bash
helm uninstall test-cnpg-cluster --namespace backstage
```
