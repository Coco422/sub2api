import { beforeEach, describe, expect, it, vi } from 'vitest'
import { formatTrafficBytes, getTraffic } from '../traffic'
const { get } = vi.hoisted(() => ({ get: vi.fn() }))
vi.mock('../client', () => ({ apiClient: { get } }))
beforeEach(() => vi.clearAllMocks())
describe('traffic API', () => {
  it('uses separate authenticated user and admin routes', async () => {
    get.mockResolvedValue({ data: { requests: 0 } })
    const params = { period: 'year' as const, page: 1, page_size: 20 }
    await getTraffic(false, params)
    expect(get).toHaveBeenLastCalledWith('/usage/traffic', { params, signal: undefined })
    await getTraffic(true, params)
    expect(get).toHaveBeenLastCalledWith('/admin/usage/traffic', { params, signal: undefined })
  })
  it('formats binary units without changing raw metering values', () => {
    expect(formatTrafficBytes(0)).toBe('0 B')
    expect(formatTrafficBytes(1023)).toBe('1023 B')
    expect(formatTrafficBytes(1024)).toBe('1.00 KiB')
    expect(formatTrafficBytes(1024 ** 3)).toBe('1.00 GiB')
  })
})
