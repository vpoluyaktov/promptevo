export function levenshtein(a: string, b: string): number {
  const ar = [...a], br = [...b]
  const m = ar.length, n = br.length
  const dp: number[][] = Array.from({ length: m + 1 }, (_, i) =>
    Array.from({ length: n + 1 }, (_, j) => (i === 0 ? j : j === 0 ? i : 0))
  )
  for (let i = 1; i <= m; i++) {
    for (let j = 1; j <= n; j++) {
      dp[i][j] = ar[i - 1] === br[j - 1]
        ? dp[i - 1][j - 1]
        : 1 + Math.min(dp[i - 1][j], dp[i][j - 1], dp[i - 1][j - 1])
    }
  }
  return dp[m][n]
}

export function normalizedLevenshtein(a: string, b: string): number {
  const maxLen = Math.max([...a].length, [...b].length)
  if (maxLen === 0) return 0
  return levenshtein(a, b) / maxLen
}

export interface BoxStats {
  min: number
  q1: number
  median: number
  q3: number
  max: number
}

export function boxStats(values: number[]): BoxStats | null {
  if (values.length === 0) return null
  const sorted = [...values].sort((a, b) => a - b)
  const n = sorted.length

  function quantile(p: number): number {
    const pos = p * (n - 1)
    const lo = Math.floor(pos)
    const hi = Math.ceil(pos)
    if (lo === hi) return sorted[lo]
    return sorted[lo] + (sorted[hi] - sorted[lo]) * (pos - lo)
  }

  return {
    min: sorted[0],
    q1: quantile(0.25),
    median: quantile(0.5),
    q3: quantile(0.75),
    max: sorted[n - 1],
  }
}
