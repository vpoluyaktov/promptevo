// Run
export interface Run {
  id: number
  createdAt: string
  playerModel: string
  reflectorModel: string
  temperature: number
  seed: number
  generations: number
  gamesPerGen: number
  wordSampleSize: number
  status: 'pending' | 'running' | 'completed' | 'failed' | 'stopped'
}

export interface RunDetail extends Run {
  convergence?: 'improving' | 'oscillating' | 'stable'
  generationsData?: Generation[]
}

// Generation
export interface Generation {
  genIndex: number
  promptText: string
  promptLen: number
  reflectionText?: string
  solveRate?: number
  meanGuesses?: number
  meanInfoGain?: number
  violationRate?: number
  tokensUsed?: number
}

// Game
export interface Game {
  id: number
  genIndex: number
  answer: string
  won: boolean
  numGuesses: number
  infoGainTotal: number
  violations: number
  agentType: 'llm' | 'random' | 'frequency' | 'entropy'
}

// Guess
export interface Guess {
  id: number
  turnIndex: number
  guess: string
  feedback: string
  infoGainBits: number
  reasoningText?: string
}

// SSE event types
export interface SSEGuess {
  type: 'guess'
  gameId: number
  genIndex: number
  turn: number
  guess: string
  feedback: string
  infoGain: number
}

export interface SSEGameEnd {
  type: 'game_end'
  gameId: number
  genIndex: number
  won: boolean
  numGuesses: number
  answer: string
}

export interface SSEGenEnd {
  type: 'gen_end'
  genIndex: number
  solveRate: number
  meanGuesses: number
  meanInfoGain: number
  violationRate: number
  prompt: string
}

export interface SSERunEnd {
  type: 'run_end'
  runId: number
  status: string
  convergence?: string
}

export interface SSEError {
  type: 'error'
  message: string
}

export type SSEEvent = SSEGuess | SSEGameEnd | SSEGenEnd | SSERunEnd | SSEError

// API request/response wrappers
export interface ListRunsResponse {
  runs: Run[]
}

export interface ListModelsResponse {
  models: string[]
}

export interface ListGamesResponse {
  games: Game[]
}

export interface ListGuessesResponse {
  guesses: Guess[]
}

export interface CreateRunRequest {
  playerModel: string
  reflectorModel: string
  temperature: number
  seed: number
  generations: number
  gamesPerGen: number
  wordSampleSize: number
  includeBaselines: boolean
}
