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

type InFlight = Record<number, 'stopping' | 'deleting' | undefined>

export default function RunsList() {
  const [runs, setRuns] = useState<Run[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string>()
  const [actionError, setActionError] = useState<Record<number, string | undefined>>({})
  const [inFlight, setInFlight] = useState<InFlight>({})
  const navigate = useNavigate()

  function fetchRuns() {
    return api.listRuns()
      .then((r) => setRuns(r.runs))
      .catch((e: unknown) => setError(e instanceof Error ? e.message : 'Failed to load runs'))
      .finally(() => setLoading(false))
  }

  useEffect(() => { fetchRuns() }, [])

  async function handleStop(e: React.MouseEvent, run: Run) {
    e.stopPropagation()
    if (!window.confirm('Stop this run?')) return
    setInFlight((prev) => ({ ...prev, [run.id]: 'stopping' }))
    setActionError((prev) => ({ ...prev, [run.id]: undefined }))
    try {
      await api.stopRun(run.id)
      setLoading(true)
      await fetchRuns()
    } catch (err) {
      setActionError((prev) => ({
        ...prev,
        [run.id]: err instanceof Error ? err.message : 'Stop failed',
      }))
    } finally {
      setInFlight((prev) => ({ ...prev, [run.id]: undefined }))
    }
  }

  async function handleDelete(e: React.MouseEvent, run: Run) {
    e.stopPropagation()
    if (!window.confirm(`Delete run #${run.id} and all its data? This cannot be undone.`)) return
    setInFlight((prev) => ({ ...prev, [run.id]: 'deleting' }))
    setActionError((prev) => ({ ...prev, [run.id]: undefined }))
    try {
      await api.deleteRun(run.id)
      setRuns((prev) => prev.filter((r) => r.id !== run.id))
    } catch (err) {
      setActionError((prev) => ({
        ...prev,
        [run.id]: err instanceof Error ? err.message : 'Delete failed',
      }))
    } finally {
      setInFlight((prev) => ({ ...prev, [run.id]: undefined }))
    }
  }

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
                  <th className="col-mobile-hide">Created</th>
                  <th>Player Model</th>
                  <th className="col-mobile-hide">Reflector Model</th>
                  <th className="col-mobile-hide">Seed</th>
                  <th className="col-mobile-hide">Gens</th>
                  <th>Status</th>
                  <th>Actions</th>
                </tr>
              </thead>
              <tbody>
                {runs.map((r) => {
                  const busy = inFlight[r.id]
                  return (
                    <tr key={r.id} className="clickable" onClick={() => navigate(`/runs/${r.id}`)}>
                      <td style={{ fontWeight: 600 }}>#{r.id}</td>
                      <td className="text-secondary col-mobile-hide">{fmt(r.createdAt)}</td>
                      <td className="font-mono text-sm col-model">{r.playerModel}</td>
                      <td className="font-mono text-sm col-mobile-hide">{r.reflectorModel}</td>
                      <td className="col-mobile-hide">{r.seed}</td>
                      <td className="col-mobile-hide">{r.generations}</td>
                      <td>{statusBadge(r.status)}</td>
                      <td onClick={(e) => e.stopPropagation()} style={{ whiteSpace: 'nowrap', display: 'flex', gap: 6 }}>
                        {r.status === 'running' && (
                          <button
                            disabled={!!busy}
                            onClick={(e) => handleStop(e, r)}
                            style={{
                              border: '1px solid #e07b00',
                              color: busy === 'stopping' ? '#aaa' : '#e07b00',
                              background: 'white',
                              borderRadius: 4,
                              padding: '2px 10px',
                              cursor: busy ? 'not-allowed' : 'pointer',
                              fontSize: '0.85rem',
                            }}
                            onMouseEnter={(e) => { if (!busy) (e.currentTarget as HTMLButtonElement).style.background = '#fff3e0' }}
                            onMouseLeave={(e) => { (e.currentTarget as HTMLButtonElement).style.background = 'white' }}
                          >
                            {busy === 'stopping' ? '…' : 'Stop'}
                          </button>
                        )}
                        <button
                          disabled={!!busy}
                          onClick={(e) => handleDelete(e, r)}
                          style={{
                            border: '1px solid #c0392b',
                            color: busy === 'deleting' ? '#aaa' : '#c0392b',
                            background: 'white',
                            borderRadius: 4,
                            padding: '2px 10px',
                            cursor: busy ? 'not-allowed' : 'pointer',
                            fontSize: '0.85rem',
                          }}
                          onMouseEnter={(e) => { if (!busy) (e.currentTarget as HTMLButtonElement).style.background = '#fdf0ef' }}
                          onMouseLeave={(e) => { (e.currentTarget as HTMLButtonElement).style.background = 'white' }}
                        >
                          {busy === 'deleting' ? '…' : 'Delete'}
                        </button>
                        {actionError[r.id] && (
                          <span style={{ marginLeft: 8, color: '#c0392b', fontSize: '0.8rem' }}>
                            {actionError[r.id]}
                          </span>
                        )}
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        </div>
      )}
    </div>
  )
}
