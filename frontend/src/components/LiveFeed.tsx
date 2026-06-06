import type { SSEEvent } from '../api/types'

interface LogEntry {
  id: number
  guess: string
  feedback: string
  infoGain?: number
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
  const entries: LogEntry[] = events.map((e, i) => {
    if (e.type === 'guess') {
      return {
        id: i,
        guess: e.guess,
        feedback: e.feedback,
        infoGain: e.infoGain,
        text: `Game ${e.gameId} · Turn ${e.turn + 1}`,
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
        <div key={e.id} className="feed-item">
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
        </div>
      ))}
    </div>
  )
}
