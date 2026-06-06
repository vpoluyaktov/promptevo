import { useState, useEffect } from 'react'
import type { Game, Guess } from '../api/types'
import { api } from '../api/client'
import GameBoard from './GameBoard'

interface Props {
  games: Game[]
}

interface GameModalProps {
  game: Game
  onClose: () => void
}

function feedbackToArrays(guesses: Guess[]) {
  return {
    guessWords: guesses.map((g) => g.guess),
    feedbacks: guesses.map((g) => g.feedback),
  }
}

function GameModal({ game, onClose }: GameModalProps) {
  const [guesses, setGuesses] = useState<Guess[] | null>(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    api.listGuesses(game.id)
      .then((r) => setGuesses(r.guesses))
      .finally(() => setLoading(false))
  }, [game.id])

  const { guessWords, feedbacks } = guesses ? feedbackToArrays(guesses) : { guessWords: [], feedbacks: [] }

  return (
    <div className="modal-overlay" onClick={onClose}>
      <div className="modal" onClick={(e) => e.stopPropagation()}>
        <button className="modal-close" onClick={onClose}>✕</button>
        <h3 style={{ marginBottom: 16 }}>
          Game #{game.id} — Answer: <strong>{game.answer.toUpperCase()}</strong>
          {' '}{game.won ? '✓' : '✗'}
        </h3>
        {loading && <div className="spinner" />}
        {!loading && guesses && (
          <div style={{ display: 'flex', gap: 32, flexWrap: 'wrap' }}>
            <GameBoard guesses={guessWords} feedbacks={feedbacks} />
            <div style={{ flex: 1, minWidth: 200 }}>
              {guesses.map((g) => (
                <div key={g.id} style={{ marginBottom: 12 }}>
                  <div style={{ fontWeight: 600, textTransform: 'uppercase', fontSize: '0.9rem' }}>
                    {g.turnIndex + 1}. {g.guess}
                    <span style={{ color: 'var(--text-secondary)', fontWeight: 400, marginLeft: 8 }}>
                      {g.feedback} (+{g.infoGainBits.toFixed(2)} bits)
                    </span>
                  </div>
                  {g.reasoningText ? (
                    <div style={{ fontSize: '0.78rem', color: 'var(--text-secondary)', marginTop: 4, fontFamily: 'monospace', whiteSpace: 'pre-wrap' }}>
                      {g.reasoningText.slice(0, 300)}{g.reasoningText.length > 300 ? '…' : ''}
                    </div>
                  ) : (
                    <span style={{ fontSize: '0.72rem', color: 'var(--text-secondary)', marginTop: 4, display: 'inline-block', background: '#f0f0f0', borderRadius: 3, padding: '1px 6px' }}>
                      fallback
                    </span>
                  )}
                </div>
              ))}
            </div>
          </div>
        )}
      </div>
    </div>
  )
}

export default function GameList({ games }: Props) {
  const [selected, setSelected] = useState<Game | null>(null)

  if (games.length === 0) {
    return <div className="empty-state"><p>No games yet.</p></div>
  }

  return (
    <>
      <div className="table-wrap">
        <table>
          <thead>
            <tr>
              <th>Gen</th>
              <th>Answer</th>
              <th>Won</th>
              <th>Guesses</th>
              <th className="col-game-hide">Info Gain</th>
              <th className="col-game-hide">Violations</th>
              <th className="col-game-hide">Agent</th>
            </tr>
          </thead>
          <tbody>
            {games.map((g) => (
              <tr key={g.id} className="clickable" onClick={() => setSelected(g)}>
                <td>{g.genIndex}</td>
                <td style={{ textTransform: 'uppercase', fontFamily: 'monospace', fontWeight: 600 }}>{g.answer}</td>
                <td>{g.won ? <span style={{ color: 'var(--accent)' }}>✓</span> : <span style={{ color: 'var(--danger)' }}>✗</span>}</td>
                <td>{g.numGuesses}</td>
                <td className="col-game-hide">{g.infoGainTotal.toFixed(2)}</td>
                <td className="col-game-hide">{g.violations}</td>
                <td className="col-game-hide"><span className="badge badge-stable" style={{ background: '#f5f5f5', color: 'var(--text-secondary)' }}>{g.agentType}</span></td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {selected && <GameModal game={selected} onClose={() => setSelected(null)} />}
    </>
  )
}
