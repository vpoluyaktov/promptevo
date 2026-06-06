import { BarChart, Bar, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer, Cell } from 'recharts'
import type { WordDifficultyPoint } from '../../api/types'

interface Props { data: WordDifficultyPoint[] }

export default function WordDifficultyChart({ data }: Props) {
  const top = data.slice(0, 30)

  return (
    <ResponsiveContainer width="100%" height={Math.max(200, top.length * 24)}>
      <BarChart layout="vertical" data={top.map(d => ({ word: d.answer.toUpperCase(), winRate: Math.round(d.winRate * 100), n: d.games, wins: d.wins }))} margin={{ top: 0, right: 20, left: 60, bottom: 0 }}>
        <CartesianGrid strokeDasharray="3 3" stroke="#f0f0f0" />
        <XAxis type="number" domain={[0, 100]} tickFormatter={v => `${v}%`} tick={{ fontSize: 12 }} />
        <YAxis type="category" dataKey="word" tick={{ fontSize: 11, fontFamily: 'monospace' }} />
        <Tooltip
          content={({ active, payload }) => {
            if (!active || !payload?.length) return null
            const d = payload[0].payload
            return (
              <div style={{ background: '#fff', border: '1px solid #ddd', padding: '6px 10px', borderRadius: 4, fontSize: '0.82rem' }}>
                <div><strong>{d.word}</strong></div>
                <div>{d.winRate}% win rate ({d.wins}/{d.n} games)</div>
              </div>
            )
          }}
        />
        <Bar dataKey="winRate" name="Win rate %">
          {top.map((d, i) => (
            <Cell key={i} fill={d.winRate < 0.33 ? '#e74c3c' : d.winRate < 0.67 ? '#f39c12' : '#6aaa64'} />
          ))}
        </Bar>
      </BarChart>
    </ResponsiveContainer>
  )
}
