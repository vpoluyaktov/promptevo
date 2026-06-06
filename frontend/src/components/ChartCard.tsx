import { useState } from 'react'
import InfoPopup, { type MetricKey } from './InfoPopup'

interface ChartCardProps {
  title: string
  metricKey: MetricKey
  children: React.ReactNode
  subtitle?: string
}

export default function ChartCard({ title, metricKey, children, subtitle }: ChartCardProps) {
  const [open, setOpen] = useState(false)
  return (
    <div className="card" style={{ marginBottom: 24 }}>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 8 }}>
        <div>
          <h3 style={{ fontWeight: 600, fontSize: '1rem', margin: 0 }}>{title}</h3>
          {subtitle && <div style={{ fontSize: '0.8rem', color: 'var(--text-secondary)', marginTop: 2 }}>{subtitle}</div>}
        </div>
        <button
          onClick={() => setOpen(true)}
          aria-label="About this metric"
          style={{ background: 'none', border: '1.5px solid var(--text-secondary)', borderRadius: '50%', width: 22, height: 22, fontSize: '0.75rem', cursor: 'pointer', color: 'var(--text-secondary)', display: 'flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0 }}
        >
          ⓘ
        </button>
      </div>
      {children}
      <InfoPopup metricKey={metricKey} open={open} onClose={() => setOpen(false)} />
    </div>
  )
}
