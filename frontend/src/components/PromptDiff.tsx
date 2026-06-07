import { useState } from 'react'
import ReactDiffViewer, { DiffMethod } from 'react-diff-viewer-continued'

interface Props {
  oldPrompt: string
  newPrompt: string
  title?: string
}

export default function PromptDiff({ oldPrompt, newPrompt, title }: Props) {
  const [changesOnly, setChangesOnly] = useState(false)

  return (
    <div>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 12 }}>
        {title && (
          <h4 style={{ fontSize: '0.9rem', fontWeight: 600, margin: 0 }}>{title}</h4>
        )}
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
      <div className="prompt-diff-wrap" style={{ border: '1px solid #e0e0e0', borderRadius: 8, overflow: 'hidden', fontSize: '0.8rem' }}>
        <ReactDiffViewer
          oldValue={oldPrompt}
          newValue={newPrompt}
          splitView={false}
          useDarkTheme={false}
          compareMethod={DiffMethod.LINES}
          disableWordDiff={true}
          showDiffOnly={changesOnly}
          extraLinesSurroundingDiff={1}
          hideLineNumbers={false}
        />
      </div>
    </div>
  )
}
