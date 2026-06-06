import ReactDiffViewer from 'react-diff-viewer-continued'

interface Props {
  oldPrompt: string
  newPrompt: string
  title?: string
}

export default function PromptDiff({ oldPrompt, newPrompt, title }: Props) {
  return (
    <div>
      {title && (
        <h4 style={{ marginBottom: 12, fontSize: '0.9rem', fontWeight: 600 }}>{title}</h4>
      )}
      <div style={{ border: '1px solid #e0e0e0', borderRadius: 8, overflow: 'hidden', fontSize: '0.8rem' }}>
        <ReactDiffViewer
          oldValue={oldPrompt}
          newValue={newPrompt}
          splitView={false}
          useDarkTheme={false}
          showDiffOnly={false}
          hideLineNumbers={false}
        />
      </div>
    </div>
  )
}
