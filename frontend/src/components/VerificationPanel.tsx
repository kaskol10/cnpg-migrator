import { Verification } from '../api/client'

interface Props {
  verification?: Verification
  loading?: boolean
  onRerun?: () => void
}

function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(1024))
  const value = bytes / Math.pow(1024, i)
  return `${value.toFixed(i > 0 ? 1 : 0)} ${units[i]}`
}

function statusLabel(status: string): string {
  switch (status) {
    case 'ok':
      return 'OK'
    case 'missing':
      return 'Missing on target'
    case 'size_mismatch':
      return 'Size mismatch'
    default:
      return status
  }
}

export default function VerificationPanel({ verification, loading, onRerun }: Props) {
  if (!verification || verification.status === 'pending') {
    return null
  }

  if (verification.status === 'running') {
    return (
      <div className="card" style={{ marginTop: '1rem' }}>
        <h2>Verification</h2>
        <p className="empty-state" style={{ padding: '1.5rem' }}>
          Comparing databases between RDS and CNPG...
        </p>
      </div>
    )
  }

  const summary = verification.summary ?? {
    total_databases: 0,
    matched: 0,
    missing: 0,
    size_mismatch: 0,
    passed: false,
  }
  const passed = verification.status === 'passed'

  return (
    <div className="card" style={{ marginTop: '1rem' }}>
      <div className="migration-item-header" style={{ marginBottom: '1rem' }}>
        <h2>Verification</h2>
        <span className={`status-badge status-${passed ? 'completed' : 'failed'}`}>
          {passed ? 'passed' : 'failed'}
        </span>
      </div>

      {verification.error && (
        <div className="error-box">{verification.error}</div>
      )}

      <div className="verification-summary">
        <div className="summary-stat">
          <span className="summary-value">{summary.total_databases}</span>
          <span className="summary-label">Databases</span>
        </div>
        <div className="summary-stat">
          <span className="summary-value ok">{summary.matched}</span>
          <span className="summary-label">Matched</span>
        </div>
        <div className="summary-stat">
          <span className="summary-value warn">{summary.size_mismatch}</span>
          <span className="summary-label">Size mismatch</span>
        </div>
        <div className="summary-stat">
          <span className="summary-value error">{summary.missing}</span>
          <span className="summary-label">Missing</span>
        </div>
      </div>

      {verification.databases && verification.databases.length > 0 && (
        <div className="comparison-table-wrap">
          <table className="comparison-table">
            <thead>
              <tr>
                <th>Database</th>
                <th>RDS size</th>
                <th>CNPG size</th>
                <th>Diff</th>
                <th>Status</th>
              </tr>
            </thead>
            <tbody>
              {verification.databases.map((db) => (
                <tr key={db.database} className={`row-${db.status}`}>
                  <td>{db.database}</td>
                  <td>{formatBytes(db.source_size_bytes)}</td>
                  <td>{db.target_exists ? formatBytes(db.target_size_bytes) : '—'}</td>
                  <td>{db.size_diff_percent.toFixed(1)}%</td>
                  <td>
                    <span className={`verify-status verify-${db.status}`}>
                      {statusLabel(db.status)}
                    </span>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      <p className="verification-note">
        Size comparison allows up to 10% difference (vacuum/bloat variance).
      </p>

      {onRerun && (
        <div className="form-actions">
          <button className="btn btn-secondary" onClick={onRerun} disabled={loading}>
            {loading ? 'Running...' : 'Re-run verification'}
          </button>
        </div>
      )}
    </div>
  )
}
