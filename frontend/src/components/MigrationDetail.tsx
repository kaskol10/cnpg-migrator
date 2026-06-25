import { useEffect, useState } from 'react'
import { api, Migration, MigrationLog } from '../api/client'
import VerificationPanel from './VerificationPanel'

interface Props {
  id: string
  onCancelled: () => void
}

function formatDate(iso?: string) {
  if (!iso) return '—'
  return new Date(iso).toLocaleString()
}

function stepClass(status: string, step: string): string {
  const order = ['pending', 'dumping', 'restoring', 'completed']
  const current = order.indexOf(status === 'failed' || status === 'cancelled' ? status : status)
  const stepIdx = { dump: 1, restore: 2, done: 3 }[step] ?? 0

  if (status === 'completed') return 'step done'
  if (status === 'failed' || status === 'cancelled') {
    if (stepIdx <= current) return stepIdx < current ? 'step done' : 'step active'
    return 'step'
  }
  if (stepIdx < current) return 'step done'
  if (stepIdx === current) return 'step active'
  return 'step'
}

export default function MigrationDetail({ id, onCancelled }: Props) {
  const [migration, setMigration] = useState<Migration | null>(null)
  const [logs, setLogs] = useState<MigrationLog[]>([])
  const [loading, setLoading] = useState(true)
  const [actionLoading, setActionLoading] = useState(false)

  const fetchData = async () => {
    try {
      const [m, l] = await Promise.all([
        api.getMigration(id),
        api.getLogs(id),
      ])
      setMigration(m)
      setLogs(l)
    } catch {
      // ignore polling errors
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    fetchData()
    const interval = setInterval(fetchData, 3000)
    return () => clearInterval(interval)
  }, [id])

  const handleCancel = async () => {
    setActionLoading(true)
    try {
      await api.cancelMigration(id)
      onCancelled()
      await fetchData()
    } finally {
      setActionLoading(false)
    }
  }

  const handleCleanup = async () => {
    setActionLoading(true)
    try {
      await api.cleanupResources(id)
      await fetchData()
    } finally {
      setActionLoading(false)
    }
  }

  const handleRerunVerification = async () => {
    setActionLoading(true)
    try {
      await api.startVerification(id)
      await fetchData()
    } finally {
      setActionLoading(false)
    }
  }

  if (loading || !migration) {
    return <div className="card"><p>Loading...</p></div>
  }

  const isActive = ['pending', 'dumping', 'restoring'].includes(migration.status)
  const logText = logs.map((l) => l.message).join('\n')

  return (
    <div className="detail-panel">
      <div className="card">
        <div className="migration-item-header" style={{ marginBottom: '1rem' }}>
          <h2 style={{ fontSize: '1.1rem' }}>{migration.name}</h2>
          <span className={`status-badge status-${migration.status}`}>
            {migration.status}
          </span>
        </div>

        {migration.error && (
          <div className="error-box">{migration.error}</div>
        )}

        <div className="progress-steps">
          <div className={stepClass(migration.status, 'dump')}>1. Dump from RDS</div>
          <div className={stepClass(migration.status, 'restore')}>2. Restore to CNPG</div>
          <div className={stepClass(migration.status, 'done')}>3. Complete</div>
        </div>

        <div className="detail-grid">
          <div className="detail-item">
            <label>PostgreSQL Versions</label>
            <span>
              dump: {migration.options.source_version} → restore client:{' '}
              {migration.options.restore_client_version || migration.options.target_version}{' '}
              (CNPG server: {migration.options.target_version})
            </span>
          </div>
          <div className="detail-item">
            <label>Source</label>
            <span>
              {migration.source.host}:{migration.source.port}/
              {migration.options.all_databases ? 'all databases' : migration.source.database}
            </span>
          </div>
          <div className="detail-item">
            <label>Target</label>
            <span>
              {(migration.target.host ||
                `${migration.target.cluster}-rw.${migration.target.namespace}.svc.cluster.local`)}
              :{migration.target.port || 5432}/
              {migration.options.all_databases ? 'all databases' : migration.target.database}
            </span>
          </div>
          <div className="detail-item">
            <label>Job</label>
            <span>{migration.job_name || '—'}</span>
          </div>
          <div className="detail-item">
            <label>PVC</label>
            <span>{migration.pvc_name || '—'}</span>
          </div>
          <div className="detail-item">
            <label>Started</label>
            <span>{formatDate(migration.started_at)}</span>
          </div>
          <div className="detail-item">
            <label>Completed</label>
            <span>{formatDate(migration.completed_at)}</span>
          </div>
        </div>

        <div className="form-actions">
          {isActive && (
            <button
              className="btn btn-danger"
              onClick={handleCancel}
              disabled={actionLoading}
            >
              Cancel
            </button>
          )}
          {!isActive && (
            <button
              className="btn btn-secondary"
              onClick={handleCleanup}
              disabled={actionLoading}
            >
              Cleanup Resources
            </button>
          )}
        </div>
      </div>

      {migration.status === 'completed' && (
        <VerificationPanel
          verification={migration.verification}
          loading={actionLoading}
          onRerun={handleRerunVerification}
        />
      )}

      <div className="card" style={{ marginTop: '1rem' }}>
        <h2>Logs</h2>
        <div className="log-viewer">{logText}</div>
      </div>
    </div>
  )
}
