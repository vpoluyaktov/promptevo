interface Props {
  value?: string
}

export default function ConvergenceBadge({ value }: Props) {
  if (!value) return null
  const cls =
    value === 'stable'
      ? 'badge badge-stable'
      : value === 'oscillating'
      ? 'badge badge-oscillating'
      : 'badge badge-improving'
  return <span className={cls}>{value}</span>
}
