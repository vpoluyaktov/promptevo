import { BarChart, Bar, XAxis, YAxis, CartesianGrid, Tooltip, Legend, ResponsiveContainer } from 'recharts'
import type { WinDistPoint } from '../../api/types'

interface Props { data: WinDistPoint[]; maxGuesses: number }

const TURN_COLORS = ['#6aaa64', '#85c17e', '#b5d99c', '#d4edaf', '#f0c040', '#e09030']
const LOST_COLOR = '#c9b458aa'

export default function WinDistributionChart({ data, maxGuesses }: Props) {
  const chartData = data.map(d => {
    const row: Record<string, unknown> = { gen: `Gen ${d.genIndex}`, lost: d.lost }
    for (let t = 1; t <= maxGuesses; t++) {
      row[`turn${t}`] = d.wonByTurn[String(t)] ?? 0
    }
    return row
  })

  return (
    <ResponsiveContainer width="100%" height={260}>
      <BarChart data={chartData} margin={{ top: 10, right: 20, left: 0, bottom: 0 }}>
        <CartesianGrid strokeDasharray="3 3" stroke="#f0f0f0" />
        <XAxis dataKey="gen" tick={{ fontSize: 12 }} />
        <YAxis tick={{ fontSize: 12 }} />
        <Tooltip />
        <Legend />
        {Array.from({ length: maxGuesses }, (_, i) => i + 1).map((t, i) => (
          <Bar key={t} dataKey={`turn${t}`} name={`Turn ${t}`} stackId="a" fill={TURN_COLORS[i % TURN_COLORS.length]} />
        ))}
        <Bar dataKey="lost" name="Lost" stackId="a" fill={LOST_COLOR} />
      </BarChart>
    </ResponsiveContainer>
  )
}
