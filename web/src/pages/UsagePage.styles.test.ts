import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'

const usagePageStyles = readFileSync(new URL('./UsagePage.module.scss', import.meta.url), 'utf8')
const usagePageSource = readFileSync(new URL('./UsagePage.tsx', import.meta.url), 'utf8')

describe('UsagePage toolbar styles', () => {
  it('keeps visible range controls content-sized in narrow layouts', () => {
    expect(usagePageStyles).toMatch(/\.timeRangeGroup\s*\{[\s\S]*?width:\s*fit-content;/)
    expect(usagePageStyles).toMatch(/\.timeRangeSelectControl\s*\{[\s\S]*?flex:\s*0 0 164px;/)
  })

  it('only renders custom range inputs when the custom range is selected', () => {
    expect(usagePageSource).toContain('{isCustomRange && (')
    expect(usagePageSource).not.toContain('aria-hidden={!isCustomRange}')
  })
})
