import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api } from '../api/client'
import RunForm from '../components/RunForm'
import type { CreateRunRequest } from '../api/types'

export default function NewRun() {
  const navigate = useNavigate()
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string>()

  async function handleSubmit(req: CreateRunRequest) {
    setLoading(true)
    setError(undefined)
    try {
      const run = await api.createRun(req)
      navigate(`/runs/${run.id}/live`)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to launch run')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="page-sm">
      <div style={{ marginBottom: 24 }}>
        <h1 style={{ fontSize: '1.6rem', fontWeight: 700, marginBottom: 6 }}>Launch Run</h1>
        <p style={{ color: 'var(--text-secondary)', fontSize: '0.9rem' }}>
          Configure a self-evolving Wordle agent experiment. The agent will play Wordle, then rewrite its own strategy after each generation.
        </p>
      </div>
      <div className="card">
        <RunForm onSubmit={handleSubmit} loading={loading} error={error} />
      </div>
    </div>
  )
}
