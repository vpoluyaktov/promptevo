import { ScatterChart, Scatter, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer } from 'recharts'
import type { RemainingCandPoint } from '../../api/types'
import { boxStats } from '../../lib/analytics'

interface Props { data: RemainingCandPoint[] }

const GEN_COLORS = ['#4a90d9', '#6aaa64', '#e97c2a', '#9b59b6', '#e84393', '#2ecc71', '#e74c3c', '#f39c12']

export default function RemainingCandidatesChart({ data }: Props) {
  if (data.length === 0) {
    return <div style={{ textAlign: 'center', color: 'var(--text-secondary)', padding: 40 }}>No lost games to display.</div>
  }

  const gens = [...new Set(data.map(d => d.genIndex))].sort((a, b) => a - b)

  const scatterByGen = gens.map((g, i) => {
    const pts = data.filter(d => d.genIndex === g)
    return {
      genIndex: g,
      color: GEN_COLORS[i % GEN_COLORS.length],
      points: pts.map(d => ({ x: g, y: d.remainingCandidates, answer: d.answer, numGuesses: d.numGuesses })),
      stats: boxStats(pts.map(d => d.remainingCandidates)),
    }
  })

  const xDomain: [number, number] = [gens[0] - 0.5, gens[gens.length - 1] + 0.5]

  return (
    <div>
      <div style={{ fontSize: '0.8rem', color: 'var(--text-secondary)', marginBottom: 8 }}>
        Lost games only · each dot is one game · hover for gen stats
      </div>
      <ResponsiveContainer width="100%" height={260}>
        <ScatterChart margin={{ top: 10, right: 20, left: 0, bottom: 0 }}>
          <CartesianGrid strokeDasharray="3 3" stroke="#f0f0f0" />
          <XAxis
            type="number"
            dataKey="x"
            domain={xDomain}
            ticks={gens}
            tickFormatter={(v) => `Gen ${v}`}
            tick={{ fontSize: 12 }}
          />
          <YAxis
            type="number"
            dataKey="y"
            tick={{ fontSize: 12 }}
            label={{ value: 'Remaining candidates', angle: -90, position: 'insideLeft', offset: 10, style: { fontSize: 11 } }}
          />
          <Tooltip
            content={({ active, payload }) => {
              if (!active || !payload?.length) return null
              const d = payload[0].payload as { x: number; y: number; answer: string; numGuesses: number }
              const genEntry = scatterByGen.find(g => g.genIndex === d.x)
              const s = genEntry?.stats
              return (
                <div style={{ background: '#fff', border: '1px solid #ddd', padding: '8px 12px', borderRadius: 4, fontSize: '0.82rem' }}>
                  <div style={{ marginBottom: 4 }}><strong>Gen {d.x}</strong></div>
                  <div>Answer: <strong style={{ textTransform: 'uppercase' }}>{d.answer}</strong></div>
                  <div>Remaining candidates: <strong>{d.y}</strong></div>
                  <div>Guesses used: {d.numGuesses}</div>
                  {s && (
                    <div style={{ marginTop: 6, paddingTop: 6, borderTop: '1px solid #eee', color: 'var(--text-secondary)' }}>
                      <div>Median: {s.median.toFixed(1)} · IQR: {s.q1.toFixed(1)}–{s.q3.toFixed(1)}</div>
                      <div>Range: {s.min}–{s.max} · n={genEntry!.points.length}</div>
                    </div>
                  )}
                </div>
              )
            }}
          />
          {scatterByGen.map(g => (
            <Scatter
              key={g.genIndex}
              name={`Gen ${g.genIndex}`}
              data={g.points}
              fill={g.color}
              fillOpacity={0.75}
              r={4}
            />
          ))}
        </ScatterChart>
      </ResponsiveContainer>
    </div>
  )
}
