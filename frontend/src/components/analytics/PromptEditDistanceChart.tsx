import { LineChart, Line, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer } from 'recharts'
import type { Generation } from '../../api/types'
import { normalizedLevenshtein } from '../../lib/analytics'

interface Props { generations: Generation[] }

export default function PromptEditDistanceChart({ generations }: Props) {
  const sorted = [...generations].sort((a, b) => a.genIndex - b.genIndex)
  const data = sorted.slice(1).map((g, i) => ({
    gen: `Gen ${g.genIndex}`,
    distance: Math.round(normalizedLevenshtein(sorted[i].promptText, g.promptText) * 1000) / 10,
  }))

  if (data.length === 0) {
    return <div style={{ textAlign: 'center', color: 'var(--text-secondary)', padding: 40 }}>Need at least 2 generations.</div>
  }

  return (
    <ResponsiveContainer width="100%" height={260}>
      <LineChart data={data} margin={{ top: 10, right: 20, left: 0, bottom: 0 }}>
        <CartesianGrid strokeDasharray="3 3" stroke="#f0f0f0" />
        <XAxis dataKey="gen" tick={{ fontSize: 12 }} />
        <YAxis domain={[0, 100]} tickFormatter={v => `${v}%`} tick={{ fontSize: 12 }} />
        <Tooltip formatter={v => `${v}%`} />
        <Line dataKey="distance" stroke="#e07040" strokeWidth={2} dot={{ r: 4 }} name="Edit Distance" />
      </LineChart>
    </ResponsiveContainer>
  )
}
