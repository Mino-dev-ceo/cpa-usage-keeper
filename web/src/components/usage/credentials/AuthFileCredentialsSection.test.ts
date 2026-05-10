import { describe, expect, it, vi } from 'vitest'
import { formatQuotaResetLabel } from './AuthFileCredentialsSection'

describe('AuthFileCredentialsSection quota reset formatting', () => {
  it('formats reset labels with days when remaining time exceeds 24 hours', () => {
    vi.setSystemTime(new Date('2026-05-10T10:00:00Z'))
    try {
      expect(formatQuotaResetLabel('2026-05-12T10:15:00Z')).toBe('2d0h15m(05/12 18:15)')
    } finally {
      vi.useRealTimers()
    }
  })

  it('formats reset labels without days when remaining time is under 24 hours', () => {
    vi.setSystemTime(new Date('2026-05-10T10:00:00Z'))
    try {
      expect(formatQuotaResetLabel('2026-05-10T14:15:00Z')).toBe('4h15m(05/10 22:15)')
    } finally {
      vi.useRealTimers()
    }
  })
})
