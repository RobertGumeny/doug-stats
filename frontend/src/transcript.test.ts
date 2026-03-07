import { describe, expect, it } from 'vitest'

import {
  buildQuery,
  deriveDurationLabel,
  formatCost,
  formatDuration,
  getTextFromContentRaw,
  isToolContentType,
} from './transcript'

describe('buildQuery', () => {
  it('returns empty string without params', () => {
    expect(buildQuery({})).toBe('')
  })

  it('encodes all provider filters, project, and doug-only', () => {
    expect(
      buildQuery({
        provider: ['claude', 'gemini', 'codex'],
        doug_only: true,
        project: '/tmp/my repo',
      }),
    ).toBe('?provider=claude&provider=gemini&provider=codex&doug_only=true&project=%2Ftmp%2Fmy%20repo')
  })
})

describe('formatCost', () => {
  it('shows question mark for unknown', () => {
    expect(formatCost(9.1234, true)).toBe('?')
  })

  it('formats usd with four decimals', () => {
    expect(formatCost(1.2, false)).toBe('$1.2000')
  })
})

describe('deriveDurationLabel', () => {
  it('returns duration from min to max message timestamp', () => {
    const durationMs = deriveDurationLabel([
      { timestamp: '2026-01-01T00:00:10Z' },
      { timestamp: '2026-01-01T00:00:45Z' },
      { timestamp: '2026-01-01T00:00:20Z' },
    ])

    expect(durationMs).toBe(35000)
  })

  it('returns null when duration cannot be derived', () => {
    expect(deriveDurationLabel([{ timestamp: 'not-a-time' }])).toBeNull()
    expect(deriveDurationLabel([{ timestamp: '2026-01-01T00:00:00Z' }])).toBeNull()
  })
})

describe('formatDuration', () => {
  it('formats short durations', () => {
    expect(formatDuration(12000)).toBe('12s')
  })

  it('formats minute and hour durations', () => {
    expect(formatDuration(63000)).toBe('1m 3s')
    expect(formatDuration(3665000)).toBe('1h 1m 5s')
  })
})

describe('content helpers', () => {
  it('identifies tool content types', () => {
    expect(isToolContentType('tool_use')).toBe(true)
    expect(isToolContentType('tool_result')).toBe(true)
    expect(isToolContentType('text')).toBe(false)
  })

  it('extracts text from common content structures', () => {
    expect(getTextFromContentRaw('hello')).toBe('hello')
    expect(getTextFromContentRaw({ type: 'text', text: 'hi' })).toBe('hi')
    expect(
      getTextFromContentRaw([
        { type: 'text', text: 'a' },
        { type: 'text', text: 'b' },
      ]),
    ).toBe('ab')
    expect(getTextFromContentRaw({ type: 'tool_use', name: 'read_file' })).toBeNull()
  })
})
