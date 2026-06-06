import { Component, type ReactNode } from 'react'

interface Props { children: ReactNode }
interface State { error: Error | null }

export default class ErrorBoundary extends Component<Props, State> {
  state: State = { error: null }

  static getDerivedStateFromError(error: Error): State {
    return { error }
  }

  render() {
    if (this.state.error) {
      return (
        <div style={{ padding: '48px 24px', maxWidth: 600, margin: '0 auto' }}>
          <div className="error-box" style={{ marginBottom: 16 }}>
            <strong>Something went wrong.</strong>
            <div style={{ marginTop: 8, fontFamily: 'monospace', fontSize: '0.8rem' }}>
              {this.state.error.message}
            </div>
          </div>
          <button
            className="btn btn-secondary"
            onClick={() => { this.setState({ error: null }); window.history.back() }}
          >
            ← Go back
          </button>
          <button
            className="btn btn-secondary"
            style={{ marginLeft: 8 }}
            onClick={() => window.location.reload()}
          >
            Reload
          </button>
        </div>
      )
    }
    return this.props.children
  }
}
