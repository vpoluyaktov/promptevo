import { ComposedChart, Bar, Scatter, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer } from 'recharts'
import type { RemainingCandPoint } from '../../api/types'
import { boxStats } from '../../lib/analytics'

interface Props { data: RemainingCandPoint[] }

export default function RemainingCandidatesChart({ data }: Props) {
  const gens = [...new Set(data.map(d => d.genIndex))].sort((a, b) => a - b)

  const chartData = gens.map(g => {
    const pts = data.filter(d => d.genIndex === g).map(d => d.remainingCandidates)
    const stats = boxStats(pts)
    if (!stats) return null
    return {
      gen: `Gen ${g}`,
      iqr: [stats.q1, stats.q3] as [number, number],
      median: stats.median,
      min: stats.min,
      max: stats.max,
      n: pts.length,
    }
  }).filter(Boolean)

  const scatterData = data.map(d => ({
    gen: `Gen ${d.genIndex}`,
    value: d.remainingCandidates,
  }))

  return (
    <div>
      <div style={{ fontSize: '0.8rem', color: 'var(--text-secondary)', marginBottom: 8 }}>Lost games only · IQR box with min/max whiskers · individual points overlaid</div>
      {chartData.length === 0 ? (
        <div style={{ textAlign: 'center', color: 'var(--text-secondary)', padding: 40 }}>No lost games to display.</div>
      ) : (
        <ResponsiveContainer width="100%" height={260}>
          <ComposedChart data={chartData} margin={{ top: 10, right: 20, left: 0, bottom: 0 }}>
            <CartesianGrid strokeDasharray="3 3" stroke="#f0f0f0" />
            <XAxis dataKey="gen" tick={{ fontSize: 12 }} />
            <YAxis tick={{ fontSize: 12 }} label={{ value: 'Remaining candidates', angle: -90, position: 'insideLeft', offset: 10, style: { fontSize: 11 } }} />
            <Tooltip
              content={({ active, payload }) => {
                if (!active || !payload?.length) return null
                const d = payload[0].payload
                return (
                  <div style={{ background: '#fff', border: '1px solid #ddd', padding: '8px 12px', borderRadius: 4, fontSize: '0.82rem' }}>
                    <div><strong>{d.gen}</strong> (n={d.n})</div>
                    <div>Median: {d.median?.toFixed(1)}</div>
                    <div>IQR: {d.iqr?.[0]?.toFixed(1)} – {d.iqr?.[1]?.toFixed(1)}</div>
                    <div>Range: {d.min} – {d.max}</div>
                  </div>
                )
              }}
            />
            <Bar dataKey="iqr" fill="#4a90d933" stroke="#4a90d9" strokeWidth={1} name="IQR (Q1–Q3)" />
            <Scatter data={scatterData} dataKey="value" fill="#4a90d966" name="Individual loss" r={3} />
          </ComposedChart>
        </ResponsiveContainer>
      )}
    </div>
  )
}
