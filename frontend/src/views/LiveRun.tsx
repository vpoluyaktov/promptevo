import { useState, useEffect, useRef } from 'react'
import { useParams, Link } from 'react-router-dom'
import { api } from '../api/client'
import { useRunStream } from '../hooks/useRunStream'
import GameBoard from '../components/GameBoard'
import LiveFeed from '../components/LiveFeed'
import PromptDiff from '../components/PromptDiff'
import type { Run, SSEEvent, SSEGuess, SSEGameEnd, SSEGenEnd } from '../api/types'

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
  tokensUsed?: number
}

export default function LiveRun() {
  const { id } = useParams<{ id: string }>()
  const runId = Number(id)

  const [run, setRun] = useState<Run | null>(null)
  const [events, setEvents] = useState<SSEEvent[]>([])
  const [currentGame, setCurrentGame] = useState<CurrentGame>({ guesses: [], feedbacks: [], animateLastRow: false })
  const currentGameIdRef = useRef<number | null>(null)
  const resetTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const [currentPrompt, setCurrentPrompt] = useState('')
  const [prevPrompt, setPrevPrompt] = useState('')
  const [showDiff, setShowDiff] = useState(false)
  const [genSummaries, setGenSummaries] = useState<GenSummary[]>([])
  const [gamesCompleted, setGamesCompleted] = useState(0)
  const [gamesWon, setGamesWon] = useState(0)
  const [gamesLost, setGamesLost] = useState(0)
  // Deduplicates game_end events vs initial listGames load (avoids double-count on reconnect).
  const countedGameIdsRef = useRef<Set<number>>(new Set())
  const [latestReasoning, setLatestReasoning] = useState('')
  const [isComplete, setIsComplete] = useState(false)
  const [streamActive, setStreamActive] = useState(true)

  useEffect(() => {
    Promise.all([
      api.getRun(runId),
    ]).then(([r]) => {
      setRun(r)

      if (r.status === 'completed' || r.status === 'failed' || r.status === 'stopped') {
        setIsComplete(true)
        setStreamActive(false)
      }

      const gens = r.generationsData ?? []
      // Generations with stats = completed; without = currently in progress
      const completedGens = gens.filter((g) => g.solveRate != null)
      const inProgressGen = gens.find((g) => g.solveRate == null)

      // Restore gen summaries from completed generations
      setGenSummaries(completedGens.map((g) => ({
        index: g.genIndex,
        solveRate: g.solveRate!,
        meanGuesses: g.meanGuesses!,
        prompt: g.promptText,
        tokensUsed: g.tokensUsed,
      })))

      // Restore current prompt and prompt diff
      const lastGen = gens[gens.length - 1]
      if (lastGen) setCurrentPrompt(lastGen.promptText)

      if (completedGens.length >= 2) {
        setPrevPrompt(completedGens[completedGens.length - 2].promptText)
        setShowDiff(true)
      } else if (completedGens.length === 1 && inProgressGen) {
        setPrevPrompt(completedGens[0].promptText)
        if (inProgressGen.promptText !== completedGens[0].promptText) setShowDiff(true)
      }

      // Count games already completed in the in-progress generation (LLM only).
      // Seeds the dedup set so SSE game_end events for the same games aren't double-counted.
      if (inProgressGen) {
        api.listGames(runId, inProgressGen.genIndex).then((g) => {
          const llmGames = g.games.filter((game) => game.agentType === 'llm')
          llmGames.forEach((game) => countedGameIdsRef.current.add(game.id))
          setGamesCompleted(llmGames.length)
          setGamesWon(llmGames.filter((game) => game.won).length)
          setGamesLost(llmGames.filter((game) => !game.won).length)
        }).catch(() => {})
      }
    }).catch(() => {})
  }, [runId])

  useRunStream(streamActive ? runId : null, (event) => {
    setEvents((prev) => [...prev.slice(-2000), event])

    if (event.type === 'guess') {
      const e = event as SSEGuess
      if (currentGameIdRef.current !== null && currentGameIdRef.current !== e.gameId) {
        // New game started before the reset timer fired — cancel it and start fresh
        if (resetTimerRef.current !== null) {
          clearTimeout(resetTimerRef.current)
          resetTimerRef.current = null
        }
        setCurrentGame({ guesses: [e.guess], feedbacks: [e.feedback], animateLastRow: true })
      } else {
        setCurrentGame((g) => ({
          guesses: [...g.guesses, e.guess],
          feedbacks: [...g.feedbacks, e.feedback],
          animateLastRow: true,
        }))
      }
      currentGameIdRef.current = e.gameId
      if (e.reasoning) setLatestReasoning(e.reasoning)
    }

    if (event.type === 'game_end') {
      const ge = event as SSEGameEnd
      if (countedGameIdsRef.current.has(ge.gameId)) return
      countedGameIdsRef.current.add(ge.gameId)
      setGamesCompleted((n) => n + 1)
      if (ge.won) setGamesWon((n) => n + 1)
      else setGamesLost((n) => n + 1)
      if (resetTimerRef.current !== null) clearTimeout(resetTimerRef.current)
      resetTimerRef.current = setTimeout(() => {
        currentGameIdRef.current = null
        setCurrentGame({ guesses: [], feedbacks: [], animateLastRow: false })
        resetTimerRef.current = null
      }, 1500)
    }

    if (event.type === 'gen_end') {
      const e = event as SSEGenEnd
      setGenSummaries((prev) => [...prev, {
        index: e.genIndex,
        solveRate: e.solveRate,
        meanGuesses: e.meanGuesses,
        prompt: e.prompt,
        tokensUsed: e.tokensUsed,
      }])
      setPrevPrompt(currentPrompt)
      setCurrentPrompt(e.prompt)
      setShowDiff(true)
      setGamesCompleted(0)
      setGamesWon(0)
      setGamesLost(0)
      countedGameIdsRef.current = new Set()
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

      <div className="grid-2 mb-24">
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
          <div style={{ display: 'flex', gap: 24, flexWrap: 'wrap', marginBottom: 24 }}>
            <div>
              <div className="big-stat">{genSummaries.length}</div>
              <div className="big-stat-label">Gens Done</div>
            </div>
            <div>
              <div className="big-stat">{gamesCompleted}</div>
              <div className="big-stat-label">Games / {totalGames}</div>
            </div>
            <div>
              <div className="big-stat" style={{ color: 'var(--accent)' }}>{gamesWon}</div>
              <div className="big-stat-label">Won</div>
            </div>
            <div>
              <div className="big-stat" style={{ color: 'var(--danger)' }}>{gamesLost}</div>
              <div className="big-stat-label">Lost</div>
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

          <div className="prompt-card" style={{ marginBottom: 12 }}>
            <div className="prompt-card-header">Agent Thinking — Last Move</div>
            <div className="prompt-card-body" style={{ fontFamily: 'monospace', fontSize: '0.8rem', whiteSpace: 'pre-wrap', maxHeight: 180, overflowY: 'auto' }}>
              {latestReasoning || <span style={{ color: 'var(--text-secondary)' }}>Waiting for first move…</span>}
            </div>
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

      {/* Fallback warnings for completed generations */}
      {genSummaries.some((g) => g.tokensUsed === 0) && (
        <div className="warning-banner" style={{ marginBottom: 24 }}>
          {genSummaries
            .filter((g) => g.tokensUsed === 0)
            .map((g) => (
              <div key={g.index}>
                ⚠️ Generation {g.index} used no LLM calls — fallback word-picker was active.
              </div>
            ))}
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
