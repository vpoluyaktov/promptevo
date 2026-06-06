import { useState, useEffect } from 'react'
import { useParams, Link } from 'react-router-dom'
import { api } from '../api/client'
import type { RunDetail as RunDetailType, Game, Generation } from '../api/types'
import MetricsChart from '../components/MetricsChart'
import PromptDiff from '../components/PromptDiff'
import GameList from '../components/GameList'
import ConvergenceBadge from '../components/ConvergenceBadge'
import AnalysisTab from '../components/analytics/AnalysisTab'

type Tab = 'charts' | 'prompts' | 'games' | 'analysis'

function fmt(iso: string) {
  return new Date(iso).toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' })
}

function statusBadge(status: string) {
  return <span className={`badge badge-${status}`}>{status}</span>
}

interface PromptTabProps {
  generations: Generation[]
}

function PromptTimeline({ generations }: PromptTabProps) {
  const [idx, setIdx] = useState(0)

  if (generations.length === 0) {
    return <div className="empty-state"><p>No generations yet.</p></div>
  }

  const cur = generations[idx]
  const prev = idx > 0 ? generations[idx - 1] : null

  return (
    <div>
      <div className="flex items-center gap-16" style={{ marginBottom: 24 }}>
        <button
          className="btn btn-secondary btn-sm"
          disabled={idx === 0}
          onClick={() => setIdx((i) => i - 1)}
        >← Prev</button>
        <span style={{ fontWeight: 600 }}>
          Generation {cur.genIndex}
        </span>
        <button
          className="btn btn-secondary btn-sm"
          disabled={idx === generations.length - 1}
          onClick={() => setIdx((i) => i + 1)}
        >Next →</button>
        <span className="text-secondary text-sm" style={{ marginLeft: 'auto' }}>
          {cur.promptLen} chars
          {cur.solveRate != null && ` · ${(cur.solveRate * 100).toFixed(0)}% solve rate`}
        </span>
      </div>

      <div className="grid-2">
        <div>
          <div className="prompt-card">
            <div className="prompt-card-header">Strategy Prompt — Gen {cur.genIndex}</div>
            <div className="prompt-card-body">{cur.promptText}</div>
          </div>
          {cur.reflectionText && (
            <div className="prompt-card mt-16">
              <div className="prompt-card-header">Reflection / Diagnosis</div>
              <div className="prompt-card-body">{cur.reflectionText}</div>
            </div>
          )}
        </div>
        <div>
          {prev ? (
            <PromptDiff
              oldPrompt={prev.promptText}
              newPrompt={cur.promptText}
              title={`Changes from Gen ${prev.genIndex} → Gen ${cur.genIndex}`}
            />
          ) : (
            <div className="empty-state" style={{ padding: 24 }}>
              <p>No previous generation to diff against.</p>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}

export default function RunDetail() {
  const { id } = useParams<{ id: string }>()
  const runId = Number(id)

  const [run, setRun] = useState<RunDetailType | null>(null)
  const [games, setGames] = useState<Game[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string>()
  const [tab, setTab] = useState<Tab>('charts')
  const [deleting, setDeleting] = useState(false)

  useEffect(() => {
    setLoading(true)
    setRun(null)
    setGames([])
    setError(undefined)
    Promise.all([
      api.getRun(runId),
      api.listGames(runId),
    ]).then(([r, g]) => {
      setRun(r)
      setGames(g.games)
    }).catch((e: unknown) => {
      setError(e instanceof Error ? e.message : 'Failed to load run')
    }).finally(() => setLoading(false))
  }, [runId])

  async function handleDelete() {
    if (!confirm('Delete this run and all its data?')) return
    setDeleting(true)
    try {
      await api.deleteRun(runId)
      window.location.href = '/runs'
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Delete failed')
      setDeleting(false)
    }
  }

  if (loading) {
    return (
      <div className="page" style={{ display: 'flex', justifyContent: 'center', paddingTop: 80 }}>
        <div className="spinner" />
      </div>
    )
  }

  if (error) {
    return (
      <div className="page">
        <div className="error-box">{error}</div>
        <Link to="/runs" className="btn btn-secondary">← Back to Runs</Link>
      </div>
    )
  }

  if (!run) {
    return (
      <div className="page" style={{ display: 'flex', justifyContent: 'center', paddingTop: 80 }}>
        <div className="spinner" />
      </div>
    )
  }

  const gens = run.generationsData ?? []
  const lastGen = gens[gens.length - 1]
  const llmNeverUsed =
    run.status === 'completed' &&
    gens.length > 0 &&
    gens.every((g) => (g.tokensUsed ?? 0) === 0)

  return (
    <div className="page">
      {/* LLM fallback warning */}
      {llmNeverUsed && (
        <div className="warning-banner">
          ⚠️ LLM was not used in this run — all guesses used the fallback word-picker.
          Check your model selection and API key. Results are not meaningful for research.
        </div>
      )}

      {/* Header */}
      <div className="flex items-center justify-between mb-24">
        <div>
          <div className="flex items-center gap-8 mb-8">
            <h1 style={{ fontSize: '1.6rem', fontWeight: 700 }}>Run #{run.id}</h1>
            {statusBadge(run.status)}
            {run.convergence && <ConvergenceBadge value={run.convergence} />}
          </div>
          <p className="text-secondary text-sm">
            {fmt(run.createdAt)} · {run.playerModel} → {run.reflectorModel} · seed {run.seed}
          </p>
        </div>
        <div className="flex gap-8">
          {run.status === 'running' && (
            <Link to={`/runs/${run.id}/live`} className="btn btn-secondary btn-sm">
              Watch Live
            </Link>
          )}
          <button className="btn btn-danger btn-sm" onClick={handleDelete} disabled={deleting}>
            {deleting ? 'Deleting…' : 'Delete'}
          </button>
        </div>
      </div>

      {/* Summary stats */}
      {lastGen && (
        <div className="card mb-24">
          <div style={{ display: 'flex', gap: 40, flexWrap: 'wrap' }}>
            <div>
              <div className="big-stat" style={{ color: 'var(--accent)' }}>
                {lastGen.solveRate != null ? `${(lastGen.solveRate * 100).toFixed(0)}%` : '—'}
              </div>
              <div className="big-stat-label">Final Solve Rate</div>
            </div>
            <div>
              <div className="big-stat">
                {lastGen.meanGuesses != null ? lastGen.meanGuesses.toFixed(1) : '—'}
              </div>
              <div className="big-stat-label">Mean Guesses</div>
            </div>
            <div>
              <div className="big-stat">
                {lastGen.meanInfoGain != null ? lastGen.meanInfoGain.toFixed(2) : '—'}
              </div>
              <div className="big-stat-label">Mean Info Gain</div>
            </div>
            <div>
              <div className="big-stat">{gens.length}</div>
              <div className="big-stat-label">Generations</div>
            </div>
            <div>
              <div className="big-stat">{run.seed}</div>
              <div className="big-stat-label">Seed</div>
            </div>
          </div>
        </div>
      )}

      {/* Tabs */}
      <div className="tabs">
        {(['charts', 'prompts', 'games', 'analysis'] as Tab[]).map((t) => (
          <button
            key={t}
            className={`tab-btn${tab === t ? ' active' : ''}`}
            onClick={() => setTab(t)}
          >
            {t === 'charts' ? 'Charts' : t === 'prompts' ? 'Prompt Timeline' : t === 'games' ? 'Games' : 'Analysis'}
          </button>
        ))}
      </div>

      {tab === 'charts' && (
        gens.length === 0
          ? <div className="empty-state"><p>No generations completed yet.</p></div>
          : <MetricsChart generations={gens} showBaselines />
      )}

      {tab === 'prompts' && <PromptTimeline generations={gens} />}

      {tab === 'games' && <GameList games={games} />}

      {tab === 'analysis' && (
        gens.length === 0
          ? <div className="empty-state"><p>No generations completed yet.</p></div>
          : <AnalysisTab runId={runId} generations={gens} maxGuesses={run.maxGuesses} />
      )}
    </div>
  )
}
