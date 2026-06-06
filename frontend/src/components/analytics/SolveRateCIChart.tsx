import { ComposedChart, Area, Line, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer, ReferenceLine } from 'recharts'
import type { SolveRateCIPoint } from '../../api/types'

interface Props { data: SolveRateCIPoint[] }

export default function SolveRateCIChart({ data }: Props) {
  const chartData = data.map(d => ({
    gen: `Gen ${d.genIndex}`,
    solveRate: Math.round(d.solveRate * 100),
    ciRange: [Math.round(d.ciLower * 100), Math.round(d.ciUpper * 100)] as [number, number],
    n: d.n,
    wins: d.wins,
  }))

  return (
    <ResponsiveContainer width="100%" height={260}>
      <ComposedChart data={chartData} margin={{ top: 10, right: 20, left: 0, bottom: 0 }}>
        <CartesianGrid strokeDasharray="3 3" stroke="#f0f0f0" />
        <XAxis dataKey="gen" tick={{ fontSize: 12 }} />
        <YAxis domain={[0, 100]} tickFormatter={v => `${v}%`} tick={{ fontSize: 12 }} />
        <Tooltip formatter={(v, name) => name === 'CI Band' ? `${(v as number[])[0]}%–${(v as number[])[1]}%` : `${v}%`} />
        <ReferenceLine y={17} stroke="#aaa" strokeDasharray="4 4" label={{ value: 'random baseline', position: 'right', fontSize: 11, fill: '#aaa' }} />
        <Area dataKey="ciRange" fill="#6aaa6433" stroke="none" name="CI Band" />
        <Line dataKey="solveRate" stroke="#6aaa64" strokeWidth={2} dot={{ r: 4 }} name="Solve Rate" />
      </ComposedChart>
    </ResponsiveContainer>
  )
}
