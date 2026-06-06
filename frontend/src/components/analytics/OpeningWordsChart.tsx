import { useState } from 'react'
import { BarChart, Bar, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer, Cell } from 'recharts'
import type { OpeningWordsPoint } from '../../api/types'

interface Props { data: OpeningWordsPoint[] }

export default function OpeningWordsChart({ data }: Props) {
  const [selectedGen, setSelectedGen] = useState(data.length > 0 ? data[data.length - 1].genIndex : 0)
  const genData = data.find(d => d.genIndex === selectedGen)
  const words = (genData?.words ?? []).slice(0, 12)

  return (
    <div>
      <div style={{ marginBottom: 12, display: 'flex', alignItems: 'center', gap: 8 }}>
        <label style={{ fontSize: '0.85rem', color: 'var(--text-secondary)' }}>Generation:</label>
        <select
          value={selectedGen}
          onChange={e => setSelectedGen(Number(e.target.value))}
          style={{ fontSize: '0.85rem', padding: '2px 8px' }}
        >
          {data.map(d => (
            <option key={d.genIndex} value={d.genIndex}>Gen {d.genIndex}</option>
          ))}
        </select>
      </div>
      <ResponsiveContainer width="100%" height={Math.max(200, words.length * 28)}>
        <BarChart layout="vertical" data={words.map(w => ({ word: w.word.toUpperCase(), count: w.count }))} margin={{ top: 0, right: 20, left: 50, bottom: 0 }}>
          <CartesianGrid strokeDasharray="3 3" stroke="#f0f0f0" />
          <XAxis type="number" tick={{ fontSize: 12 }} />
          <YAxis type="category" dataKey="word" tick={{ fontSize: 12, fontFamily: 'monospace' }} />
          <Tooltip />
          <Bar dataKey="count" name="Games">
            {words.map((_, i) => (
              <Cell key={i} fill={i === 0 ? '#6aaa64' : '#85c17e'} />
            ))}
          </Bar>
        </BarChart>
      </ResponsiveContainer>
      {(genData?.words.length ?? 0) > 12 && (
        <div style={{ fontSize: '0.78rem', color: 'var(--text-secondary)', textAlign: 'center', marginTop: 4 }}>
          +{(genData!.words.length) - 12} more words not shown
        </div>
      )}
    </div>
  )
}
