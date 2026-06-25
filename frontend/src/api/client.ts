export type MigrationStatus =
  | 'pending'
  | 'dumping'
  | 'restoring'
  | 'completed'
  | 'failed'
  | 'cancelled'

export interface SourceConfig {
  host: string
  port: number
  database: string
  username: string
  password: string
  ssl_mode: string
}

export interface TargetConfig {
  namespace: string
  cluster: string
  host?: string
  port: number
  database: string
  username: string
  password: string
}

export interface MigrationOptions {
  format: string
  jobs: number
  schema_only: boolean
  data_only: boolean
  clean_before_restore: boolean
  all_databases: boolean
  exclude_databases: string
  storage_size: string
  source_version: string
  target_version: string
  restore_client_version?: string
}

export interface PostgresVersion {
  version: string
  image: string
}

export interface AppConfig {
  postgres_versions: PostgresVersion[]
  default_source_version: string
  default_target_version: string
}

export interface DatabaseComparison {
  database: string
  source_size_bytes: number
  target_size_bytes: number
  target_exists: boolean
  size_match: boolean
  size_diff_percent: number
  status: string
}

export interface VerificationSummary {
  total_databases: number
  matched: number
  missing: number
  size_mismatch: number
  passed: boolean
}

export type VerificationStatus = 'pending' | 'running' | 'passed' | 'failed'

export interface Verification {
  status: VerificationStatus
  job_name?: string
  databases?: DatabaseComparison[]
  summary: VerificationSummary
  error?: string
  started_at?: string
  completed_at?: string
}

export interface Migration {
  id: string
  name: string
  status: MigrationStatus
  source: SourceConfig
  target: TargetConfig
  options: MigrationOptions
  job_name?: string
  pvc_name?: string
  error?: string
  phase?: string
  created_at: string
  updated_at: string
  started_at?: string
  completed_at?: string
  verification?: Verification
}

export interface CreateMigrationRequest {
  name: string
  source: SourceConfig
  target: TargetConfig
  options: MigrationOptions
}

export interface MigrationLog {
  timestamp: string
  message: string
}

const API_BASE = ''

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const res = await fetch(`${API_BASE}${path}`, {
    headers: { 'Content-Type': 'application/json' },
    ...options,
  })
  if (!res.ok) {
    const body = await res.json().catch(() => ({}))
    throw new Error(body.error || `Request failed: ${res.status}`)
  }
  return res.json()
}

export const api = {
  getConfig: () => request<AppConfig>('/api/v1/config'),
  listMigrations: () => request<Migration[]>('/api/v1/migrations'),
  getMigration: (id: string) => request<Migration>(`/api/v1/migrations/${id}`),
  createMigration: (data: CreateMigrationRequest) =>
    request<Migration>('/api/v1/migrations', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  getLogs: (id: string) => request<MigrationLog[]>(`/api/v1/migrations/${id}/logs`),
  getVerification: (id: string) => request<Verification>(`/api/v1/migrations/${id}/verification`),
  startVerification: (id: string) =>
    request<Verification>(`/api/v1/migrations/${id}/verify`, { method: 'POST' }),
  cancelMigration: (id: string) =>
    request<Migration>(`/api/v1/migrations/${id}/cancel`, { method: 'POST' }),
  cleanupResources: (id: string) =>
    request<{ status: string }>(`/api/v1/migrations/${id}/resources`, { method: 'DELETE' }),
}
