import { useState } from 'react'
import {
  LineChart,
  Line,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  Legend,
  ReferenceLine,
  ResponsiveContainer,
} from 'recharts'
import type { Generation } from '../api/types'
import InfoPopup from './InfoPopup'

interface Props {
  generations: Generation[]
  showBaselines?: boolean
}

export default function MetricsChart({ generations, showBaselines }: Props) {
  const [violationInfoOpen, setViolationInfoOpen] = useState(false)
  const data = generations.map((g) => ({
    gen: `Gen ${g.genIndex}`,
    solveRate: g.solveRate != null ? +(g.solveRate * 100).toFixed(1) : null,
    meanGuesses: g.meanGuesses != null ? +g.meanGuesses.toFixed(2) : null,
    meanInfoGain: g.meanInfoGain != null ? +g.meanInfoGain.toFixed(2) : null,
    violationRate: g.violationRate != null ? +g.violationRate.toFixed(2) : null,
    promptLen: g.promptLen,
  }))

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 32 }}>
      {/* Solve rate */}
      <div>
        <h4 style={{ marginBottom: 12, fontSize: '0.9rem', color: 'var(--text-secondary)', textTransform: 'uppercase', letterSpacing: '0.05em' }}>
          Solve Rate (%)
        </h4>
        <ResponsiveContainer width="100%" height={220}>
          <LineChart data={data}>
            <CartesianGrid strokeDasharray="3 3" stroke="#e0e0e0" />
            <XAxis dataKey="gen" tick={{ fontSize: 12 }} />
            <YAxis domain={[0, 100]} tick={{ fontSize: 12 }} />
            <Tooltip />
            <Legend />
            {showBaselines && (
              <ReferenceLine y={17} stroke="#c9b458" strokeDasharray="4 4" label={{ value: 'Random ~17%', position: 'insideTopRight', fontSize: 11, fill: '#c9b458' }} />
            )}
            <Line type="monotone" dataKey="solveRate" name="Solve Rate %" stroke="#6aaa64" strokeWidth={2} dot={{ r: 4 }} connectNulls />
          </LineChart>
        </ResponsiveContainer>
      </div>

      {/* Mean guesses */}
      <div>
        <h4 style={{ marginBottom: 12, fontSize: '0.9rem', color: 'var(--text-secondary)', textTransform: 'uppercase', letterSpacing: '0.05em' }}>
          Mean Guesses
        </h4>
        <ResponsiveContainer width="100%" height={220}>
          <LineChart data={data}>
            <CartesianGrid strokeDasharray="3 3" stroke="#e0e0e0" />
            <XAxis dataKey="gen" tick={{ fontSize: 12 }} />
            <YAxis domain={[1, 7]} tick={{ fontSize: 12 }} />
            <Tooltip />
            <Line type="monotone" dataKey="meanGuesses" name="Mean Guesses" stroke="#c9b458" strokeWidth={2} dot={{ r: 4 }} connectNulls />
          </LineChart>
        </ResponsiveContainer>
      </div>

      {/* Mean info gain */}
      <div>
        <h4 style={{ marginBottom: 12, fontSize: '0.9rem', color: 'var(--text-secondary)', textTransform: 'uppercase', letterSpacing: '0.05em' }}>
          Mean Info Gain (bits)
        </h4>
        <ResponsiveContainer width="100%" height={220}>
          <LineChart data={data}>
            <CartesianGrid strokeDasharray="3 3" stroke="#e0e0e0" />
            <XAxis dataKey="gen" tick={{ fontSize: 12 }} />
            <YAxis tick={{ fontSize: 12 }} />
            <Tooltip />
            <Line type="monotone" dataKey="meanInfoGain" name="Mean Info Gain" stroke="#787c7e" strokeWidth={2} dot={{ r: 4 }} connectNulls />
          </LineChart>
        </ResponsiveContainer>
      </div>

      {/* Constraint violation rate */}
      <div>
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 12 }}>
          <h4 style={{ fontSize: '0.9rem', color: 'var(--text-secondary)', textTransform: 'uppercase', letterSpacing: '0.05em', margin: 0 }}>
            Constraint Violations (per game)
          </h4>
          <button
            onClick={() => setViolationInfoOpen(true)}
            aria-label="About constraint violations"
            style={{ background: 'none', border: '1.5px solid var(--text-secondary)', borderRadius: '50%', width: 22, height: 22, fontSize: '0.75rem', cursor: 'pointer', color: 'var(--text-secondary)', display: 'flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0 }}
          >
            ⓘ
          </button>
        </div>
        <InfoPopup metricKey="violationRate" open={violationInfoOpen} onClose={() => setViolationInfoOpen(false)} />
        <ResponsiveContainer width="100%" height={220}>
          <LineChart data={data}>
            <CartesianGrid strokeDasharray="3 3" stroke="#e0e0e0" />
            <XAxis dataKey="gen" tick={{ fontSize: 12 }} />
            <YAxis tickFormatter={v => `${v}`} tick={{ fontSize: 12 }} />
            <Tooltip formatter={(v: number) => [`${v}`, 'Violations/game']} />
            <ReferenceLine y={0} stroke="#e0e0e0" />
            <Line type="monotone" dataKey="violationRate" name="Violations/game" stroke="#e06c75" strokeWidth={2} dot={{ r: 4 }} connectNulls />
          </LineChart>
        </ResponsiveContainer>
      </div>

      {/* Prompt length sparkline */}
      <div>
        <h4 style={{ marginBottom: 12, fontSize: '0.9rem', color: 'var(--text-secondary)', textTransform: 'uppercase', letterSpacing: '0.05em' }}>
          Prompt Length (chars)
        </h4>
        <ResponsiveContainer width="100%" height={160}>
          <LineChart data={data}>
            <CartesianGrid strokeDasharray="3 3" stroke="#e0e0e0" />
            <XAxis dataKey="gen" tick={{ fontSize: 12 }} />
            <YAxis tick={{ fontSize: 12 }} />
            <Tooltip />
            <Line type="monotone" dataKey="promptLen" name="Prompt Chars" stroke="#1a1a1b" strokeWidth={2} dot={{ r: 3 }} />
          </LineChart>
        </ResponsiveContainer>
      </div>
    </div>
  )
}
