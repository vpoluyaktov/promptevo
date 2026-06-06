import { BarChart, Bar, XAxis, YAxis, CartesianGrid, Tooltip, Legend, ResponsiveContainer } from 'recharts'
import type { TurnInfoGainPoint } from '../../api/types'

interface Props { data: TurnInfoGainPoint[] }

const GEN_COLORS = ['#6aaa64', '#4a90d9', '#e07040', '#9b59b6', '#16a085', '#e74c3c', '#f39c12', '#2980b9']

export default function TurnInfoGainChart({ data }: Props) {
  const gens = [...new Set(data.map(d => d.genIndex))].sort((a, b) => a - b)
  const turns = [...new Set(data.map(d => d.turnIndex))].sort((a, b) => a - b)

  const chartData = turns.map(t => {
    const row: Record<string, unknown> = { turn: `Turn ${t + 1}` }
    for (const g of gens) {
      const pt = data.find(d => d.genIndex === g && d.turnIndex === t)
      row[`gen${g}`] = pt ? pt.meanInfoGain : null
    }
    return row
  })

  return (
    <ResponsiveContainer width="100%" height={260}>
      <BarChart data={chartData} margin={{ top: 10, right: 20, left: 0, bottom: 0 }}>
        <CartesianGrid strokeDasharray="3 3" stroke="#f0f0f0" />
        <XAxis dataKey="turn" tick={{ fontSize: 12 }} />
        <YAxis tickFormatter={v => `${v} bits`} tick={{ fontSize: 12 }} />
        <Tooltip formatter={(v: unknown) => v !== null ? `${v} bits` : 'n/a'} />
        <Legend />
        {gens.slice(0, 8).map((g, i) => (
          <Bar key={g} dataKey={`gen${g}`} name={`Gen ${g}`} fill={GEN_COLORS[i % GEN_COLORS.length]} />
        ))}
      </BarChart>
    </ResponsiveContainer>
  )
}
