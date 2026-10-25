import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import TrafficPanel from '../TrafficPanel.vue'
import Select from '@/components/common/Select.vue'
import type { TrafficReport } from '@/api/traffic'

const { getTraffic, searchUsers, searchApiKeys, listGroups } = vi.hoisted(() => ({
  getTraffic: vi.fn(), searchUsers: vi.fn(), searchApiKeys: vi.fn(), listGroups: vi.fn()
}))
vi.mock('@/api/admin', () => ({ adminAPI: { usage: { searchUsers, searchApiKeys }, groups: { list: listGroups } } }))
vi.mock('@/api/traffic', async importOriginal => ({ ...await importOriginal<typeof import('@/api/traffic')>(), getTraffic }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/api/client', () => ({ apiClient: { get: vi.fn() } }))
vi.mock('vue-chartjs', () => ({ Bar: { template: '<div data-testid="traffic-chart" />' } }))

const fixture = (): TrafficReport => ({
  upload_bytes: 1024, download_bytes: 2048, total_bytes: 3072, requests: 1,
  period: 'month', start: '2026-10-01T00:00:00+08:00', end: '2026-11-01T00:00:00+08:00',
  timezone: 'Asia/Shanghai', page: 1, page_size: 20, has_more: true,
  recorded_since: '2026-10-25T01:00:00+08:00', measurement_version: 1,
  trend: [{ bucket: '2026-10-25', upload_bytes: 1024, download_bytes: 2048, total_bytes: 3072, requests: 1 }],
  items: [{ event_id: 'event-1', user_id: 7, api_key_id: 8, group_id: 9, request_id: 'request-1',
    endpoint: '/v1/messages', method: 'POST', started_at: '2026-10-25T01:00:00+08:00', finished_at: '2026-10-25T01:01:00+08:00',
    upload_bytes: 1024, download_bytes: 2048, status_code: 499, outcome: 'canceled', request_complete: false, measurement_version: 1 }]
})
let wrappers: ReturnType<typeof mount>[] = []
afterEach(() => { wrappers.forEach(w => w.unmount()); wrappers = []; vi.useRealTimers() })
beforeEach(() => {
  vi.clearAllMocks(); getTraffic.mockResolvedValue(fixture())
  searchUsers.mockResolvedValue([{ id: 7, email: 'demo@example.test', deleted: false }])
  searchApiKeys.mockResolvedValue([{ id: 8, name: 'Production key' }])
})
const render = (admin = false) => { const wrapper = mount(TrafficPanel, { props: { admin, apiKeyOptions: [{ value: null, label: 'All keys' }, { value: 8, label: 'Production key' }, { value: 9, label: 'Test key' }] }, global: { stubs: { Teleport: true } } }); wrappers.push(wrapper); return wrapper }

describe('TrafficPanel', () => {
  it('renders traffic independently of token usage, with scope and partial results', async () => {
    const wrapper = render(); await flushPromises()
    expect(getTraffic).toHaveBeenCalledWith(false, expect.objectContaining({ period: 'month', page: 1 }), expect.any(AbortSignal))
    expect(wrapper.text()).toContain('3.00 KiB')
    expect(wrapper.text()).toContain('/v1/messages')
    expect(wrapper.text()).toContain('traffic.scope')
    expect(wrapper.text()).toContain('traffic.partialRead')
    expect(wrapper.text()).toContain('traffic.outcomes.canceled')
    expect(wrapper.text()).toContain('Asia/Shanghai')
    expect(wrapper.find('#traffic-user').exists()).toBe(false)
  })
  it('queries calendar periods, moves through history, paginates and resets filters', async () => {
    const wrapper = render(true); await flushPromises()
    vi.useFakeTimers()
    const userInput = wrapper.get('input[placeholder="admin.usage.searchUserPlaceholder"]')
    await userInput.trigger('focus'); await userInput.setValue('demo')
    await vi.advanceTimersByTimeAsync(300); await flushPromises()
    expect(searchUsers).toHaveBeenLastCalledWith('demo')
    await wrapper.findAll('button').find(b => b.text().includes('demo@example.test'))!.trigger('click'); await flushPromises()
    expect(searchApiKeys).toHaveBeenLastCalledWith(7, '')
    await wrapper.get('input[placeholder="admin.usage.searchApiKeyPlaceholder"]').trigger('focus')
    await wrapper.findAll('button').find(b => b.text().includes('Production key'))!.trigger('click'); await flushPromises()
    expect(listGroups).not.toHaveBeenCalled()
    expect(getTraffic).toHaveBeenLastCalledWith(true, expect.objectContaining({ user_id: 7, api_key_id: 8 }), expect.any(AbortSignal))
    const nextPage = wrapper.findAll('button').find(b => b.text() === 'traffic.next')!
    await nextPage.trigger('click'); await flushPromises()
    expect(getTraffic).toHaveBeenLastCalledWith(true, expect.objectContaining({ page: 2 }), expect.any(AbortSignal))
    await wrapper.get('#traffic-period').setValue('year'); await flushPromises()
    expect(getTraffic).toHaveBeenLastCalledWith(true, expect.objectContaining({ period: 'year', page: 1 }), expect.any(AbortSignal))
    await wrapper.findAll('button').find(b => b.text() === 'traffic.previousPeriod')!.trigger('click'); await flushPromises()
    expect(getTraffic).toHaveBeenLastCalledWith(true, expect.objectContaining({ date: '2025-10-01' }), expect.any(AbortSignal))
    await wrapper.findAll('button').find(b => b.text() === 'common.reset')!.trigger('click'); await flushPromises()
    expect(getTraffic).toHaveBeenLastCalledWith(true, expect.objectContaining({ period: 'month', page: 1 }), expect.any(AbortSignal))
    expect(getTraffic.mock.lastCall?.[1]).not.toHaveProperty('user_id')
    expect(getTraffic.mock.lastCall?.[1]).not.toHaveProperty('api_key_id')
    expect(wrapper.get('input[placeholder="admin.usage.searchUserPlaceholder"]').element).toHaveProperty('value', '')
    expect(wrapper.get('input[placeholder="admin.usage.searchApiKeyPlaceholder"]').element).toHaveProperty('value', '')
  })
  it('searches only the signed-in user’s key names and applies or clears their selection', async () => {
    const wrapper = render(); await flushPromises()
    const select = wrapper.getComponent(Select)
    expect(select.props('searchable')).toBe(true)
    expect(wrapper.get('#traffic-key').text()).toContain('All keys')
    await wrapper.get('#traffic-key').trigger('click')
    await wrapper.get('.select-search-input').setValue('Production')
    expect(wrapper.findAll('[role="option"]').map(option => option.text())).toEqual(['Production key'])
    await wrapper.get('[role="option"]').trigger('click'); await flushPromises()
    expect(getTraffic).toHaveBeenLastCalledWith(false, expect.objectContaining({ api_key_id: 8, page: 1 }), expect.any(AbortSignal))
    expect(getTraffic.mock.lastCall?.[1]).not.toHaveProperty('user_id')
    await wrapper.get('#traffic-key').trigger('click')
    await wrapper.findAll('[role="option"]').find(option => option.text() === 'All keys')!.trigger('click'); await flushPromises()
    expect(getTraffic.mock.lastCall?.[1]).not.toHaveProperty('api_key_id')
    await wrapper.findAll('button').find(b => b.text() === 'common.reset')!.trigger('click')
    expect(wrapper.get('#traffic-key').text()).toContain('All keys')
    expect(searchUsers).not.toHaveBeenCalled()
    expect(searchApiKeys).not.toHaveBeenCalled()
  })
  it('resets typed searches and cancels pending lookups', async () => {
    vi.useFakeTimers()
    const wrapper = render(true); await flushPromises()
    await wrapper.get('input[placeholder="admin.usage.searchUserPlaceholder"]').setValue('pending user')
    await wrapper.get('input[placeholder="admin.usage.searchApiKeyPlaceholder"]').setValue('pending key')
    await wrapper.findAll('button').find(b => b.text() === 'common.reset')!.trigger('click')
    await vi.advanceTimersByTimeAsync(300); await flushPromises()
    expect(searchUsers).not.toHaveBeenCalled()
    expect(searchApiKeys).not.toHaveBeenCalled()
    expect(wrapper.get('input[placeholder="admin.usage.searchUserPlaceholder"]').element).toHaveProperty('value', '')
    expect(wrapper.get('input[placeholder="admin.usage.searchApiKeyPlaceholder"]').element).toHaveProperty('value', '')
  })
  it('clears stale results on failure and distinguishes no records from historical collection', async () => {
    const wrapper = render(); await flushPromises()
    getTraffic.mockRejectedValueOnce(new Error('unavailable'))
    await wrapper.findAll('button').find(b => b.text() === 'common.refresh')!.trigger('click'); await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('traffic.failed')
    expect(wrapper.text()).not.toContain('/v1/messages')
    getTraffic.mockResolvedValueOnce({ ...fixture(), items: [], trend: [], recorded_since: null, requests: 0 })
    await wrapper.findAll('button').find(b => b.text() === 'common.refresh')!.trigger('click'); await flushPromises()
    expect(wrapper.text()).toContain('traffic.notRecorded')
    expect(wrapper.text()).toContain('traffic.history')
  })
  it('aborts outdated requests and never displays their responses', async () => {
    let resolveFirst!: (data: TrafficReport) => void
    getTraffic.mockImplementationOnce(() => new Promise<TrafficReport>(resolve => { resolveFirst = resolve }))
    const wrapper = render()
    const firstSignal = getTraffic.mock.calls[0]![2] as AbortSignal
    await wrapper.get('#traffic-period').setValue('day'); await flushPromises()
    expect(firstSignal.aborted).toBe(true)
    resolveFirst({ ...fixture(), items: [{ ...fixture().items[0]!, endpoint: '/stale' }] }); await flushPromises()
    expect(wrapper.text()).not.toContain('/stale')
    wrapper.unmount()
    expect((getTraffic.mock.lastCall![2] as AbortSignal).aborted).toBe(true)
  })
})
