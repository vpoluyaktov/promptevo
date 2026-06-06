import { useState, useEffect } from 'react'
import { api } from '../api/client'
import type { Run, RunDetail } from '../api/types'
import {
  LineChart,
  Line,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  Legend,
  ResponsiveContainer,
} from 'recharts'

const COLORS = ['#6aaa64', '#c9b458', '#787c7e', '#1565c0', '#c62828', '#7b1fa2', '#e65100', '#00695c']

interface RunWithGens extends Run {
  generationsData?: RunDetail['generationsData']
}

function sameSeed(selected: RunWithGens[]): boolean {
  if (selected.length < 2) return false
  const first = selected[0].seed
  return selected.every((r) => r.seed === first)
}

function buildCompareData(runs: RunWithGens[], metric: 'solveRate' | 'meanGuesses') {
  const maxGens = Math.max(...runs.map((r) => r.generationsData?.length ?? 0))
  return Array.from({ length: maxGens }, (_, i) => {
    const row: Record<string, string | number> = { gen: `Gen ${i}` }
    for (const r of runs) {
      const g = r.generationsData?.[i]
      if (g) {
        const v = metric === 'solveRate' ? (g.solveRate != null ? +(g.solveRate * 100).toFixed(1) : null) : (g.meanGuesses != null ? +g.meanGuesses.toFixed(2) : null)
        if (v !== null) row[`Run #${r.id}`] = v
      }
    }
    return row
  })
}

export default function Compare() {
  const [allRuns, setAllRuns] = useState<Run[]>([])
  const [selectedIds, setSelectedIds] = useState<Set<number>>(new Set())
  const [loadedRuns, setLoadedRuns] = useState<Map<number, RunWithGens>>(new Map())
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    api.listRuns().then((r) => {
      setAllRuns(r.runs.filter((run) => run.status === 'completed'))
    }).finally(() => setLoading(false))
  }, [])

  async function toggleRun(id: number) {
    setSelectedIds((prev) => {
      const next = new Set(prev)
      if (next.has(id)) {
        next.delete(id)
      } else {
        next.add(id)
      }
      return next
    })

    if (!loadedRuns.has(id)) {
      const detail = await api.getRun(id)
      setLoadedRuns((prev) => new Map(prev).set(id, detail))
    }
  }

  const selected: RunWithGens[] = Array.from(selectedIds)
    .map((id) => loadedRuns.get(id))
    .filter((r): r is RunWithGens => r !== undefined)

  const solveData = selected.length > 0 ? buildCompareData(selected, 'solveRate') : []
  const guessData = selected.length > 0 ? buildCompareData(selected, 'meanGuesses') : []
  const sameSeeds = sameSeed(selected)

  return (
    <div className="page">
      <div className="mb-24">
        <h1 style={{ fontSize: '1.6rem', fontWeight: 700, marginBottom: 4 }}>Model Comparison</h1>
        <p className="text-secondary text-sm">Select completed runs to overlay their performance metrics.</p>
      </div>

      {loading && <div style={{ display: 'flex', justifyContent: 'center', padding: 48 }}><div className="spinner" /></div>}

      {!loading && allRuns.length === 0 && (
        <div className="empty-state">
          <h3>No completed runs</h3>
          <p>Complete some runs first to compare them here.</p>
        </div>
      )}

      {!loading && allRuns.length > 0 && (
        <div className="compare-layout">
          {/* Run selector */}
          <div className="card" style={{ padding: 0, alignSelf: 'start' }}>
            <div style={{ padding: '14px 16px', borderBottom: '1px solid #e0e0e0', fontWeight: 600, fontSize: '0.9rem' }}>
              Select Runs
            </div>
            <div style={{ padding: 8 }}>
              {allRuns.map((r, i) => (
                <label
                  key={r.id}
                  style={{
                    display: 'flex',
                    alignItems: 'flex-start',
                    gap: 10,
                    padding: '10px 8px',
                    cursor: 'pointer',
                    borderRadius: 6,
                    background: selectedIds.has(r.id) ? '#f0faf0' : 'transparent',
                  }}
                >
                  <input
                    type="checkbox"
                    checked={selectedIds.has(r.id)}
                    onChange={() => toggleRun(r.id)}
                    style={{ marginTop: 2 }}
                  />
                  <div>
                    <div style={{ fontWeight: 600, fontSize: '0.85rem', color: selectedIds.has(r.id) ? COLORS[i % COLORS.length] : 'var(--text)' }}>
                      Run #{r.id}
                    </div>
                    <div className="text-secondary" style={{ fontSize: '0.75rem' }}>
                      {r.playerModel}
                    </div>
                    <div className="text-secondary" style={{ fontSize: '0.75rem' }}>
                      seed {r.seed} · {r.generations} gens
                    </div>
                  </div>
                </label>
              ))}
            </div>
          </div>

          {/* Charts */}
          <div>
            {selected.length === 0 && (
              <div className="empty-state card">
                <p>Select at least one run to see charts.</p>
              </div>
            )}

            {selected.length > 0 && (
              <>
                {sameSeeds && (
                  <div style={{ marginBottom: 16 }}>
                    <span className="badge badge-stable">Same seed — fair comparison</span>
                  </div>
                )}

                <div className="card mb-24">
                  <h3 style={{ marginBottom: 16, fontWeight: 600, fontSize: '1rem' }}>Solve Rate (%)</h3>
                  <ResponsiveContainer width="100%" height={260}>
                    <LineChart data={solveData}>
                      <CartesianGrid strokeDasharray="3 3" stroke="#e0e0e0" />
                      <XAxis dataKey="gen" tick={{ fontSize: 12 }} />
                      <YAxis domain={[0, 100]} tick={{ fontSize: 12 }} />
                      <Tooltip />
                      <Legend />
                      {selected.map((r, i) => (
                        <Line
                          key={r.id}
                          type="monotone"
                          dataKey={`Run #${r.id}`}
                          stroke={COLORS[i % COLORS.length]}
                          strokeWidth={2}
                          dot={{ r: 4 }}
                          connectNulls
                        />
                      ))}
                    </LineChart>
                  </ResponsiveContainer>
                </div>

                <div className="card">
                  <h3 style={{ marginBottom: 16, fontWeight: 600, fontSize: '1rem' }}>Mean Guesses</h3>
                  <ResponsiveContainer width="100%" height={260}>
                    <LineChart data={guessData}>
                      <CartesianGrid strokeDasharray="3 3" stroke="#e0e0e0" />
                      <XAxis dataKey="gen" tick={{ fontSize: 12 }} />
                      <YAxis domain={[1, 7]} tick={{ fontSize: 12 }} />
                      <Tooltip />
                      <Legend />
                      {selected.map((r, i) => (
                        <Line
                          key={r.id}
                          type="monotone"
                          dataKey={`Run #${r.id}`}
                          stroke={COLORS[i % COLORS.length]}
                          strokeWidth={2}
                          dot={{ r: 4 }}
                          connectNulls
                        />
                      ))}
                    </LineChart>
                  </ResponsiveContainer>
                </div>

                {/* Legend detail */}
                <div className="card mt-24">
                  <h3 style={{ marginBottom: 12, fontWeight: 600, fontSize: '0.9rem' }}>Legend</h3>
                  <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
                    {selected.map((r, i) => (
                      <div key={r.id} className="flex items-center gap-16">
                        <div style={{ width: 16, height: 4, background: COLORS[i % COLORS.length], borderRadius: 2 }} />
                        <span style={{ fontWeight: 600, fontSize: '0.85rem' }}>Run #{r.id}</span>
                        <span className="text-secondary text-sm">{r.playerModel} · seed {r.seed}</span>
                      </div>
                    ))}
                  </div>
                </div>
              </>
            )}
          </div>
        </div>
      )}
    </div>
  )
}
