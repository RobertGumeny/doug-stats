export function buildQuery(params: {
  provider?: string[]
  doug_only?: boolean
  project?: string
}): string {
  const parts: string[] = []

  if (params.provider && params.provider.length > 0) {
    for (const p of params.provider) {
      parts.push(`provider=${encodeURIComponent(p)}`)
    }
  }

  if (params.doug_only) {
    parts.push('doug_only=true')
  }

  if (params.project) {
    parts.push(`project=${encodeURIComponent(params.project)}`)
  }

  return parts.length > 0 ? `?${parts.join('&')}` : ''
}

export function formatCost(cost: number, unknown: boolean): string {
  if (unknown) return '?'
  return `$${cost.toFixed(4)}`
}

export function formatTimestamp(raw: string): string {
  const t = new Date(raw)
  if (Number.isNaN(t.getTime()) || t.getUTCFullYear() < 1971) {
    return 'Unknown'
  }

  return t.toLocaleString()
}

export function deriveDurationLabel(messages: Array<{ timestamp: string }>): number | null {
  let minMs = Number.POSITIVE_INFINITY
  let maxMs = Number.NEGATIVE_INFINITY

  for (const msg of messages) {
    const ms = Date.parse(msg.timestamp)
    if (Number.isNaN(ms)) {
      continue
    }

    if (ms < minMs) minMs = ms
    if (ms > maxMs) maxMs = ms
  }

  if (!Number.isFinite(minMs) || !Number.isFinite(maxMs) || maxMs <= minMs) {
    return null
  }

  return maxMs - minMs
}

export function formatDuration(durationMs: number): string {
  const totalSeconds = Math.floor(durationMs / 1000)
  const hours = Math.floor(totalSeconds / 3600)
  const minutes = Math.floor((totalSeconds % 3600) / 60)
  const seconds = totalSeconds % 60

  if (hours > 0) {
    return `${hours}h ${minutes}m ${seconds}s`
  }

  if (minutes > 0) {
    return `${minutes}m ${seconds}s`
  }

  return `${seconds}s`
}

export function isToolContentType(type: string): boolean {
  return type === 'tool_use' || type === 'tool_result'
}

export function getTextFromContentRaw(raw: unknown): string | null {
  if (typeof raw === 'string') {
    return raw
  }

  if (!raw || typeof raw !== 'object') {
    return null
  }

  if ('text' in raw && typeof raw.text === 'string') {
    return raw.text
  }

  if (Array.isArray(raw)) {
    const textParts: string[] = []
    for (const part of raw) {
      if (!part || typeof part !== 'object' || !('text' in part)) {
        continue
      }
      const value = (part as { text?: unknown }).text
      if (typeof value === 'string' && value.length > 0) {
        textParts.push(value)
      }
    }
    return textParts.length > 0 ? textParts.join('') : null
  }

  return null
}
