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
  maxGuesses: number
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
  playerTokens?: number
  reflectorTokens?: number
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
  reasoning?: string
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
  tokensUsed?: number
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
  maxGuesses: number
  includeBaselines: boolean
}

export interface SolveRateCIPoint {
  genIndex: number
  n: number
  wins: number
  solveRate: number
  ciLower: number
  ciUpper: number
}
export interface WinDistPoint {
  genIndex: number
  total: number
  wonByTurn: Record<string, number>
  lost: number
}
export interface TurnInfoGainPoint {
  genIndex: number
  turnIndex: number
  meanInfoGain: number
  n: number
}
export interface OpeningWordRow { word: string; count: number }
export interface OpeningWordsPoint { genIndex: number; words: OpeningWordRow[] }
export interface RemainingCandPoint {
  gameId: number
  genIndex: number
  answer: string
  remainingCandidates: number
  numGuesses: number
}
export interface TokenEfficiencyPoint {
  genIndex: number
  playerTokens: number
  reflectorTokens: number
  tokensUsed: number
  split: boolean
}
export interface ReasoningPoint {
  gameId: number
  genIndex: number
  won: boolean
  reasoningChars: number
  numGuesses: number
}
export interface WordDifficultyPoint {
  answer: string
  games: number
  wins: number
  winRate: number
}
export interface AnalyticsResponse {
  runId: number
  maxGuesses: number
  meta: { totalLlmGames: number; generations: number }
  solveRateCI: SolveRateCIPoint[]
  winDistribution: WinDistPoint[]
  turnInfoGain: TurnInfoGainPoint[]
  openingWords: OpeningWordsPoint[]
  remainingCandidates: RemainingCandPoint[]
  tokenEfficiency: TokenEfficiencyPoint[]
  reasoningVerbosity: ReasoningPoint[]
  wordDifficulty: WordDifficultyPoint[]
}
