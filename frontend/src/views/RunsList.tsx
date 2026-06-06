import { useState, useEffect } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { api } from '../api/client'
import type { Run } from '../api/types'

function statusBadge(status: Run['status']) {
  return <span className={`badge badge-${status}`}>{status}</span>
}

function fmt(iso: string) {
  return new Date(iso).toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' })
}

export default function RunsList() {
  const [runs, setRuns] = useState<Run[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string>()
  const navigate = useNavigate()

  useEffect(() => {
    api.listRuns()
      .then((r) => setRuns(r.runs))
      .catch((e: unknown) => setError(e instanceof Error ? e.message : 'Failed to load runs'))
      .finally(() => setLoading(false))
  }, [])

  return (
    <div className="page">
      <div className="flex items-center justify-between mb-24">
        <div>
          <h1 style={{ fontSize: '1.6rem', fontWeight: 700, marginBottom: 4 }}>Runs</h1>
          <p className="text-secondary text-sm">All past and active experiments</p>
        </div>
        <Link to="/" className="btn btn-primary">+ New Run</Link>
      </div>

      {loading && <div style={{ display: 'flex', justifyContent: 'center', padding: 48 }}><div className="spinner" /></div>}
      {error && <div className="error-box">{error}</div>}
      {!loading && !error && runs.length === 0 && (
        <div className="empty-state">
          <h3>No runs yet</h3>
          <p>Launch your first experiment to get started.</p>
          <Link to="/" className="btn btn-primary" style={{ marginTop: 16 }}>Launch Run</Link>
        </div>
      )}
      {!loading && !error && runs.length > 0 && (
        <div className="card" style={{ padding: 0 }}>
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>ID</th>
                  <th>Created</th>
                  <th>Player Model</th>
                  <th>Reflector Model</th>
                  <th>Seed</th>
                  <th>Gens</th>
                  <th>Status</th>
                </tr>
              </thead>
              <tbody>
                {runs.map((r) => (
                  <tr key={r.id} className="clickable" onClick={() => navigate(`/runs/${r.id}`)}>
                    <td style={{ fontWeight: 600 }}>#{r.id}</td>
                    <td className="text-secondary">{fmt(r.createdAt)}</td>
                    <td className="font-mono text-sm">{r.playerModel}</td>
                    <td className="font-mono text-sm">{r.reflectorModel}</td>
                    <td>{r.seed}</td>
                    <td>{r.generations}</td>
                    <td>{statusBadge(r.status)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}
    </div>
  )
}
