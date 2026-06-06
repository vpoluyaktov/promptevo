import type {
  Run,
  RunDetail,
  ListRunsResponse,
  ListModelsResponse,
  ListGamesResponse,
  ListGuessesResponse,
  CreateRunRequest,
} from './types'

const BASE = '/api'

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    headers: { 'Content-Type': 'application/json' },
    ...options,
  })
  if (!res.ok) {
    const body = await res.json().catch(() => ({ error: res.statusText }))
    throw new Error((body as { error: string }).error ?? res.statusText)
  }
  return res.json() as Promise<T>
}

export const api = {
  listModels(): Promise<ListModelsResponse> {
    return request('/models')
  },

  listRuns(): Promise<ListRunsResponse> {
    return request('/runs')
  },

  getRun(id: number): Promise<RunDetail> {
    return request(`/runs/${id}`)
  },

  createRun(body: CreateRunRequest): Promise<Run> {
    return request('/runs', {
      method: 'POST',
      body: JSON.stringify(body),
    })
  },

  deleteRun(id: number): Promise<{ deleted: boolean }> {
    return request(`/runs/${id}`, { method: 'DELETE' })
  },

  listGames(runId: number, genIndex?: number): Promise<ListGamesResponse> {
    const q = genIndex !== undefined ? `?gen=${genIndex}` : ''
    return request(`/runs/${runId}/games${q}`)
  },

  listGuesses(gameId: number): Promise<ListGuessesResponse> {
    return request(`/games/${gameId}/guesses`)
  },
}
