import { useEffect, useState } from 'react'
import { api, Migration } from '../api/client'

interface Props {
  selectedId: string | null
  onSelect: (id: string) => void
}

function formatDate(iso: string) {
  return new Date(iso).toLocaleString()
}

export default function MigrationList({ selectedId, onSelect }: Props) {
  const [migrations, setMigrations] = useState<Migration[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const fetchMigrations = async () => {
    try {
      const data = await api.listMigrations()
      setMigrations(
        data.sort(
          (a, b) => new Date(b.created_at).getTime() - new Date(a.created_at).getTime()
        )
      )
      setError(null)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load migrations')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    fetchMigrations()
    const interval = setInterval(fetchMigrations, 5000)
    return () => clearInterval(interval)
  }, [])

  if (loading) {
    return <div className="card"><p className="empty-state">Loading...</p></div>
  }

  if (error) {
    return <div className="card"><div className="alert alert-error">{error}</div></div>
  }

  if (migrations.length === 0) {
    return (
      <div className="card">
        <div className="empty-state">
          <h3>No migrations yet</h3>
          <p>Create a new migration to get started.</p>
        </div>
      </div>
    )
  }

  return (
    <div className="migration-list">
      {migrations.map((m) => (
        <div
          key={m.id}
          className={`migration-item ${selectedId === m.id ? 'selected' : ''}`}
          onClick={() => onSelect(m.id)}
        >
          <div className="migration-item-header">
            <h3>{m.name}</h3>
            <span className={`status-badge status-${m.status}`}>{m.status}</span>
          </div>
          <div className="migration-item-meta">
            {m.options.all_databases
              ? `all databases → ${m.target.cluster}`
              : `${m.source.database} → ${m.target.cluster}/${m.target.database}`}
            <br />
            {formatDate(m.created_at)}
          </div>
        </div>
      ))}
    </div>
  )
}
