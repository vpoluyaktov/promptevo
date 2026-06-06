import { useState, useEffect } from 'react'
import { api } from '../api/client'
import type { CreateRunRequest } from '../api/types'

interface Props {
  onSubmit: (req: CreateRunRequest) => Promise<void>
  loading: boolean
  error?: string
}

interface FormState {
  playerModel: string
  reflectorModel: string
  seed: string
  generations: string
  gamesPerGen: string
  temperature: string
  wordSampleSize: string
  includeBaselines: boolean
}

const DEFAULTS: FormState = {
  playerModel: '',
  reflectorModel: '',
  seed: String(Math.floor(Math.random() * 999999) + 1),
  generations: '5',
  gamesPerGen: '10',
  temperature: '0.7',
  wordSampleSize: '20',
  includeBaselines: false,
}

function randomSeed() {
  return String(Math.floor(Math.random() * 999999) + 1)
}

function validate(f: FormState): Partial<Record<keyof FormState, string>> {
  const errs: Partial<Record<keyof FormState, string>> = {}
  if (!f.playerModel) errs.playerModel = 'Required'
  if (!f.reflectorModel) errs.reflectorModel = 'Required'
  const seed = Number(f.seed)
  if (!f.seed || !Number.isInteger(seed) || seed < 1) errs.seed = 'Must be a positive integer'
  const gens = Number(f.generations)
  if (!Number.isInteger(gens) || gens < 1 || gens > 20) errs.generations = 'Must be 1–20'
  const gpg = Number(f.gamesPerGen)
  if (!Number.isInteger(gpg) || gpg < 1 || gpg > 50) errs.gamesPerGen = 'Must be 1–50'
  const temp = Number(f.temperature)
  if (isNaN(temp) || temp < 0 || temp > 2) errs.temperature = 'Must be 0.0–2.0'
  const wss = Number(f.wordSampleSize)
  if (!Number.isInteger(wss) || wss < 5 || wss > 100) errs.wordSampleSize = 'Must be 5–100'
  return errs
}

export default function RunForm({ onSubmit, loading, error }: Props) {
  const [models, setModels] = useState<string[]>([])
  const [form, setForm] = useState<FormState>(DEFAULTS)
  const [touched, setTouched] = useState<Partial<Record<keyof FormState, boolean>>>({})
  const [submitted, setSubmitted] = useState(false)

  useEffect(() => {
    api.listModels().then((r) => {
      setModels(r.models)
      setForm((f) => ({
        ...f,
        playerModel: r.models[0] ?? '',
        reflectorModel: r.models[0] ?? '',
      }))
    }).catch(() => {
      // use fallback list
      const fallback = ['openai/gpt-4o-mini', 'anthropic/claude-3-haiku']
      setModels(fallback)
      setForm((f) => ({ ...f, playerModel: fallback[0], reflectorModel: fallback[0] }))
    })
  }, [])

  const errs = validate(form)
  const showErr = (k: keyof FormState) => (touched[k] || submitted) && errs[k]

  function set<K extends keyof FormState>(k: K, v: FormState[K]) {
    setForm((f) => ({ ...f, [k]: v }))
    setTouched((t) => ({ ...t, [k]: true }))
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setSubmitted(true)
    if (Object.keys(errs).length > 0) return
    await onSubmit({
      playerModel: form.playerModel,
      reflectorModel: form.reflectorModel,
      temperature: Number(form.temperature),
      seed: Number(form.seed),
      generations: Number(form.generations),
      gamesPerGen: Number(form.gamesPerGen),
      wordSampleSize: Number(form.wordSampleSize),
      includeBaselines: form.includeBaselines,
    })
  }

  return (
    <form onSubmit={handleSubmit}>
      {error && <div className="error-box">{error}</div>}

      <div className="form-row">
        <div className="form-group">
          <label className="form-label">Player Model</label>
          <select className="form-control" value={form.playerModel} onChange={(e) => set('playerModel', e.target.value)}>
            {models.map((m) => <option key={m} value={m}>{m}</option>)}
          </select>
          {showErr('playerModel') && <div className="form-error">{errs.playerModel}</div>}
        </div>
        <div className="form-group">
          <label className="form-label">Reflector Model</label>
          <select className="form-control" value={form.reflectorModel} onChange={(e) => set('reflectorModel', e.target.value)}>
            {models.map((m) => <option key={m} value={m}>{m}</option>)}
          </select>
          {showErr('reflectorModel') && <div className="form-error">{errs.reflectorModel}</div>}
        </div>
      </div>

      <div className="form-row">
        <div className="form-group">
          <label className="form-label">Seed</label>
          <div style={{ display: 'flex', gap: 8 }}>
            <input
              type="number"
              className="form-control"
              value={form.seed}
              onChange={(e) => set('seed', e.target.value)}
              min={1}
            />
            <button type="button" className="btn btn-secondary" onClick={() => set('seed', randomSeed())}>
              🎲
            </button>
          </div>
          {showErr('seed') && <div className="form-error">{errs.seed}</div>}
        </div>
        <div className="form-group">
          <label className="form-label">Temperature</label>
          <input
            type="number"
            className="form-control"
            value={form.temperature}
            onChange={(e) => set('temperature', e.target.value)}
            min={0} max={2} step={0.1}
          />
          {showErr('temperature') && <div className="form-error">{errs.temperature}</div>}
        </div>
      </div>

      <div className="form-row">
        <div className="form-group">
          <label className="form-label">Generations</label>
          <input
            type="number"
            className="form-control"
            value={form.generations}
            onChange={(e) => set('generations', e.target.value)}
            min={1} max={20}
          />
          <div className="form-hint">1–20 generations</div>
          {showErr('generations') && <div className="form-error">{errs.generations}</div>}
        </div>
        <div className="form-group">
          <label className="form-label">Games per Generation</label>
          <input
            type="number"
            className="form-control"
            value={form.gamesPerGen}
            onChange={(e) => set('gamesPerGen', e.target.value)}
            min={1} max={50}
          />
          <div className="form-hint">1–50 games</div>
          {showErr('gamesPerGen') && <div className="form-error">{errs.gamesPerGen}</div>}
        </div>
      </div>

      <div className="form-row">
        <div className="form-group">
          <label className="form-label">Word Sample Size</label>
          <input
            type="number"
            className="form-control"
            value={form.wordSampleSize}
            onChange={(e) => set('wordSampleSize', e.target.value)}
            min={5} max={100}
          />
          <div className="form-hint">5–100 words per generation</div>
          {showErr('wordSampleSize') && <div className="form-error">{errs.wordSampleSize}</div>}
        </div>
        <div className="form-group" style={{ paddingTop: 28 }}>
          <label style={{ display: 'flex', alignItems: 'center', gap: 8, cursor: 'pointer' }}>
            <input
              type="checkbox"
              checked={form.includeBaselines}
              onChange={(e) => set('includeBaselines', e.target.checked)}
            />
            <span className="form-label" style={{ margin: 0 }}>Include baseline players in gen 0</span>
          </label>
          <div className="form-hint">Adds random / frequency / entropy agents for comparison</div>
        </div>
      </div>

      <button type="submit" className="btn btn-primary" disabled={loading} style={{ marginTop: 8, padding: '10px 24px' }}>
        {loading ? 'Launching…' : 'Launch Run'}
      </button>
    </form>
  )
}
