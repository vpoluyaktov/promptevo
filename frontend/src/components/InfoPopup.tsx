import { useEffect, useRef } from 'react'

export type MetricKey =
  | 'solveRateCI'
  | 'winDistribution'
  | 'turnInfoGain'
  | 'openingWords'
  | 'remainingCandidates'
  | 'promptEditDistance'
  | 'tokenEfficiency'
  | 'reasoningVerbosity'
  | 'wordDifficulty'
  | 'violationRate'

export interface InfoContent {
  title: string
  whatItMeasures: string
  whyItMatters: string
  goodBad: string
}

export const INFO_CONTENT: Record<MetricKey, InfoContent> = {
  solveRateCI: {
    title: 'Solve Rate with 95% Confidence Interval',
    whatItMeasures: 'The fraction of games the agent solved in each generation, shown with a 95% Wilson score confidence interval. The shaded band is the range of true solve rates consistent with the observed wins, given the sample size.',
    whyItMatters: 'Prompt evolution is only meaningful if a generation\'s improvement exceeds sampling noise. With a small number of games per generation, a solve-rate jump can be pure chance. The confidence band tells you whether two generations are statistically distinguishable: if their intervals overlap heavily, the apparent gain may not be real. The Wilson interval is used rather than the normal approximation because it remains valid at the boundaries (0% and 100%) and for small samples.',
    goodBad: 'Higher is better; a tight band is better than a wide one. Bands shrink as games-per-generation grows (roughly with 1/√n). If you see an upward solve-rate trend whose later intervals sit entirely above the earlier ones, that is strong evidence the reflector is genuinely improving strategy. If the bands overlap across all generations, treat the run as inconclusive and increase the sample size.',
  },
  winDistribution: {
    title: 'Win Distribution by Turn',
    whatItMeasures: 'For each generation, how the solved games are distributed across the turn on which they were won (turn 1, 2, …), plus the count of lost games. Each bar is one generation; segments stack to the total games played.',
    whyItMatters: 'Solve rate alone hides how the agent wins. Two prompts with the same solve rate can differ sharply in efficiency: one may grind out wins on the final allowed turn while another wins early with confident information-dense guesses. Watching mass shift toward earlier turns across generations is direct evidence that the reflector is teaching the agent to gather information faster, not just to avoid losing.',
    goodBad: 'Healthy evolution shifts the colored mass leftward (earlier-turn wins) and shrinks the \'lost\' segment over generations. A distribution piled up on the last allowed turn indicates a fragile strategy that barely succeeds and will collapse under a tighter guess budget. A growing \'lost\' segment is a regression signal.',
  },
  turnInfoGain: {
    title: 'Turn-Level Information Gain',
    whatItMeasures: 'The mean information gain, in bits, contributed by each guess position (turn 1, turn 2, …) within a game, broken down by generation. One bit means the guess halved the set of remaining candidate answers.',
    whyItMatters: 'Wordle is, formally, an active-learning problem: each guess is an experiment that should maximally reduce answer uncertainty. This chart shows whether the evolved prompt front-loads information — strong openers that eliminate large candidate sets — and how gain decays over the course of a game. It separates opening strategy from endgame strategy, which solve rate cannot.',
    goodBad: 'Early turns should show the highest bits (a good opener on the full answer list yields roughly 4–6 bits). Bits naturally decay on later turns as fewer candidates remain — that decay is expected, not a problem. A weak or flat turn-1 bar across generations means the reflector has not discovered high-entropy openings. Note the per-point sample size (n) in the tooltip: late turns are estimated from fewer games and are noisier.',
  },
  openingWords: {
    title: 'Opening Word Frequency',
    whatItMeasures: 'The distribution of first guesses the agent chooses in each generation, and how often each is used.',
    whyItMatters: 'The opening word is the single highest-leverage decision in Wordle and a clean fingerprint of strategy. Convergence onto a small set of high-entropy openers (e.g. words rich in common letters) across generations is a visible sign that prompt evolution is discovering and committing to a strong fixed policy. Persistent scatter across many openers indicates the prompt has not constrained the opening, leaving it to model temperature and chance.',
    goodBad: 'Increasing concentration on one or a few strong openers over generations is the desirable trend — it shows the reflector is encoding a reusable heuristic. High diversity that never narrows suggests an under-specified prompt or excessive temperature. Beware concentration on a weak opener: cross-reference with Turn-Level Information Gain to confirm the favored opener actually yields high bits.',
  },
  remainingCandidates: {
    title: 'Remaining Candidates at Game-Over (losses)',
    whatItMeasures: 'For every lost game, the number of answer candidates still consistent with all feedback when the guess budget ran out, computed by replaying the agent\'s guesses against the answer word list. Shown as a box plot per generation (median, interquartile range, and range), losses only.',
    whyItMatters: 'This diagnoses why the agent loses. A small number of remaining candidates at game-over means the agent had nearly solved the puzzle and lost on the last step — a near-miss, often a guess-budget or tie-breaking problem. A large number means the agent failed to constrain the search at all — a fundamental strategy failure. The two cases call for completely different prompt fixes, and solve rate alone cannot tell them apart.',
    goodBad: 'Lower is better: boxes near 1–3 remaining candidates indicate \'unlucky near-misses\' that a small guess-budget or end-game tweak could convert to wins. Boxes in the tens or hundreds indicate the agent is not exploiting feedback — the prompt needs stronger constraint-tracking and elimination guidance. A downward shift of the boxes across generations means the reflector is teaching the agent to box-in the answer even on its failures.',
  },
  promptEditDistance: {
    title: 'Prompt Edit Distance',
    whatItMeasures: 'The normalized Levenshtein (character edit) distance between each generation\'s strategy prompt and the previous generation\'s, expressed as a fraction of the longer prompt (0% = identical, 100% = completely rewritten).',
    whyItMatters: 'This quantifies the magnitude of mutation the reflector applies at each step — the size of the evolutionary jump. The reflector is instructed to make surgical, targeted edits; this metric verifies it. Large oscillating edits suggest the reflector is thrashing (rewriting wholesale rather than refining), while distances trending toward zero suggest the search has converged on a stable prompt.',
    goodBad: 'Small, decreasing edits (a few percent, shrinking over generations) indicate healthy convergence toward a stable strategy. Persistently large edits (tens of percent) indicate instability or prompt thrashing — often correlated with oscillating solve rate. A flat 0% means the reflector stopped changing the prompt (it reused the prior prompt, e.g. unparseable reflector output or the final, non-reflecting generation). Read this chart alongside Solve Rate: useful evolution shows shrinking edits and rising solve rate.',
  },
  tokenEfficiency: {
    title: 'Token Efficiency (player vs reflector)',
    whatItMeasures: 'Per generation, the LLM tokens consumed by the player (all game-play calls) versus the reflector (the single self-reflection call that rewrites the prompt).',
    whyItMatters: 'Self-improvement has a compute cost, and this splits it into its two functional parts: the player is the fitness evaluation, the reflector is the evolutionary operator. Tracking them separately reveals the overhead of reflection relative to raw play, and exposes prompt bloat — if player tokens climb generation over generation, the evolved prompt is growing longer and more expensive to run at inference time, a hidden cost of evolution.',
    goodBad: 'There is no universally \'good\' value — interpret trends. Flat or declining player tokens alongside rising solve rate is ideal (the agent gets better without getting more expensive). Steadily rising player tokens signal prompt bloat; weigh the accuracy gain against the inference cost. Reflector tokens are incurred once per generation and are zero for the final generation (which never reflects) — that zero is expected, not missing data.',
  },
  reasoningVerbosity: {
    title: 'Reasoning Verbosity vs Outcome',
    whatItMeasures: 'For each game, the total number of characters of chain-of-thought reasoning the agent produced across all of its guesses, plotted against whether the game was won or lost, colored by generation.',
    whyItMatters: 'It probes the relationship between deliberation and success. Does the evolved prompt make the agent reason more, and does more reasoning actually help? A positive association (won games cluster at higher verbosity) supports reasoning-eliciting prompt changes; a negative or null association suggests verbosity is wasted tokens — or even a symptom of the model floundering on hard words. It also surfaces whether reflection is inadvertently inflating reasoning length over generations.',
    goodBad: 'There is no single target length; look at separation. If won and lost games occupy clearly different verbosity ranges, reasoning length is informative. If the two outcomes are fully intermixed, verbosity is not predictive and any prompt change that merely increases it is paying tokens for nothing. Watch for runaway verbosity in later generations with no solve-rate benefit — a sign the reflector is rewarding length over substance.',
  },
  violationRate: {
    title: 'Constraint Violations per Game',
    whatItMeasures: 'The average number of constraint violations the agent commits per game in each generation. A violation occurs when the agent guesses a word that contradicts known feedback: placing a letter in a position already marked GRAY, re-using a letter confirmed absent, or putting a YELLOW letter back in the same position it was marked wrong.',
    whyItMatters: 'Violations are direct evidence that the agent is ignoring the feedback it has already received — the most fundamental failure mode in Wordle. A high violation rate means the prompt is not enforcing constraint-tracking reliably, regardless of solve rate. This metric separates \'lucky wins despite bad reasoning\' from \'wins because the agent actually understood the feedback\'. Prompt evolution should drive violations toward zero; a rising rate across generations means the evolved prompt is becoming harder for the player model to follow, often a sign of prompt bloat or conflicting rules.',
    goodBad: 'Zero is ideal. Values below 0.5/game are acceptable for weaker player models. Values consistently above 1.0/game indicate the player is routinely ignoring constraints — the prompt\'s constraint-checking instructions are not effective, or the player model lacks the instruction-following capacity to execute them. A rising trend across generations is a strong warning sign: longer, more detailed prompts are confusing the model rather than helping it. If violations rise while solve rate stays flat, the evolved rules are adding noise, not signal.',
  },
  wordDifficulty: {
    title: 'Per-Word Difficulty',
    whatItMeasures: 'Each answer word\'s win rate aggregated across all generations, sorted from hardest (lowest win rate) to easiest. The tooltip shows the number of games behind each rate.',
    whyItMatters: 'It identifies the agent\'s systematic blind spots — words it fails on repeatedly regardless of prompt generation. Persistent failures often share structure (rare letters, repeated letters, many near-anagrams) and point to concrete, targetable weaknesses the reflector should address. It also separates strategy problems from vocabulary problems: a word missed every time across many generations is unlikely to be fixed by yet another strategy tweak.',
    goodBad: 'A short, shrinking tail of hard words is good — it means failures are rare and idiosyncratic. A long flat tail of zero-win-rate words is a red flag: the agent has structural blind spots that prompt evolution is not resolving. Treat words with very few games (n = 1) cautiously — a single loss reads as 0% win rate but is statistically weak; rely on words with more games when diagnosing persistent weaknesses.',
  },
}

interface InfoPopupProps {
  metricKey: MetricKey
  open: boolean
  onClose: () => void
}

export default function InfoPopup({ metricKey, open, onClose }: InfoPopupProps) {
  const content = INFO_CONTENT[metricKey]
  const dialogRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!open) return
    const prev = document.activeElement as HTMLElement
    dialogRef.current?.focus()
    const handler = (e: KeyboardEvent) => { if (e.key === 'Escape') onClose() }
    document.addEventListener('keydown', handler)
    return () => {
      document.removeEventListener('keydown', handler)
      prev?.focus()
    }
  }, [open, onClose])

  if (!open) return null

  return (
    <div
      className="modal-overlay"
      onClick={onClose}
      role="presentation"
    >
      <div
        ref={dialogRef}
        className="modal"
        role="dialog"
        aria-modal="true"
        aria-labelledby="info-popup-title"
        tabIndex={-1}
        onClick={(e) => e.stopPropagation()}
        style={{ maxWidth: 560, maxHeight: '80vh', overflowY: 'auto' }}
      >
        <button className="modal-close" onClick={onClose}>✕</button>
        <h3 id="info-popup-title" style={{ marginBottom: 16 }}>{content.title}</h3>
        <div style={{ marginBottom: 12 }}>
          <div style={{ fontWeight: 600, fontSize: '0.85rem', marginBottom: 4, color: 'var(--text-secondary)' }}>WHAT IT MEASURES</div>
          <p style={{ fontSize: '0.9rem', lineHeight: 1.5 }}>{content.whatItMeasures}</p>
        </div>
        <div style={{ marginBottom: 12 }}>
          <div style={{ fontWeight: 600, fontSize: '0.85rem', marginBottom: 4, color: 'var(--text-secondary)' }}>WHY IT MATTERS FOR PROMPT EVOLUTION</div>
          <p style={{ fontSize: '0.9rem', lineHeight: 1.5 }}>{content.whyItMatters}</p>
        </div>
        <div style={{ marginBottom: 20 }}>
          <div style={{ fontWeight: 600, fontSize: '0.85rem', marginBottom: 4, color: 'var(--text-secondary)' }}>HOW TO READ GOOD VS. BAD VALUES</div>
          <p style={{ fontSize: '0.9rem', lineHeight: 1.5 }}>{content.goodBad}</p>
        </div>
        <div style={{ textAlign: 'right' }}>
          <button className="btn btn-secondary btn-sm" onClick={onClose}>Close</button>
        </div>
      </div>
    </div>
  )
}
