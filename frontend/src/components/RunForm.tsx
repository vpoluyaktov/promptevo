import { useState, useEffect, useRef } from 'react'
import { api } from '../api/client'
import type { CreateRunRequest, DefaultPromptsResponse } from '../api/types'

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
  maxGuesses: string
  includeBaselines: boolean
}

const STORAGE_KEY = 'promptevo_run_settings'

const DEFAULTS: FormState = {
  playerModel: '',
  reflectorModel: '',
  seed: String(Math.floor(Math.random() * 999999) + 1),
  generations: '20',
  gamesPerGen: '50',
  temperature: '0.3',
  wordSampleSize: '50',
  maxGuesses: '3',
  includeBaselines: false,
}

const FIELD_INFO: Record<string, { title: string; body: string }> = {
  playerModel: {
    title: 'Player Model',
    body: 'The LLM that plays Wordle every game. It receives the evolving strategy prompt and must guess a 5-letter word within the guess budget. Smaller/faster models keep experiment cost low; larger models produce richer reasoning traces that the reflector can learn from. A mismatch between player capability and prompt complexity is a common failure mode — start with the same model for both roles.',
  },
  reflectorModel: {
    title: 'Reflector Model',
    body: 'The LLM that acts as the evolutionary operator. After each generation it reads the game transcripts, win/loss statistics, and the current strategy prompt, then rewrites the prompt to improve the next generation. The reflector is called once per generation (much cheaper than the player). A stronger model here typically produces more targeted, surgical edits. Using the same model as the player is fine; using a larger model as reflector with a smaller player is a common cost-saving pattern.',
  },
  seed: {
    title: 'Random Seed',
    body: 'Controls which words are sampled from the answer list for each generation. The same seed produces the same word sequence across all generations, ensuring the player always faces the same vocabulary and making solve-rate changes attributable to prompt improvements rather than word-difficulty variance. Change the seed to run a fresh experiment on a different vocabulary sample; keep it fixed to reproduce a prior run exactly.',
  },
  temperature: {
    title: 'Temperature',
    body: 'Controls the randomness of both the player and reflector LLM responses. 0.0 = fully deterministic (same prompt → same output every time); 1.0 = standard sampling; above 1.0 increases creativity but also noise. For prompt evolution, 0.5–0.8 is typical: low enough to get stable, reproducible guesses that reflect the prompt, high enough to let the player explore alternatives. Very high temperatures (>1.2) make the player unpredictable and blur the signal the reflector needs.',
  },
  generations: {
    title: 'Generations',
    body: 'The number of prompt-evolution cycles to run. Each generation: (1) the player plays all games with the current prompt, (2) the reflector analyzes results and writes a new prompt. More generations give the reflector more chances to refine the strategy but cost proportionally more tokens. 5–10 generations is a typical first experiment; 15–20 is needed to observe convergence behavior. The experiment stops early if the run is manually stopped.',
  },
  gamesPerGen: {
    title: 'Games per Generation',
    body: 'How many Wordle games the player plays in each generation. This is the sample size for the solve-rate estimate. Larger samples make the Wilson confidence interval narrower and prompt improvements easier to detect statistically — a jump from 60% to 70% is convincing at n=50 but could be noise at n=10. The tradeoff is cost: each game consumes player tokens proportional to the number of guesses. 10–20 games is a fast exploratory run; 30–50 gives publication-quality signal.',
  },
  wordSampleSize: {
    title: 'Word Sample Size',
    body: 'How many answer words are drawn from the full answer list for each generation (using the seed for reproducibility). All generations use the same sampled words, so the player faces a consistent vocabulary across the experiment. A smaller sample (10–20) makes each generation cheaper and faster; a larger sample (50–100) reduces word-difficulty variance in the solve-rate estimate. Setting this larger than Games per Generation has no effect — the player plays exactly gamesPerGen games regardless.',
  },
  maxGuesses: {
    title: 'Max Guesses',
    body: 'The maximum number of guesses the player is allowed per game (difficulty). Standard Wordle uses 6. Lower values create a harder problem with more room for prompt-driven improvement:\n\n• 6 (standard): easy — most strategies reach >90% quickly, leaving little gradient for the reflector.\n• 4–5 (hard): moderate challenge; good for verifying that a reflector works at all.\n• 3 (very hard): recommended for research — solve rates start low (~30–50%), improvement is clearly visible, and the reflector must discover real strategy.\n• 2 (extreme): very few words can be solved in 2 guesses; mostly useful for ablation studies.\n\nLower max guesses = stronger training signal for the reflector.',
  },
  includeBaselines: {
    title: 'Include Baseline Players',
    body: 'When enabled, three non-LLM reference agents play the same words in generation 0: a random-word picker, a frequency-based picker (common letters first), and an entropy-maximizing solver (information-theoretic optimal). Their results appear in the Charts tab as reference lines. Baselines are cheap (no LLM calls) and help you calibrate whether the player\'s evolved prompt is approaching theoretically good play. Disable for pure prompt-evolution runs where reference comparison is not needed.',
  },
}

function loadSaved(): Partial<FormState> {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (raw) return JSON.parse(raw) as Partial<FormState>
  } catch { /* ignore */ }
  return {}
}

function saveSettings(f: FormState) {
  try {
    const { seed: _seed, ...rest } = f
    localStorage.setItem(STORAGE_KEY, JSON.stringify(rest))
  } catch { /* ignore */ }
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
  if (!Number.isInteger(gens) || gens < 1 || gens > 50) errs.generations = 'Must be 1–50'
  const gpg = Number(f.gamesPerGen)
  if (!Number.isInteger(gpg) || gpg < 1 || gpg > 500) errs.gamesPerGen = 'Must be 1–500'
  const temp = Number(f.temperature)
  if (isNaN(temp) || temp < 0 || temp > 2) errs.temperature = 'Must be 0.0–2.0'
  const wss = Number(f.wordSampleSize)
  if (!Number.isInteger(wss) || wss < 5 || wss > 100) errs.wordSampleSize = 'Must be 5–100'
  const mg = Number(f.maxGuesses)
  if (!Number.isInteger(mg) || mg < 2 || mg > 6) errs.maxGuesses = 'Must be 2–6'
  return errs
}

function FieldInfoPopup({ fieldKey, onClose }: { fieldKey: string; onClose: () => void }) {
  const info = FIELD_INFO[fieldKey]
  const dialogRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const prev = document.activeElement as HTMLElement
    dialogRef.current?.focus()
    const handler = (e: KeyboardEvent) => { if (e.key === 'Escape') onClose() }
    document.addEventListener('keydown', handler)
    return () => {
      document.removeEventListener('keydown', handler)
      prev?.focus()
    }
  }, [onClose])

  if (!info) return null
  return (
    <div className="modal-overlay" onClick={onClose}>
      <div
        ref={dialogRef}
        className="modal"
        role="dialog"
        aria-modal="true"
        tabIndex={-1}
        onClick={(e) => e.stopPropagation()}
        style={{ maxWidth: 520, maxHeight: '80vh', overflowY: 'auto' }}
      >
        <button className="modal-close" onClick={onClose}>✕</button>
        <h3 style={{ marginBottom: 12 }}>{info.title}</h3>
        <p style={{ fontSize: '0.9rem', lineHeight: 1.6, whiteSpace: 'pre-line', color: 'var(--text)' }}>{info.body}</p>
        <div style={{ textAlign: 'right', marginTop: 16 }}>
          <button className="btn btn-secondary btn-sm" onClick={onClose}>Close</button>
        </div>
      </div>
    </div>
  )
}

function InfoBtn({ fieldKey }: { fieldKey: string }) {
  const [open, setOpen] = useState(false)
  return (
    <>
      <button
        type="button"
        onClick={() => setOpen(true)}
        aria-label={`About ${FIELD_INFO[fieldKey]?.title ?? fieldKey}`}
        style={{
          background: 'none',
          border: '1.5px solid var(--text-secondary)',
          borderRadius: '50%',
          width: 18,
          height: 18,
          fontSize: '0.68rem',
          cursor: 'pointer',
          color: 'var(--text-secondary)',
          display: 'inline-flex',
          alignItems: 'center',
          justifyContent: 'center',
          marginLeft: 6,
          verticalAlign: 'middle',
          lineHeight: 1,
          flexShrink: 0,
        }}
      >
        i
      </button>
      {open && <FieldInfoPopup fieldKey={fieldKey} onClose={() => setOpen(false)} />}
    </>
  )
}

function FieldLabel({ children, fieldKey }: { children: React.ReactNode; fieldKey: string }) {
  return (
    <label className="form-label" style={{ display: 'flex', alignItems: 'center' }}>
      {children}
      <InfoBtn fieldKey={fieldKey} />
    </label>
  )
}

function PromptPreviewModal({ title, text, onClose }: { title: string; text: string; onClose: () => void }) {
  const dialogRef = useRef<HTMLDivElement>(null)
  useEffect(() => {
    const prev = document.activeElement as HTMLElement
    dialogRef.current?.focus()
    const handler = (e: KeyboardEvent) => { if (e.key === 'Escape') onClose() }
    document.addEventListener('keydown', handler)
    return () => { document.removeEventListener('keydown', handler); prev?.focus() }
  }, [onClose])
  return (
    <div className="modal-overlay" onClick={onClose}>
      <div
        ref={dialogRef}
        className="modal"
        role="dialog"
        aria-modal="true"
        tabIndex={-1}
        onClick={(e) => e.stopPropagation()}
        style={{ maxWidth: 660, width: '90vw', maxHeight: '80vh', display: 'flex', flexDirection: 'column' }}
      >
        <button className="modal-close" onClick={onClose}>✕</button>
        <h3 style={{ marginBottom: 12 }}>{title}</h3>
        <pre style={{
          flex: 1,
          overflowY: 'auto',
          fontFamily: 'monospace',
          fontSize: '0.82rem',
          lineHeight: 1.6,
          whiteSpace: 'pre-wrap',
          wordBreak: 'break-word',
          background: 'var(--surface)',
          border: '1px solid var(--border)',
          borderRadius: 6,
          padding: '12px 14px',
          color: 'var(--text)',
          margin: 0,
        }}>
          {text}
        </pre>
        <div style={{ textAlign: 'right', marginTop: 12 }}>
          <button className="btn btn-secondary btn-sm" onClick={onClose}>Close</button>
        </div>
      </div>
    </div>
  )
}

export default function RunForm({ onSubmit, loading, error }: Props) {
  const [models, setModels] = useState<string[]>([])
  const [form, setForm] = useState<FormState>({ ...DEFAULTS, ...loadSaved() })
  const [touched, setTouched] = useState<Partial<Record<keyof FormState, boolean>>>({})
  const [submitted, setSubmitted] = useState(false)
  const [prompts, setPrompts] = useState<DefaultPromptsResponse | null>(null)
  const [promptModal, setPromptModal] = useState<'player' | 'reflector' | null>(null)

  useEffect(() => {
    const saved = loadSaved()
    api.listModels().then((r) => {
      setModels(r.models)
      const pickModel = (saved: string | undefined) =>
        saved && r.models.includes(saved) ? saved : (r.models[0] ?? '')
      setForm((f) => ({
        ...f,
        playerModel: pickModel(saved.playerModel),
        reflectorModel: pickModel(saved.reflectorModel),
      }))
    }).catch(() => {
      const fallback = ['openai/gpt-4o-mini', 'anthropic/claude-3-haiku']
      setModels(fallback)
      setForm((f) => ({
        ...f,
        playerModel: saved.playerModel && fallback.includes(saved.playerModel) ? saved.playerModel : fallback[0],
        reflectorModel: saved.reflectorModel && fallback.includes(saved.reflectorModel) ? saved.reflectorModel : fallback[0],
      }))
    })
  }, [])

  useEffect(() => {
    api.getDefaultPrompts(Number(form.maxGuesses)).then(setPrompts).catch(() => {})
  }, [form.maxGuesses])

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
    saveSettings(form)
    await onSubmit({
      playerModel: form.playerModel,
      reflectorModel: form.reflectorModel,
      temperature: Number(form.temperature),
      seed: Number(form.seed),
      generations: Number(form.generations),
      gamesPerGen: Number(form.gamesPerGen),
      wordSampleSize: Number(form.wordSampleSize),
      maxGuesses: Number(form.maxGuesses),
      includeBaselines: form.includeBaselines,
    })
  }

  return (
    <form onSubmit={handleSubmit}>
      {error && <div className="error-box">{error}</div>}

      <div className="form-row">
        <div className="form-group">
          <FieldLabel fieldKey="playerModel">Player Model</FieldLabel>
          <select className="form-control" value={form.playerModel} onChange={(e) => set('playerModel', e.target.value)}>
            {models.map((m) => <option key={m} value={m}>{m}</option>)}
          </select>
          {showErr('playerModel') && <div className="form-error">{errs.playerModel}</div>}
        </div>
        <div className="form-group">
          <FieldLabel fieldKey="reflectorModel">Reflector Model</FieldLabel>
          <select className="form-control" value={form.reflectorModel} onChange={(e) => set('reflectorModel', e.target.value)}>
            {models.map((m) => <option key={m} value={m}>{m}</option>)}
          </select>
          {showErr('reflectorModel') && <div className="form-error">{errs.reflectorModel}</div>}
        </div>
      </div>

      <div className="form-row">
        <div className="form-group">
          <FieldLabel fieldKey="seed">Seed</FieldLabel>
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
          <FieldLabel fieldKey="temperature">Temperature</FieldLabel>
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
          <FieldLabel fieldKey="generations">Generations</FieldLabel>
          <input
            type="number"
            className="form-control"
            value={form.generations}
            onChange={(e) => set('generations', e.target.value)}
            min={1} max={50}
          />
          <div className="form-hint">1–50 generations</div>
          {showErr('generations') && <div className="form-error">{errs.generations}</div>}
        </div>
        <div className="form-group">
          <FieldLabel fieldKey="gamesPerGen">Games per Generation</FieldLabel>
          <input
            type="number"
            className="form-control"
            value={form.gamesPerGen}
            onChange={(e) => set('gamesPerGen', e.target.value)}
            min={1} max={500}
          />
          <div className="form-hint">1–500 games</div>
          {showErr('gamesPerGen') && <div className="form-error">{errs.gamesPerGen}</div>}
        </div>
      </div>

      <div className="form-row">
        <div className="form-group">
          <FieldLabel fieldKey="wordSampleSize">Word Sample Size</FieldLabel>
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
        <div className="form-group">
          <FieldLabel fieldKey="maxGuesses">Max Guesses</FieldLabel>
          <select
            className="form-control"
            value={form.maxGuesses}
            onChange={(e) => set('maxGuesses', e.target.value)}
          >
            <option value="2">2 — extreme</option>
            <option value="3">3 — very hard (recommended)</option>
            <option value="4">4 — hard</option>
            <option value="5">5 — medium</option>
            <option value="6">6 — standard</option>
          </select>
          <div className="form-hint">Lower = harder, more room to improve</div>
          {showErr('maxGuesses') && <div className="form-error">{errs.maxGuesses}</div>}
        </div>
      </div>

      <div className="form-row">
        <div className="form-group" style={{ paddingTop: 28 }}>
          <label style={{ display: 'flex', alignItems: 'center', gap: 8, cursor: 'pointer' }}>
            <input
              type="checkbox"
              checked={form.includeBaselines}
              onChange={(e) => set('includeBaselines', e.target.checked)}
            />
            <span className="form-label" style={{ margin: 0 }}>Include baseline players in gen 0</span>
            <InfoBtn fieldKey="includeBaselines" />
          </label>
          <div className="form-hint">Adds random / frequency / entropy agents for comparison</div>
        </div>
      </div>

      <div style={{ display: 'flex', alignItems: 'center', gap: 12, marginTop: 8, flexWrap: 'wrap' }}>
        <button type="submit" className="btn btn-primary" disabled={loading} style={{ padding: '10px 24px' }}>
          {loading ? 'Launching…' : 'Launch Run'}
        </button>
        <button
          type="button"
          className="btn btn-secondary btn-sm"
          onClick={() => setPromptModal('player')}
          disabled={!prompts}
        >
          Player Prompt
        </button>
        <button
          type="button"
          className="btn btn-secondary btn-sm"
          onClick={() => setPromptModal('reflector')}
          disabled={!prompts}
        >
          Reflector Prompt
        </button>
      </div>

      {promptModal === 'player' && prompts && (
        <PromptPreviewModal
          title={`Player Prompt (max ${form.maxGuesses} guesses)`}
          text={prompts.playerPrompt}
          onClose={() => setPromptModal(null)}
        />
      )}
      {promptModal === 'reflector' && prompts && (
        <PromptPreviewModal
          title="Reflector System Prompt"
          text={prompts.reflectorPrompt}
          onClose={() => setPromptModal(null)}
        />
      )}
    </form>
  )
}
