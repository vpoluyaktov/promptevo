import { useEffect, useRef, useCallback } from 'react'
import type { SSEEvent } from '../api/types'
import { getToken } from '../auth'

type Handler = (event: SSEEvent) => void

export function useRunStream(runId: number | null, onEvent: Handler) {
  const handlerRef = useRef<Handler>(onEvent)
  handlerRef.current = onEvent

  const esRef = useRef<EventSource | null>(null)
  const retryRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const closedRef = useRef(false)

  const connect = useCallback(() => {
    if (closedRef.current || runId === null) return

    const token = getToken()
    const url = token
      ? `/api/runs/${runId}/stream?token=${encodeURIComponent(token)}`
      : `/api/runs/${runId}/stream`
    const es = new EventSource(url)
    esRef.current = es

    es.onmessage = (e: MessageEvent) => {
      try {
        const evt = JSON.parse(e.data as string) as SSEEvent
        handlerRef.current(evt)
        if (evt.type === 'run_end') {
          closedRef.current = true
          es.close()
        }
      } catch {
        // ignore malformed events
      }
    }

    es.onerror = () => {
      es.close()
      if (!closedRef.current) {
        retryRef.current = setTimeout(connect, 3000)
      }
    }
  }, [runId])

  useEffect(() => {
    closedRef.current = false
    connect()
    return () => {
      closedRef.current = true
      if (retryRef.current) clearTimeout(retryRef.current)
      esRef.current?.close()
    }
  }, [connect])
}
