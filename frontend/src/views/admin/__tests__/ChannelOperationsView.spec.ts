import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import ChannelOperationsView from '../ChannelOperationsView.vue'
import * as channels from '@/api/admin/channels'
import * as ops from '@/api/admin/channelOperations'

const state = vi.hoisted(() => ({ query: {} as Record<string, string>, replace: vi.fn() }))
vi.mock('vue-router', () => ({ useRoute: () => ({ query: state.query }), useRouter: () => ({ replace: state.replace }), RouterLink: { props: ['to'], template: '<a><slot /></a>' } }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/components/layout/AppLayout.vue', () => ({ default: { template: '<main><slot /></main>' } }))
vi.mock('@/components/admin/operations/SmartOpsNav.vue', () => ({ default: { template: '<nav />' } }))
vi.mock('@/components/common/BaseDialog.vue', () => ({ default: { props: ['show'], template: '<div v-if="show" data-testid="dialog"><slot /><slot name="footer" /></div>' } }))
vi.mock('@/api/admin/channels', () => ({ list: vi.fn(), getById: vi.fn(), create: vi.fn(), update: vi.fn(), remove: vi.fn() }))
vi.mock('@/api/admin/channelOperations', () => ({ catalog: vi.fn(), snapshot: vi.fn(), mutate: vi.fn() }))
const channel = { id: 7, name: 'Canonical', description: 'Keep pricing', status: 'active', group_ids: [11], model_mapping: { openai: { public: 'routing' } }, model_pricing: [{ models: ['public'] }] } as unknown as channels.Channel
const catalog: ops.Catalog = { revision: 42, upstreams: [{ id: 3, name: 'Provider', members: 1 }], upstream_total: 1, accounts: [{ id: 9, name: 'Account', platform: 'openai', status: 'active', schedulable: true, concurrency: 5, upstream_id: 3, eligible: true }], total: 1 }
const now = '2026-10-27T00:00:00Z'
const metrics: ops.Metrics = { requests: 3, samples: 1, unknown_samples: 2, duration_samples: 3, p50_ms: 123, p90_ms: 123, p95_ms: 123, duration_p90_ms: 456, latest_sample_at: now }
function snapshot(): ops.Snapshot {
  return { scope: { channel_id: 7, upstream_id: 0, account_id: 0, group_id: 0, model: '', model_scope: 'public', dimension: 'upstream' }, from: now, until: now, model_scope: 'public', usage: { status: 'ready', queried_at: now, data: { source: 'usage_logs', attribution: 'current', latency_measurement: 'legacy_forward', summary: { ...metrics }, total: 1, rows: [{ ...metrics, dimension_id: 0, name: 'Unassigned' }], lineage: [], lineage_total: 0, error_rate: null, error_scope: 'unsupported' } }, quality: { status: 'ready', queried_at: now, data: { source: 'scheduled_test_results', scope: 'current accounts', summary: { rounds: 1, passed: 0, degraded: 1, inconclusive: 0, latest_sample_at: now }, rows: [{ id: 99, plan_id: 8, account_id: 9, measured_model: 'provider-model', observed_at: now, probes: 2, passed_probes: 1, expected: 2, outcome: 'degraded', actions: ['failure_counted:1/2'] }], has_more: true } }, errors: { status: 'ready', queried_at: now, data: { source: 'ops_error_logs', scope: 'current accounts', records: 1, upstream_5xx: 1, latest_sample_at: now, rows: [{ error_type: 'upstream', error_phase: 'forward', status_code: 502, records: 1 }], bucket_total: 1, error_rate: null } }, capacity: { status: 'ready', queried_at: now, data: { source: 'accounts', scope: 'current', accounts: 1, schedulable: 1, configured_concurrency: 5 } } }
}
function button(wrapper: ReturnType<typeof mount>, text: string) { const found = wrapper.findAll('button').find(b => b.text() === text); if (!found) throw new Error(`Button missing: ${text}`); return found }
beforeEach(() => {
  vi.resetAllMocks(); state.query = { channel_id: '7' }
  vi.mocked(channels.list).mockResolvedValue({ items: [channel], total: 1 }); vi.mocked(channels.getById).mockResolvedValue(channel)
  vi.mocked(channels.update).mockResolvedValue(channel); vi.mocked(channels.create).mockResolvedValue(channel)
  vi.mocked(ops.catalog).mockResolvedValue(structuredClone(catalog)); vi.mocked(ops.snapshot).mockImplementation(async () => snapshot()); vi.mocked(ops.mutate).mockResolvedValue({ revision: 43 })
})
describe('channel operations workflow', () => {
  it('shows measured samples, current data scope and allows unassigned account drilldown without losing the channel denominator', async () => {
    const w = mount(ChannelOperationsView); await flushPromises()
    expect(w.text()).toContain('channelOps.unknown'); expect(w.text()).toContain('123 ms'); expect(w.text()).toContain('channelOps.qualityNote')
    expect(w.text()).toContain('channelOps.degraded'); expect(w.text()).toContain('failure_counted:1/2')
    await button(w, 'Unassigned').trigger('click'); await flushPromises()
    expect(ops.snapshot).toHaveBeenLastCalledWith(expect.objectContaining({ channel_id: 7, upstream_id: -1, dimension: 'account' }), expect.anything())
    w.unmount()
  })
  it('keeps query failure separate from successful no-data and never displays a zero metric for failure', async () => {
    const failed = snapshot(); failed.usage = { status: 'query_failed', data: null, queried_at: now, error: 'read failed' }
    vi.mocked(ops.snapshot).mockResolvedValue(failed)
    const w = mount(ChannelOperationsView); await flushPromises()
    expect(w.get('[data-testid="usage-section"]').text()).toContain('read failed'); expect(w.find('[data-testid="no-usage"]').exists()).toBe(false)
    expect(w.get('[data-testid="usage-section"]').find('dl').exists()).toBe(false)
    const empty = snapshot(); empty.usage.data!.summary.requests = 0; empty.usage.data!.rows = []
    vi.mocked(ops.snapshot).mockResolvedValue(empty); await button(w, 'channelOps.refresh').trigger('click'); await flushPromises()
    expect(w.get('[data-testid="no-usage"]').text()).toBe('channelOps.noUsage'); w.unmount()
  })
  it('requires a separate confirmation and submits both revision and previous upstream for detachment', async () => {
    const w = mount(ChannelOperationsView); await flushPromises()
    await button(w, 'channelOps.detach').trigger('click'); expect(ops.mutate).not.toHaveBeenCalled()
    await button(w, 'channelOps.confirm').trigger('click'); await flushPromises()
    expect(ops.mutate).toHaveBeenCalledWith(7, { action: 'detach', account_id: 9, expected_upstream_id: 3, confirm: true, revision: 42 })
    expect(channels.remove).not.toHaveBeenCalled(); w.unmount()
  })
  it('refreshes catalog after conflicts and leaves an actionable error instead of silently overwriting', async () => {
    vi.mocked(ops.mutate).mockRejectedValue({ response: { status: 409 } })
    const w = mount(ChannelOperationsView); await flushPromises()
    await button(w, 'channelOps.detach').trigger('click'); await button(w, 'channelOps.confirm').trigger('click'); await flushPromises()
    expect(w.text()).toContain('channelOps.conflict'); expect(ops.catalog).toHaveBeenCalledTimes(2); expect(ops.mutate).toHaveBeenCalledTimes(1); w.unmount()
  })
  it('updates only edited canonical basic fields and preserves billing/mappings/groups by omitting them', async () => {
    const w = mount(ChannelOperationsView); await flushPromises()
    await w.get('[data-testid="edit-channel"]').trigger('click')
    const form = w.get('[data-testid="dialog"] form'); await form.get('input').setValue('Renamed'); await form.trigger('submit'); await flushPromises()
    expect(channels.update).toHaveBeenCalledWith(7, { name: 'Renamed', description: 'Keep pricing', status: 'active' }); w.unmount()
  })
  it('uses exact selected model identity and group, paginates quality history by cursor', async () => {
    const w = mount(ChannelOperationsView); await flushPromises()
    const form = w.findAll('form')[1]
    await form.get('select').setValue('upstream'); await form.get('input[maxlength="200"]').setValue('upstream-alias'); await form.get('input[type="number"]').setValue(11); await form.trigger('submit'); await flushPromises()
    expect(ops.snapshot).toHaveBeenLastCalledWith(expect.objectContaining({ model_scope: 'upstream', model: 'upstream-alias', group_id: 11 }), expect.anything())
    await button(w, 'channelOps.older').trigger('click'); await flushPromises()
    expect(ops.snapshot).toHaveBeenLastCalledWith(expect.objectContaining({ quality_before: 99 }), expect.anything()); w.unmount()
  })
  it('supports a searched upstream picker and confirms reassignment against the previous membership', async () => {
    const targets = structuredClone(catalog); targets.upstreams.push({ id: 4, name: 'Provider B', members: 0 }); targets.upstream_total = 2
    vi.mocked(ops.catalog).mockResolvedValue(targets)
    const w = mount(ChannelOperationsView); await flushPromises()
    await button(w, 'channelOps.assign').trigger('click')
    const picker = w.get('[data-testid="dialog"]')
    await picker.get('input[type="radio"][value="4"]').setValue()
    await picker.get('input[maxlength="100"]').setValue('Provider'); await picker.get('form').trigger('submit'); await flushPromises()
    expect(ops.catalog).toHaveBeenLastCalledWith(7, expect.objectContaining({ upstream_search: 'Provider', upstream_page: 1 }), expect.anything())
    await button(w, 'channelOps.save').trigger('click'); expect(ops.mutate).not.toHaveBeenCalled()
    await button(w, 'channelOps.confirm').trigger('click'); await flushPromises()
    expect(ops.mutate).toHaveBeenCalledWith(7, expect.objectContaining({ action: 'assign', account_id: 9, upstream_id: 4, expected_upstream_id: 3, revision: 42, confirm: true })); w.unmount()
  })
  it('creates a real upstream mutation and requires confirmation before deleting the canonical channel', async () => {
    const w = mount(ChannelOperationsView); await flushPromises()
    await button(w, 'channelOps.createUpstream').trigger('click')
    await w.get('[data-testid="dialog"] input').setValue('New Provider'); await w.get('[data-testid="dialog"] form').trigger('submit'); await flushPromises()
    expect(ops.mutate).toHaveBeenCalledWith(7, { action: 'create_upstream', upstream_id: 0, name: 'New Provider', revision: 42 })
    await w.get('[data-testid="delete-channel"]').trigger('click'); expect(channels.remove).not.toHaveBeenCalled()
    await button(w, 'channelOps.confirm').trigger('click'); await flushPromises()
    expect(channels.remove).toHaveBeenCalledWith(7); w.unmount()
  })
  it('disables assignment for an ineligible member while retaining a confirmed detach path', async () => {
    const invalid = structuredClone(catalog); invalid.accounts[0].eligible = false
    vi.mocked(ops.catalog).mockResolvedValue(invalid)
    const w = mount(ChannelOperationsView); await flushPromises()
    expect(button(w, 'channelOps.assign').attributes('disabled')).toBeDefined()
    expect(button(w, 'channelOps.detach').attributes('disabled')).toBeUndefined()
    expect(w.text()).toContain('channelOps.ineligible'); w.unmount()
  })
  it('rejects a late response from the previous channel selection', async () => {
    let resolve!: (value: ops.Snapshot) => void
    vi.mocked(ops.snapshot).mockImplementationOnce(() => new Promise(r => { resolve = r }))
    const w = mount(ChannelOperationsView); await flushPromises()
    // A newer refresh can complete while a transport ignoring abort finishes late.
    const vm = w.vm as unknown as { refresh: () => Promise<void> }
    await vm.refresh(); await flushPromises()
    const late = snapshot(); late.usage.data!.summary.p50_ms = 999999; resolve(late); await flushPromises()
    expect(w.text()).not.toContain('999,999 ms'); expect(w.text()).toContain('123 ms'); w.unmount()
  })
})
