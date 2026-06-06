import { useState, useEffect } from 'react'
import { useParams, Link } from 'react-router-dom'
import { api } from '../api/client'
import { useRunStream } from '../hooks/useRunStream'
import GameBoard from '../components/GameBoard'
import LiveFeed from '../components/LiveFeed'
import PromptDiff from '../components/PromptDiff'
import type { Run, SSEEvent, SSEGuess, SSEGenEnd } from '../api/types'

interface CurrentGame {
  guesses: string[]
  feedbacks: string[]
  animateLastRow: boolean
}

interface GenSummary {
  index: number
  solveRate: number
  meanGuesses: number
  prompt: string
}

export default function LiveRun() {
  const { id } = useParams<{ id: string }>()
  const runId = Number(id)

  const [run, setRun] = useState<Run | null>(null)
  const [events, setEvents] = useState<SSEEvent[]>([])
  const [currentGame, setCurrentGame] = useState<CurrentGame>({ guesses: [], feedbacks: [], animateLastRow: false })
  const [currentPrompt, setCurrentPrompt] = useState('')
  const [prevPrompt, setPrevPrompt] = useState('')
  const [showDiff, setShowDiff] = useState(false)
  const [genSummaries, setGenSummaries] = useState<GenSummary[]>([])
  const [gamesCompleted, setGamesCompleted] = useState(0)
  const [isComplete, setIsComplete] = useState(false)
  const [streamActive, setStreamActive] = useState(true)

  useEffect(() => {
    api.getRun(runId).then((r) => {
      setRun(r)
      if (r.status === 'completed' || r.status === 'failed') {
        setIsComplete(true)
        setStreamActive(false)
      }
      // Pre-populate prompt from latest generation if available
      if (r.generationsData && r.generationsData.length > 0) {
        const last = r.generationsData[r.generationsData.length - 1]
        setCurrentPrompt(last.promptText)
      }
    }).catch(() => { /* run might not exist yet */ })
  }, [runId])

  useRunStream(streamActive ? runId : null, (event) => {
    setEvents((prev) => [...prev.slice(-200), event])

    if (event.type === 'guess') {
      const e = event as SSEGuess
      setCurrentGame((g) => {
        const newGuesses = [...g.guesses, e.guess]
        const newFeedbacks = [...g.feedbacks, e.feedback]
        return { guesses: newGuesses, feedbacks: newFeedbacks, animateLastRow: true }
      })
    }

    if (event.type === 'game_end') {
      setGamesCompleted((n) => n + 1)
      // Reset board for next game after short delay
      setTimeout(() => {
        setCurrentGame({ guesses: [], feedbacks: [], animateLastRow: false })
      }, 1500)
    }

    if (event.type === 'gen_end') {
      const e = event as SSEGenEnd
      setGenSummaries((prev) => [...prev, {
        index: e.genIndex,
        solveRate: e.solveRate,
        meanGuesses: e.meanGuesses,
        prompt: e.prompt,
      }])
      setPrevPrompt(currentPrompt)
      setCurrentPrompt(e.prompt)
      setShowDiff(true)
      setGamesCompleted(0)
    }

    if (event.type === 'run_end') {
      setIsComplete(true)
      setStreamActive(false)
    }
  })

  const latestGen = genSummaries[genSummaries.length - 1]
  const totalGames = run?.gamesPerGen ?? '?'

  return (
    <div className="page">
      <div className="flex items-center justify-between mb-24">
        <div>
          <h1 style={{ fontSize: '1.6rem', fontWeight: 700, marginBottom: 4 }}>
            Run #{runId} — Live
          </h1>
          {run && (
            <p className="text-secondary text-sm">
              {run.playerModel} → {run.reflectorModel} · seed {run.seed}
            </p>
          )}
        </div>
        <Link to={`/runs/${runId}`} className="btn btn-secondary btn-sm">
          View Details →
        </Link>
      </div>

      {isComplete && (
        <div className="success-banner">
          <h3>Run Complete</h3>
          <p>
            <Link to={`/runs/${runId}`}>View full analysis →</Link>
          </p>
        </div>
      )}

      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 24, marginBottom: 24 }}>
        {/* Left: Wordle board */}
        <div className="card">
          <h3 style={{ marginBottom: 16, fontWeight: 600, fontSize: '1rem' }}>Current Game</h3>
          <div style={{ display: 'flex', justifyContent: 'center' }}>
            <GameBoard
              guesses={currentGame.guesses}
              feedbacks={currentGame.feedbacks}
              animateLastRow={currentGame.animateLastRow}
            />
          </div>
        </div>

        {/* Right: Stats */}
        <div className="card">
          <h3 style={{ marginBottom: 16, fontWeight: 600, fontSize: '1rem' }}>Progress</h3>
          <div style={{ display: 'flex', gap: 24, marginBottom: 24 }}>
            <div>
              <div className="big-stat">{genSummaries.length}</div>
              <div className="big-stat-label">Gens Done</div>
            </div>
            <div>
              <div className="big-stat">{gamesCompleted}</div>
              <div className="big-stat-label">Games / {totalGames}</div>
            </div>
            {latestGen && (
              <div>
                <div className="big-stat" style={{ color: 'var(--accent)' }}>
                  {(latestGen.solveRate * 100).toFixed(0)}%
                </div>
                <div className="big-stat-label">Solve Rate</div>
              </div>
            )}
          </div>

          {currentPrompt && (
            <div className="prompt-card">
              <div className="prompt-card-header">Current Strategy Prompt</div>
              <div className="prompt-card-body">{currentPrompt}</div>
            </div>
          )}
        </div>
      </div>

      {/* Prompt diff */}
      {showDiff && prevPrompt && currentPrompt && prevPrompt !== currentPrompt && (
        <div className="card mb-24">
          <div className="flex items-center justify-between mb-16">
            <h3 style={{ fontWeight: 600, fontSize: '1rem' }}>Prompt Evolution</h3>
            <button className="btn btn-secondary btn-sm" onClick={() => setShowDiff(false)}>Hide</button>
          </div>
          <PromptDiff oldPrompt={prevPrompt} newPrompt={currentPrompt} />
        </div>
      )}

      {/* Event log */}
      <div className="card">
        <h3 style={{ marginBottom: 12, fontWeight: 600, fontSize: '1rem' }}>Event Log</h3>
        <LiveFeed events={events} />
      </div>
    </div>
  )
}
