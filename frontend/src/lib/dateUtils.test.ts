import { describe, it, expect } from 'vitest'
import { formatDate } from './dateUtils'

describe('formatDate', () => {
  it('formats year precision correctly', () => {
    expect(formatDate('1989-01-01', 'year')).toBe('1989')
    expect(formatDate('2020-06-15', 'year')).toBe('2020')
  })

  it('formats month precision correctly', () => {
    expect(formatDate('1989-06-01', 'month')).toBe('June 1989')
    expect(formatDate('2020-12-01', 'month')).toBe('December 2020')
  })

  it('formats exact precision correctly', () => {
    expect(formatDate('2017-06-25', 'exact')).toBe('June 25, 2017')
    expect(formatDate('2020-12-15', 'exact')).toBe('December 15, 2020')
  })

  it('returns "Unknown date" for empty or null date', () => {
    expect(formatDate('', 'exact')).toBe('Unknown date')
    expect(formatDate(null as any, 'year')).toBe('Unknown date')
    expect(formatDate(undefined as any, 'month')).toBe('Unknown date')
  })
})