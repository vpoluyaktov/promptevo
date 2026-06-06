import { ScatterChart, Scatter, XAxis, YAxis, CartesianGrid, Tooltip, Legend, ResponsiveContainer } from 'recharts'
import type { ReasoningPoint } from '../../api/types'

interface Props { data: ReasoningPoint[] }

export default function ReasoningScatterChart({ data }: Props) {
  const wonData = data.filter(d => d.won).map(d => ({ x: d.reasoningChars, y: d.genIndex, gameId: d.gameId }))
  const lostData = data.filter(d => !d.won).map(d => ({ x: d.reasoningChars, y: d.genIndex, gameId: d.gameId }))

  return (
    <ResponsiveContainer width="100%" height={280}>
      <ScatterChart margin={{ top: 10, right: 20, left: 0, bottom: 10 }}>
        <CartesianGrid strokeDasharray="3 3" stroke="#f0f0f0" />
        <XAxis dataKey="x" name="Reasoning chars" type="number" tick={{ fontSize: 12 }} label={{ value: 'Reasoning characters', position: 'insideBottom', offset: -4, style: { fontSize: 11 } }} />
        <YAxis dataKey="y" name="Generation" type="number" allowDecimals={false} tick={{ fontSize: 12 }} label={{ value: 'Generation', angle: -90, position: 'insideLeft', style: { fontSize: 11 } }} />
        <Tooltip cursor={{ strokeDasharray: '3 3' }} content={({ active, payload }) => {
          if (!active || !payload?.length) return null
          const d = payload[0].payload
          return (
            <div style={{ background: '#fff', border: '1px solid #ddd', padding: '6px 10px', borderRadius: 4, fontSize: '0.82rem' }}>
              <div>Game #{d.gameId}</div>
              <div>{d.x} reasoning chars</div>
              <div>Gen {d.y}</div>
            </div>
          )
        }} />
        <Legend />
        <Scatter name="Won" data={wonData} fill="#6aaa64" opacity={0.7} />
        <Scatter name="Lost" data={lostData} fill="#c9b458" opacity={0.7} />
      </ScatterChart>
    </ResponsiveContainer>
  )
}
