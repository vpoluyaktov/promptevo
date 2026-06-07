import { useState, useEffect } from 'react'
import { api } from '../../api/client'
import type { AnalyticsResponse } from '../../api/types'
import type { Generation } from '../../api/types'
import ChartCard from '../ChartCard'
import SolveRateCIChart from './SolveRateCIChart'
import WinDistributionChart from './WinDistributionChart'
import TurnInfoGainChart from './TurnInfoGainChart'
import OpeningWordsChart from './OpeningWordsChart'
import RemainingCandidatesChart from './RemainingCandidatesChart'
import PromptEditDistanceChart from './PromptEditDistanceChart'
import TokenEfficiencyChart from './TokenEfficiencyChart'
import ReasoningScatterChart from './ReasoningScatterChart'
import WordDifficultyChart from './WordDifficultyChart'
import ViolationRateChart from './ViolationRateChart'

interface Props {
  runId: number
  generations: Generation[]
  maxGuesses: number
}

export default function AnalysisTab({ runId, generations, maxGuesses }: Props) {
  const [data, setData] = useState<AnalyticsResponse | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    setLoading(true)
    setError('')
    api.getAnalytics(runId)
      .then(setData)
      .catch(() => setError('Failed to load analytics.'))
      .finally(() => setLoading(false))
  }, [runId])

  if (loading) return <div style={{ textAlign: 'center', padding: 40 }}><div className="spinner" /></div>
  if (error) return <div className="error-banner">{error}</div>
  if (!data) return null
  if (data.meta.totalLlmGames === 0) {
    return <div className="empty-state"><p>No LLM games to analyze yet.</p></div>
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 0 }}>
      <ChartCard title="Solve Rate with 95% CI" metricKey="solveRateCI" subtitle={`${data.meta.totalLlmGames} LLM games · ${data.meta.generations} generations`}>
        <SolveRateCIChart data={data.solveRateCI} baselines={data.baselineStats} />
      </ChartCard>
      <ChartCard title="Remaining Candidates at Game-Over" metricKey="remainingCandidates" subtitle="Losses only">
        <RemainingCandidatesChart data={data.remainingCandidates} />
      </ChartCard>
      <ChartCard title="Turn-Level Information Gain" metricKey="turnInfoGain">
        <TurnInfoGainChart data={data.turnInfoGain} />
      </ChartCard>
      <ChartCard title="Constraint Violations per Game" metricKey="violationRate">
        <ViolationRateChart generations={generations} />
      </ChartCard>
      <ChartCard title="Prompt Edit Distance" metricKey="promptEditDistance">
        <PromptEditDistanceChart generations={generations} />
      </ChartCard>
      <ChartCard title="Reasoning Verbosity vs Outcome" metricKey="reasoningVerbosity">
        <ReasoningScatterChart data={data.reasoningVerbosity} />
      </ChartCard>
      <ChartCard title="Token Efficiency" metricKey="tokenEfficiency">
        <TokenEfficiencyChart data={data.tokenEfficiency} />
      </ChartCard>
      <ChartCard title="Win Distribution by Turn" metricKey="winDistribution">
        <WinDistributionChart data={data.winDistribution} maxGuesses={maxGuesses} />
      </ChartCard>
      <ChartCard title="Opening Word Frequency" metricKey="openingWords">
        <OpeningWordsChart data={data.openingWords} />
      </ChartCard>
      <ChartCard title="Per-Word Difficulty" metricKey="wordDifficulty" subtitle="Hardest first · top 30">
        <WordDifficultyChart data={data.wordDifficulty} />
      </ChartCard>
    </div>
  )
}
