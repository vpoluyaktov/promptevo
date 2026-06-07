import { LineChart, Line, XAxis, YAxis, CartesianGrid, Tooltip, ReferenceLine, ResponsiveContainer } from 'recharts'
import type { Generation } from '../../api/types'

interface Props {
  generations: Generation[]
}

export default function ViolationRateChart({ generations }: Props) {
  const data = generations
    .filter((g) => g.violationRate != null)
    .map((g) => ({
      gen: `Gen ${g.genIndex}`,
      violationRate: +g.violationRate!.toFixed(2),
    }))

  return (
    <ResponsiveContainer width="100%" height={260}>
      <LineChart data={data} margin={{ top: 10, right: 20, left: 0, bottom: 0 }}>
        <CartesianGrid strokeDasharray="3 3" stroke="#f0f0f0" />
        <XAxis dataKey="gen" tick={{ fontSize: 12 }} />
        <YAxis tickFormatter={(v) => `${v}`} tick={{ fontSize: 12 }} />
        <Tooltip formatter={(v: number) => [`${v}`, 'Violations/game']} />
        <ReferenceLine y={0} stroke="#e0e0e0" />
        <Line type="monotone" dataKey="violationRate" name="Violations/game" stroke="#e06c75" strokeWidth={2} dot={{ r: 4 }} connectNulls />
      </LineChart>
    </ResponsiveContainer>
  )
}
