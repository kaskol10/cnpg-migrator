import { useState, FormEvent, useEffect } from 'react'
import { api, AppConfig, CreateMigrationRequest } from '../api/client'

interface Props {
  onCreated: (id: string) => void
}

const emptyForm = (config?: AppConfig): CreateMigrationRequest => ({
  name: '',
  source: {
    host: '',
    port: 5432,
    database: '',
    username: '',
    password: '',
    ssl_mode: 'require',
  },
  target: {
    namespace: 'default',
    cluster: '',
    host: '',
    port: 5432,
    database: '',
    username: 'postgres',
    password: '',
  },
  options: {
    format: 'custom',
    jobs: 4,
    schema_only: false,
    data_only: false,
    clean_before_restore: false,
    preserve_ownership: false,
    migrate_roles: true,
    skip_extensions: true,
    all_databases: false,
    exclude_databases: 'rdsadmin',
    storage_size: '50Gi',
    source_version: config?.default_source_version ?? '16',
    target_version: config?.default_target_version ?? '16',
  },
})

export default function MigrationForm({ onCreated }: Props) {
  const [config, setConfig] = useState<AppConfig | null>(null)
  const [form, setForm] = useState<CreateMigrationRequest>(emptyForm())
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    api.getConfig()
      .then((cfg) => {
        setConfig(cfg)
        setForm(emptyForm(cfg))
      })
      .catch(() => {
        setConfig({
          postgres_versions: [
            { version: '13', image: 'postgres:13' },
            { version: '14', image: 'postgres:14' },
            { version: '15', image: 'postgres:15' },
            { version: '16', image: 'postgres:16' },
            { version: '17', image: 'postgres:17' },
          ],
          default_source_version: '16',
          default_target_version: '16',
        })
      })
  }, [])

  const updateSource = (field: string, value: string | number) => {
    setForm((f) => ({ ...f, source: { ...f.source, [field]: value } }))
  }

  const updateTarget = (field: string, value: string) => {
    setForm((f) => ({ ...f, target: { ...f.target, [field]: value } }))
  }

  const updateOptions = (field: string, value: string | number | boolean) => {
    setForm((f) => ({ ...f, options: { ...f.options, [field]: value } }))
  }

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault()
    setLoading(true)
    setError(null)
    try {
      const migration = await api.createMigration(form)
      onCreated(migration.id)
      setForm(emptyForm(config ?? undefined))
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create migration')
    } finally {
      setLoading(false)
    }
  }

  const versions = config?.postgres_versions ?? []
  const allDatabases = form.options.all_databases
  const defaultTargetHost =
    form.target.cluster && form.target.namespace
      ? `${form.target.cluster}-rw.${form.target.namespace}.svc.cluster.local`
      : '<cluster>-rw.<namespace>.svc.cluster.local'

  return (
    <form onSubmit={handleSubmit}>
      {error && <div className="alert alert-error">{error}</div>}

      <div className="card" style={{ marginBottom: '1rem' }}>
        <h2>Migration Details</h2>
        <div className="form-grid">
          <div className="field full">
            <label htmlFor="name">Migration Name</label>
            <input
              id="name"
              required
              placeholder="e.g. production-app-db"
              value={form.name}
              onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))}
            />
          </div>
        </div>
      </div>

      <div className="layout-split">
        <div className="card">
          <h2>Source — PostgreSQL</h2>
          <div className="form-grid">
            <div className="field">
              <label htmlFor="src-version">PostgreSQL Version</label>
              <select
                id="src-version"
                value={form.options.source_version}
                onChange={(e) => updateOptions('source_version', e.target.value)}
              >
                {versions.map((v) => (
                  <option key={v.version} value={v.version}>
                    PostgreSQL {v.version}
                  </option>
                ))}
              </select>
            </div>
            <div className="field">
              <label htmlFor="src-ssl">SSL Mode</label>
              <select
                id="src-ssl"
                value={form.source.ssl_mode}
                onChange={(e) => updateSource('ssl_mode', e.target.value)}
              >
                <option value="require">require</option>
                <option value="verify-full">verify-full</option>
                <option value="prefer">prefer</option>
                <option value="disable">disable</option>
              </select>
            </div>
            <div className="field full">
              <label htmlFor="src-host">Host</label>
              <input
                id="src-host"
                required
                placeholder="db.example.com or mydb.region.rds.amazonaws.com"
                value={form.source.host}
                onChange={(e) => updateSource('host', e.target.value)}
              />
            </div>
            <div className="field">
              <label htmlFor="src-port">Port</label>
              <input
                id="src-port"
                type="number"
                value={form.source.port}
                onChange={(e) => updateSource('port', parseInt(e.target.value) || 5432)}
              />
            </div>
            <div className="field">
              <label htmlFor="src-db">Database</label>
              <input
                id="src-db"
                required={!allDatabases}
                disabled={allDatabases}
                placeholder={allDatabases ? 'All databases (via postgres)' : ''}
                value={form.source.database}
                onChange={(e) => updateSource('database', e.target.value)}
              />
            </div>
            <div className="field">
              <label htmlFor="src-user">Username</label>
              <input
                id="src-user"
                required
                value={form.source.username}
                onChange={(e) => updateSource('username', e.target.value)}
              />
            </div>
            <div className="field full">
              <label htmlFor="src-pass">Password</label>
              <input
                id="src-pass"
                type="password"
                required
                value={form.source.password}
                onChange={(e) => updateSource('password', e.target.value)}
              />
            </div>
          </div>
        </div>

        <div className="card">
          <h2>Target — CNPG Cluster</h2>
          <div className="form-grid">
            <div className="field">
              <label htmlFor="tgt-version">PostgreSQL Version</label>
              <select
                id="tgt-version"
                value={form.options.target_version}
                onChange={(e) => updateOptions('target_version', e.target.value)}
              >
                {versions.map((v) => (
                  <option key={v.version} value={v.version}>
                    PostgreSQL {v.version}
                  </option>
                ))}
              </select>
            </div>
            <div className="field">
              <label htmlFor="tgt-ns">Namespace</label>
              <input
                id="tgt-ns"
                required
                value={form.target.namespace}
                onChange={(e) => updateTarget('namespace', e.target.value)}
              />
            </div>
            <div className="field">
              <label htmlFor="tgt-cluster">CNPG Cluster Name</label>
              <input
                id="tgt-cluster"
                required
                placeholder="backstage-postgresql"
                value={form.target.cluster}
                onChange={(e) => updateTarget('cluster', e.target.value)}
              />
            </div>
            <div className="field full">
              <label htmlFor="tgt-host">Host (optional)</label>
              <input
                id="tgt-host"
                placeholder={defaultTargetHost}
                value={form.target.host ?? ''}
                onChange={(e) => updateTarget('host', e.target.value)}
              />
            </div>
            <div className="field">
              <label htmlFor="tgt-port">Port</label>
              <input
                id="tgt-port"
                type="number"
                value={form.target.port}
                onChange={(e) =>
                  setForm((f) => ({
                    ...f,
                    target: { ...f.target, port: parseInt(e.target.value) || 5432 },
                  }))
                }
              />
            </div>
            <div className="field">
              <label htmlFor="tgt-db">Database</label>
              <input
                id="tgt-db"
                required={!allDatabases}
                disabled={allDatabases}
                placeholder={allDatabases ? 'Same name as source' : ''}
                value={form.target.database}
                onChange={(e) => updateTarget('database', e.target.value)}
              />
            </div>
            <div className="field">
              <label htmlFor="tgt-user">Username</label>
              <input
                id="tgt-user"
                value={form.target.username}
                onChange={(e) => updateTarget('username', e.target.value)}
              />
            </div>
            <div className="field full">
              <label htmlFor="tgt-pass">Password</label>
              <input
                id="tgt-pass"
                type="password"
                required
                value={form.target.password}
                onChange={(e) => updateTarget('password', e.target.value)}
              />
            </div>
          </div>
        </div>
      </div>

      <div className="card" style={{ marginTop: '1rem' }}>
        <h2>Options</h2>
        <div className="form-grid">
          <div className="field full">
            <label className="checkbox-row">
              <input
                type="checkbox"
                checked={form.options.all_databases}
                onChange={(e) => updateOptions('all_databases', e.target.checked)}
              />
              Migrate all databases
            </label>
          </div>
          {allDatabases && (
            <div className="field full">
              <label htmlFor="exclude-db">Exclude databases (comma-separated)</label>
              <input
                id="exclude-db"
                placeholder="rdsadmin,postgres"
                value={form.options.exclude_databases}
                onChange={(e) => updateOptions('exclude_databases', e.target.value)}
              />
            </div>
          )}
          <div className="field">
            <label htmlFor="format">Dump Format</label>
            <select
              id="format"
              value={form.options.format}
              onChange={(e) => updateOptions('format', e.target.value)}
            >
              <option value="custom">custom (recommended)</option>
              <option value="directory">directory (parallel)</option>
              <option value="plain">plain SQL</option>
            </select>
          </div>
          <div className="field">
            <label htmlFor="jobs">Parallel Jobs</label>
            <input
              id="jobs"
              type="number"
              min={1}
              max={16}
              value={form.options.jobs}
              onChange={(e) => updateOptions('jobs', parseInt(e.target.value) || 4)}
            />
          </div>
          <div className="field">
            <label htmlFor="storage">PVC Storage Size</label>
            <input
              id="storage"
              value={form.options.storage_size}
              onChange={(e) => updateOptions('storage_size', e.target.value)}
            />
          </div>
          <div className="field" style={{ justifyContent: 'flex-end' }}>
            <label className="checkbox-row">
              <input
                type="checkbox"
                checked={form.options.schema_only}
                onChange={(e) => updateOptions('schema_only', e.target.checked)}
              />
              Schema only
            </label>
          </div>
          <div className="field" style={{ justifyContent: 'flex-end' }}>
            <label className="checkbox-row">
              <input
                type="checkbox"
                checked={form.options.data_only}
                onChange={(e) => updateOptions('data_only', e.target.checked)}
              />
              Data only
            </label>
          </div>
          <div className="field" style={{ justifyContent: 'flex-end' }}>
            <label className="checkbox-row">
              <input
                type="checkbox"
                checked={form.options.preserve_ownership}
                onChange={(e) => {
                  const checked = e.target.checked
                  setForm((f) => ({
                    ...f,
                    options: {
                      ...f.options,
                      preserve_ownership: checked,
                      migrate_roles: checked ? f.options.migrate_roles : false,
                    },
                  }))
                }}
              />
              Preserve ownership and grants
            </label>
          </div>
          <div className="field" style={{ justifyContent: 'flex-end' }}>
            <label className="checkbox-row">
              <input
                type="checkbox"
                checked={form.options.migrate_roles}
                disabled={!form.options.preserve_ownership}
                onChange={(e) => updateOptions('migrate_roles', e.target.checked)}
              />
              Migrate roles from source
            </label>
          </div>
          <div className="field" style={{ justifyContent: 'flex-end' }}>
            <label className="checkbox-row">
              <input
                type="checkbox"
                checked={form.options.skip_extensions}
                onChange={(e) => updateOptions('skip_extensions', e.target.checked)}
              />
              Skip extensions on restore
            </label>
          </div>
          <div className="field" style={{ justifyContent: 'flex-end' }}>
            <label className="checkbox-row">
              <input
                type="checkbox"
                checked={form.options.clean_before_restore}
                onChange={(e) => updateOptions('clean_before_restore', e.target.checked)}
              />
              Clean before restore
            </label>
          </div>
        </div>

        <div className="form-actions">
          <button type="submit" className="btn btn-primary" disabled={loading || !config}>
            {loading ? 'Starting...' : 'Start Migration'}
          </button>
        </div>
      </div>
    </form>
  )
}
