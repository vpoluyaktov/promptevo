interface Props {
  guesses: string[]      // up to 6 lowercase 5-letter words
  feedbacks: string[]    // parallel array of 5-char G/Y/X codes
  animateLastRow?: boolean
}

function tileClass(fb: string, i: number): string {
  const c = fb[i]?.toUpperCase()
  if (c === 'G') return 'tile green'
  if (c === 'Y') return 'tile yellow'
  if (c === 'X') return 'tile gray'
  return 'tile filled'
}

export default function GameBoard({ guesses, feedbacks, animateLastRow }: Props) {
  const rows: Array<{ guess: string; feedback: string }> = []
  for (let i = 0; i < 6; i++) {
    rows.push({ guess: guesses[i] ?? '', feedback: feedbacks[i] ?? '' })
  }

  return (
    <div className="wordle-board">
      {rows.map((row, ri) => (
        <div key={ri} className="wordle-row">
          {Array.from({ length: 5 }, (_, ci) => {
            const letter = row.guess[ci] ?? ''
            const cls =
              row.feedback
                ? tileClass(row.feedback, ci) +
                  (animateLastRow && ri === guesses.length - 1 ? ' flip' : '')
                : row.guess
                ? 'tile filled'
                : 'tile'
            return (
              <div key={ci} className={cls}>
                {letter.toUpperCase()}
              </div>
            )
          })}
        </div>
      ))}
    </div>
  )
}
