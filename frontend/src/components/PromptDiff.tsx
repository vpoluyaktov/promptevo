import { useState, useMemo } from 'react'
import { diffLines } from 'diff'

interface Props {
  oldPrompt: string
  newPrompt: string
  title?: string
}

interface DiffLine {
  type: 'added' | 'removed' | 'unchanged'
  content: string
  lineNum: number
}

function computeDiff(oldText: string, newText: string): DiffLine[] {
  const changes = diffLines(oldText, newText)
  const lines: DiffLine[] = []
  let oldLineNum = 1
  let newLineNum = 1

  for (const change of changes) {
    const parts = change.value.split('\n')
    if (parts[parts.length - 1] === '') parts.pop()

    for (const content of parts) {
      if (change.added) {
        lines.push({ type: 'added', content, lineNum: newLineNum++ })
      } else if (change.removed) {
        lines.push({ type: 'removed', content, lineNum: oldLineNum++ })
      } else {
        lines.push({ type: 'unchanged', content, lineNum: newLineNum })
        oldLineNum++
        newLineNum++
      }
    }
  }

  return lines
}

function withContext(lines: DiffLine[], ctx: number): (DiffLine | null)[] {
  const include = new Set<number>()
  for (let i = 0; i < lines.length; i++) {
    if (lines[i].type !== 'unchanged') {
      for (let j = Math.max(0, i - ctx); j <= Math.min(lines.length - 1, i + ctx); j++) {
        include.add(j)
      }
    }
  }

  const result: (DiffLine | null)[] = []
  for (let i = 0; i < lines.length; i++) {
    if (include.has(i)) {
      result.push(lines[i])
    } else if (result.length > 0 && result[result.length - 1] !== null) {
      result.push(null)
    }
  }
  return result
}

const BG: Record<string, string> = { added: '#e6ffed', removed: '#ffeef0', unchanged: '#fff' }
const GUTTER: Record<string, string> = { added: '#cdffd8', removed: '#ffd7d5', unchanged: '#f6f8fa' }
const PREFIX: Record<string, string> = { added: '+', removed: '-', unchanged: ' ' }

export default function PromptDiff({ oldPrompt, newPrompt, title }: Props) {
  const [changesOnly, setChangesOnly] = useState(false)

  const allLines = useMemo(() => computeDiff(oldPrompt, newPrompt), [oldPrompt, newPrompt])
  const displayLines = useMemo(
    () => (changesOnly ? withContext(allLines, 1) : allLines as (DiffLine | null)[]),
    [allLines, changesOnly],
  )
  const hasChanges = allLines.some((l) => l.type !== 'unchanged')

  return (
    <div>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 12 }}>
        {title && <h4 style={{ fontSize: '0.9rem', fontWeight: 600, margin: 0 }}>{title}</h4>}
        <button
          onClick={() => setChangesOnly((v) => !v)}
          title={changesOnly ? 'Show all lines' : 'Show changed lines only'}
          aria-label={changesOnly ? 'Show all lines' : 'Show changed lines only'}
          style={{
            marginLeft: 'auto',
            background: changesOnly ? 'var(--accent)' : '#f0f0f0',
            color: changesOnly ? '#fff' : 'var(--text-secondary)',
            border: 'none',
            borderRadius: 4,
            padding: '3px 10px',
            fontSize: '0.75rem',
            fontWeight: 600,
            cursor: 'pointer',
            display: 'flex',
            alignItems: 'center',
            gap: 4,
          }}
        >
          ≡ Changes only
        </button>
      </div>

      <div style={{ border: '1px solid #e0e0e0', borderRadius: 8, overflow: 'hidden', fontSize: '0.8rem', fontFamily: 'monospace' }}>
        {!hasChanges ? (
          <div style={{ padding: '12px 16px', color: '#57606a', fontStyle: 'italic' }}>No changes</div>
        ) : (
          displayLines.map((line, i) => {
            if (line === null) {
              return (
                <div key={i} style={{ display: 'flex', background: '#f6f8fa', color: '#bbb', userSelect: 'none' }}>
                  <span style={{ minWidth: 40, padding: '1px 8px', textAlign: 'right', background: '#f0f0f0' }}>…</span>
                  <span style={{ minWidth: 20, padding: '1px 6px', background: '#f0f0f0' }} />
                  <span style={{ padding: '1px 8px' }}>...</span>
                </div>
              )
            }
            return (
              <div key={i} style={{ display: 'flex', background: BG[line.type], minHeight: '1.4em' }}>
                <span style={{ minWidth: 40, padding: '1px 8px', textAlign: 'right', color: '#57606a', background: GUTTER[line.type], flexShrink: 0, userSelect: 'none' }}>
                  {line.lineNum}
                </span>
                <span style={{ minWidth: 20, padding: '1px 6px', color: '#57606a', background: GUTTER[line.type], flexShrink: 0, userSelect: 'none' }}>
                  {PREFIX[line.type]}
                </span>
                <span style={{ padding: '1px 8px', whiteSpace: 'pre-wrap', wordBreak: 'break-word', overflowWrap: 'anywhere', flex: 1, minWidth: 0, color: '#24292e' }}>
                  {line.content}
                </span>
              </div>
            )
          })
        )}
      </div>
    </div>
  )
}
