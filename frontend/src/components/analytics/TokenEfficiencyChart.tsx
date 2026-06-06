import { BarChart, Bar, XAxis, YAxis, CartesianGrid, Tooltip, Legend, ResponsiveContainer } from 'recharts'
import type { TokenEfficiencyPoint } from '../../api/types'

interface Props { data: TokenEfficiencyPoint[] }

export default function TokenEfficiencyChart({ data }: Props) {
  const hasSplit = data.some(d => d.split)

  const chartData = data.map(d => ({
    gen: `Gen ${d.genIndex}`,
    player: hasSplit ? d.playerTokens : undefined,
    reflector: hasSplit ? d.reflectorTokens : undefined,
    total: !hasSplit ? d.tokensUsed : undefined,
  }))

  return (
    <div>
      {!hasSplit && (
        <div style={{ fontSize: '0.78rem', color: 'var(--text-secondary)', marginBottom: 8 }}>Token split unavailable for this run — showing combined total only.</div>
      )}
      <ResponsiveContainer width="100%" height={260}>
        <BarChart data={chartData} margin={{ top: 10, right: 20, left: 0, bottom: 0 }}>
          <CartesianGrid strokeDasharray="3 3" stroke="#f0f0f0" />
          <XAxis dataKey="gen" tick={{ fontSize: 12 }} />
          <YAxis tick={{ fontSize: 12 }} />
          <Tooltip />
          <Legend />
          {hasSplit ? (
            <>
              <Bar dataKey="player" name="Player tokens" fill="#4a90d9" />
              <Bar dataKey="reflector" name="Reflector tokens" fill="#e07040" />
            </>
          ) : (
            <Bar dataKey="total" name="Total tokens" fill="#4a90d9" />
          )}
        </BarChart>
      </ResponsiveContainer>
    </div>
  )
}
