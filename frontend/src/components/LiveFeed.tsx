import { useState } from 'react'
import type { SSEEvent, SSEGuess } from '../api/types'

interface LogEntry {
  id: number
  guess: string
  feedback: string
  infoGain?: number
  reasoning?: string
  text: string
  kind: 'guess' | 'game_end' | 'gen_end' | 'run_end' | 'error'
}

interface Props {
  events: SSEEvent[]
}

function feedbackColor(c: string) {
  if (c.toUpperCase() === 'G') return 'mini-tile green'
  if (c.toUpperCase() === 'Y') return 'mini-tile yellow'
  return 'mini-tile gray'
}

function MiniTiles({ guess, feedback }: { guess: string; feedback: string }) {
  return (
    <div className="mini-tiles">
      {Array.from({ length: 5 }, (_, i) => (
        <div key={i} className={feedbackColor(feedback[i] ?? 'X')}>
          {(guess[i] ?? '').toUpperCase()}
        </div>
      ))}
    </div>
  )
}

export default function LiveFeed({ events }: Props) {
  const [expanded, setExpanded] = useState<Set<number>>(new Set())

  function toggleExpand(id: number) {
    setExpanded((s) => {
      const next = new Set(s)
      next.has(id) ? next.delete(id) : next.add(id)
      return next
    })
  }

  const entries: LogEntry[] = events.map((e, i) => {
    if (e.type === 'guess') {
      const ge = e as SSEGuess
      return {
        id: i,
        guess: ge.guess,
        feedback: ge.feedback,
        infoGain: ge.infoGain,
        reasoning: ge.reasoning,
        text: `Game ${ge.gameId} · Turn ${ge.turn + 1}`,
        kind: 'guess',
      }
    }
    if (e.type === 'game_end') {
      return {
        id: i,
        guess: '',
        feedback: '',
        text: `Game ${e.gameId} ended — ${e.won ? `✓ won in ${e.numGuesses}` : `✗ lost (${e.answer})`}`,
        kind: 'game_end',
      }
    }
    if (e.type === 'gen_end') {
      return {
        id: i,
        guess: '',
        feedback: '',
        text: `Gen ${e.genIndex} done — solve ${(e.solveRate * 100).toFixed(0)}% · avg ${e.meanGuesses.toFixed(1)} guesses`,
        kind: 'gen_end',
      }
    }
    if (e.type === 'run_end') {
      return { id: i, guess: '', feedback: '', text: `Run ${e.runId} ${e.status}`, kind: 'run_end' }
    }
    return { id: i, guess: '', feedback: '', text: `Error: ${e.message}`, kind: 'error' }
  })

  return (
    <div className="feed-log">
      {entries.length === 0 && (
        <span style={{ color: 'var(--text-secondary)' }}>Waiting for events…</span>
      )}
      {entries.map((e) => (
        <div key={e.id}>
          <div className="feed-item">
            {e.kind === 'guess' && e.feedback && (
              <MiniTiles guess={e.guess} feedback={e.feedback} />
            )}
            <span style={{ color: e.kind === 'error' ? 'var(--danger)' : e.kind === 'gen_end' || e.kind === 'run_end' ? 'var(--text)' : 'var(--text-secondary)' }}>
              {e.text}
            </span>
            {e.kind === 'guess' && e.infoGain !== undefined && (
              <span style={{ marginLeft: 'auto', color: 'var(--accent)', fontSize: '0.75rem' }}>
                +{e.infoGain.toFixed(2)} bits
              </span>
            )}
            {e.kind === 'guess' && e.reasoning && (
              <button
                onClick={() => toggleExpand(e.id)}
                style={{ marginLeft: 8, background: 'none', border: 'none', cursor: 'pointer', color: 'var(--text-secondary)', fontSize: '0.72rem', padding: '0 4px' }}
              >
                {expanded.has(e.id) ? '▲' : '▼'} thinking
              </button>
            )}
          </div>
          {e.kind === 'guess' && e.reasoning && expanded.has(e.id) && (
            <div style={{ paddingLeft: 8, paddingBottom: 6, fontSize: '0.75rem', fontFamily: 'monospace', color: 'var(--text-secondary)', whiteSpace: 'pre-wrap', borderLeft: '2px solid #e0e0e0', marginLeft: 4, marginBottom: 4 }}>
              {e.reasoning}
            </div>
          )}
        </div>
      ))}
    </div>
  )
}
