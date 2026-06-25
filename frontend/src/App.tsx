import { useState } from 'react'
import MigrationForm from './components/MigrationForm'
import MigrationList from './components/MigrationList'
import MigrationDetail from './components/MigrationDetail'

type Tab = 'new' | 'migrations'

function App() {
  const [tab, setTab] = useState<Tab>('new')
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const [refreshKey, setRefreshKey] = useState(0)

  const handleCreated = (id: string) => {
    setSelectedId(id)
    setTab('migrations')
    setRefreshKey((k) => k + 1)
  }

  return (
    <div className="app">
      <header className="header">
        <h1>CNPG Migrator</h1>
        <p>
          Migrate PostgreSQL databases to CloudNativePG clusters using dump &amp;
          restore via ephemeral Kubernetes jobs.
        </p>
      </header>

      <nav className="tabs">
        <button
          className={`tab ${tab === 'new' ? 'active' : ''}`}
          onClick={() => setTab('new')}
        >
          New Migration
        </button>
        <button
          className={`tab ${tab === 'migrations' ? 'active' : ''}`}
          onClick={() => setTab('migrations')}
        >
          Migrations
        </button>
      </nav>

      {tab === 'new' && (
        <MigrationForm onCreated={handleCreated} />
      )}

      {tab === 'migrations' && (
        <div className="layout-split">
          <MigrationList
            key={refreshKey}
            selectedId={selectedId}
            onSelect={setSelectedId}
          />
          {selectedId && (
            <MigrationDetail
              id={selectedId}
              onCancelled={() => setRefreshKey((k) => k + 1)}
            />
          )}
        </div>
      )}
    </div>
  )
}

export default App
