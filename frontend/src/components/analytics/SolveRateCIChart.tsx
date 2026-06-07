import { ComposedChart, Area, Line, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer, ReferenceLine } from 'recharts'
import type { SolveRateCIPoint, BaselineStat } from '../../api/types'

interface Props {
  data: SolveRateCIPoint[]
  baselines?: BaselineStat[]
}

const BASELINE_COLORS: Record<string, string> = {
  random:    '#aaa',
  frequency: '#f5a623',
  entropy:   '#9b59b6',
}

export default function SolveRateCIChart({ data, baselines }: Props) {
  const chartData = data.map(d => ({
    gen: `Gen ${d.genIndex}`,
    solveRate: Math.round(d.solveRate * 100),
    ciRange: [Math.round(d.ciLower * 100), Math.round(d.ciUpper * 100)] as [number, number],
    n: d.n,
    wins: d.wins,
  }))

  return (
    <ResponsiveContainer width="100%" height={260}>
      <ComposedChart data={chartData} margin={{ top: 10, right: 130, left: 0, bottom: 0 }}>
        <CartesianGrid strokeDasharray="3 3" stroke="#f0f0f0" />
        <XAxis dataKey="gen" tick={{ fontSize: 12 }} />
        <YAxis domain={[0, 100]} tickFormatter={v => `${v}%`} tick={{ fontSize: 12 }} />
        <Tooltip formatter={(v, name) => name === 'CI Band' ? `${(v as number[])[0]}%–${(v as number[])[1]}%` : `${v}%`} />
        {(baselines ?? []).map(b => (
          <ReferenceLine
            key={b.agentType}
            y={Math.round(b.solveRate * 100)}
            stroke={BASELINE_COLORS[b.agentType] ?? '#ccc'}
            strokeDasharray="4 4"
            label={{
              value: `${b.agentType} ${Math.round(b.solveRate * 100)}%`,
              position: 'right',
              fontSize: 11,
              fill: BASELINE_COLORS[b.agentType] ?? '#ccc',
            }}
          />
        ))}
        <Area dataKey="ciRange" fill="#6aaa6433" stroke="none" name="CI Band" />
        <Line dataKey="solveRate" stroke="#6aaa64" strokeWidth={2} dot={{ r: 4 }} name="Solve Rate" />
      </ComposedChart>
    </ResponsiveContainer>
  )
}
