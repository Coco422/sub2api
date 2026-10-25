<template>
  <section class="space-y-5 p-4 sm:p-6" :aria-label="t('traffic.title')" :aria-busy="loading">
    <p class="rounded-lg bg-sky-50 p-3 text-sm text-sky-900 dark:bg-sky-950/40 dark:text-sky-200">{{ t('traffic.scope') }}</p>
    <form class="flex flex-wrap items-end gap-3" @submit.prevent="applyFilters">
      <div>
        <label for="traffic-period" class="input-label">{{ t('traffic.period') }}</label>
        <select id="traffic-period" v-model="period" class="input">
          <option v-for="value in periods" :key="value" :value="value">{{ t(`traffic.periods.${value}`) }}</option>
        </select>
      </div>
      <div>
        <label for="traffic-date" class="input-label">{{ t('traffic.date') }}</label>
        <input id="traffic-date" :value="date || currentDate" type="date" class="input" @change="setDate" />
      </div>
      <div v-if="admin">
        <label for="traffic-user" class="input-label">{{ t('traffic.userId') }}</label>
        <input id="traffic-user" v-model="draftUserId" type="number" min="1" step="1" class="input w-36" />
      </div>
      <div>
        <label for="traffic-key" class="input-label">{{ t('traffic.keyId') }}</label>
        <input id="traffic-key" v-model="draftKeyId" type="number" min="1" step="1" class="input w-36" />
      </div>
      <button class="btn btn-primary" type="submit" :disabled="loading">{{ t('traffic.apply') }}</button>
      <button class="btn btn-secondary" type="button" :disabled="loading" @click="load">{{ t('common.refresh') }}</button>
      <button class="btn btn-secondary" type="button" @click="reset">{{ t('common.reset') }}</button>
    </form>
    <div class="flex flex-wrap items-center justify-between gap-3 text-sm">
      <div class="flex gap-2">
        <button class="btn btn-secondary btn-sm" type="button" :disabled="loading || !report" @click="shift(-1)">{{ t('traffic.previousPeriod') }}</button>
        <button class="btn btn-secondary btn-sm" type="button" :disabled="loading || !report" @click="shift(1)">{{ t('traffic.nextPeriod') }}</button>
      </div>
      <span v-if="report" class="text-gray-500 dark:text-gray-400">{{ rangeLabel }} · {{ report.timezone }}</span>
    </div>
    <p v-if="error" role="alert" class="rounded-lg bg-red-50 p-3 text-sm text-red-700 dark:bg-red-950/40 dark:text-red-300">{{ error }}</p>
    <p v-if="loading" role="status" class="py-8 text-center text-sm text-gray-500">{{ t('traffic.loading') }}</p>
    <template v-else-if="report">
      <div class="grid grid-cols-2 gap-3 lg:grid-cols-4">
        <div v-for="card in cards" :key="card.label" class="rounded-xl border border-gray-200 p-4 dark:border-dark-700">
          <p class="text-sm text-gray-500 dark:text-gray-400">{{ card.label }}</p>
          <p class="mt-2 text-xl font-semibold tabular-nums text-gray-900 dark:text-white" :title="card.raw">{{ card.value }}</p>
        </div>
      </div>
      <p class="text-xs text-gray-500 dark:text-gray-400">{{ report.recorded_since ? t('traffic.since', { time: displayTime(report.recorded_since) }) : t('traffic.notRecorded') }} {{ t('traffic.history') }}</p>
      <div v-if="report.requests" class="rounded-xl border border-gray-200 p-4 dark:border-dark-700">
        <h3 class="mb-3 text-sm font-medium">{{ t('traffic.trend') }}</h3>
        <div class="h-64" role="img" :aria-label="t('traffic.trend')"><Bar :data="chartData" :options="chartOptions" /></div>
      </div>
      <details v-if="report.requests" class="overflow-x-auto rounded-xl border border-gray-200 dark:border-dark-700">
        <summary class="cursor-pointer px-4 py-3 text-sm font-medium">{{ t('traffic.trendDetails') }}</summary>
        <table class="w-full text-left text-sm">
          <caption class="px-4 py-3 text-left font-medium">{{ t('traffic.trend') }} · {{ t('traffic.upload') }} / {{ t('traffic.download') }}</caption>
          <thead class="bg-gray-50 text-gray-500 dark:bg-dark-800"><tr><th class="px-4 py-2">{{ t('traffic.bucket') }}</th><th class="min-w-40 px-4 py-2">{{ t('traffic.upload') }}</th><th class="min-w-40 px-4 py-2">{{ t('traffic.download') }}</th><th class="px-4 py-2">{{ t('traffic.total') }}</th></tr></thead>
          <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
            <tr v-for="point in report.trend" :key="point.bucket">
              <td class="whitespace-nowrap px-4 py-2 font-mono text-xs">{{ point.bucket }}</td>
              <td v-for="direction in directions" :key="direction" class="px-4 py-2 tabular-nums" :title="`${point[direction]} B`">
                {{ bytes(point[direction]) }}
                <div class="mt-1 h-1.5 rounded bg-gray-100 dark:bg-dark-700"><div class="h-full rounded" :class="direction === 'upload_bytes' ? 'bg-sky-500' : 'bg-emerald-500'" :style="{ width: `${point[direction] / maxBytes * 100}%` }" /></div>
              </td>
              <td class="px-4 py-2 tabular-nums">{{ bytes(point.total_bytes) }}</td>
            </tr>
          </tbody>
        </table>
      </details>
      <div class="overflow-x-auto rounded-xl border border-gray-200 dark:border-dark-700">
        <table class="w-full text-left text-sm">
          <caption class="px-4 py-3 text-left font-medium">{{ t('traffic.records') }}</caption>
          <thead class="bg-gray-50 text-gray-500 dark:bg-dark-800"><tr><th class="px-4 py-2">{{ t('usage.time') }}</th><th v-if="admin" class="px-4 py-2">{{ t('traffic.user') }}</th><th class="px-4 py-2">API Key</th><th class="px-4 py-2">{{ t('traffic.endpoint') }}</th><th class="px-4 py-2">{{ t('traffic.upload') }}</th><th class="px-4 py-2">{{ t('traffic.download') }}</th><th class="px-4 py-2">{{ t('traffic.result') }}</th></tr></thead>
          <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
            <tr v-for="item in report.items" :key="item.event_id">
              <td class="whitespace-nowrap px-4 py-3 text-xs">{{ displayTime(item.started_at) }}</td>
              <td v-if="admin" class="px-4 py-3">#{{ item.user_id }}</td>
              <td class="px-4 py-3">#{{ item.api_key_id }}</td>
              <td class="px-4 py-3"><span class="whitespace-nowrap font-mono text-xs">{{ item.method }} {{ item.endpoint }}</span><span class="block text-xs text-gray-500" :title="item.event_id">{{ item.request_id || item.event_id }}</span></td>
              <td class="whitespace-nowrap px-4 py-3 tabular-nums" :title="`${item.upload_bytes} B`">{{ bytes(item.upload_bytes) }}<span v-if="!item.request_complete" class="block text-xs text-amber-700 dark:text-amber-400">{{ t('traffic.partialRead') }}</span></td>
              <td class="whitespace-nowrap px-4 py-3 tabular-nums" :title="`${item.download_bytes} B`">{{ bytes(item.download_bytes) }}</td>
              <td class="whitespace-nowrap px-4 py-3 text-xs">HTTP {{ item.status_code }}<span class="block" :class="item.outcome === 'completed' ? 'text-emerald-700 dark:text-emerald-400' : 'text-amber-700 dark:text-amber-400'">{{ t(`traffic.outcomes.${item.outcome}`) }}</span></td>
            </tr>
            <tr v-if="!report.items.length"><td :colspan="admin ? 7 : 6" class="p-8 text-center text-gray-500">{{ t('traffic.empty') }}</td></tr>
          </tbody>
        </table>
      </div>
      <div class="flex items-center justify-between text-sm">
        <span>{{ t('traffic.page', { page }) }}</span>
        <div class="flex gap-2"><button type="button" class="btn btn-secondary" :disabled="page <= 1" @click="page--">{{ t('traffic.previous') }}</button><button type="button" class="btn btn-secondary" :disabled="!report.has_more || page >= 10000" @click="page++">{{ t('traffic.next') }}</button></div>
      </div>
    </template>
  </section>
</template>

<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Chart as ChartJS, CategoryScale, LinearScale, BarElement, Tooltip, Legend, type ChartOptions } from 'chart.js'
import { Bar } from 'vue-chartjs'
import { getTraffic, formatTrafficBytes as bytes, type TrafficPeriod, type TrafficReport } from '@/api/traffic'

const props = withDefaults(defineProps<{ admin?: boolean }>(), { admin: false })
const { t } = useI18n()
ChartJS.register(CategoryScale, LinearScale, BarElement, Tooltip, Legend)
const periods: TrafficPeriod[] = ['day', 'week', 'month', 'year']
const directions = ['upload_bytes', 'download_bytes'] as const
const period = ref<TrafficPeriod>('month'), date = ref(''), page = ref(1)
const draftUserId = ref(''), draftKeyId = ref('')
const filters = ref<{ user_id?: number; api_key_id?: number }>({})
const report = ref<TrafficReport | null>(null), loading = ref(false), error = ref('')
let controller: AbortController | undefined
const siteDate = (value: string | Date) => {
  const parts = new Intl.DateTimeFormat('en-CA', { timeZone: report.value?.timezone || 'UTC', year: 'numeric', month: '2-digit', day: '2-digit' }).formatToParts(new Date(value))
  const get = (type: string) => parts.find(p => p.type === type)?.value || ''
  return `${get('year')}-${get('month')}-${get('day')}`
}
const currentDate = computed(() => report.value ? siteDate(new Date()) : '')
const displayTime = (value: string) => new Intl.DateTimeFormat(undefined, { timeZone: report.value?.timezone || 'UTC', dateStyle: 'short', timeStyle: 'medium' }).format(new Date(value))
const rangeLabel = computed(() => report.value ? `${siteDate(report.value.start)} — ${siteDate(new Date(new Date(report.value.end).getTime() - 1))}` : '')
const cards = computed(() => report.value ? [
  { label: t('traffic.upload'), value: bytes(report.value.upload_bytes), raw: `${report.value.upload_bytes} B` },
  { label: t('traffic.download'), value: bytes(report.value.download_bytes), raw: `${report.value.download_bytes} B` },
  { label: t('traffic.total'), value: bytes(report.value.total_bytes), raw: `${report.value.total_bytes} B` },
  { label: t('traffic.requests'), value: report.value.requests.toLocaleString(), raw: String(report.value.requests) }
] : [])
const maxBytes = computed(() => Math.max(1, ...(report.value?.trend.flatMap(p => [p.upload_bytes, p.download_bytes]) || [])))
const chartData = computed(() => ({
  labels: report.value?.trend.map(point => point.bucket) || [],
  datasets: [
    { label: t('traffic.upload'), data: report.value?.trend.map(point => point.upload_bytes) || [], backgroundColor: '#0ea5e9', borderRadius: 3 },
    { label: t('traffic.download'), data: report.value?.trend.map(point => point.download_bytes) || [], backgroundColor: '#10b981', borderRadius: 3 }
  ]
}))
const chartOptions = computed<ChartOptions<'bar'>>(() => ({
  responsive: true, maintainAspectRatio: false,
  plugins: { legend: { position: 'bottom' }, tooltip: { callbacks: { label: context => `${context.dataset.label}: ${bytes(Number(context.raw))}` } } },
  scales: { x: { stacked: true, grid: { display: false }, ticks: { maxRotation: 0, maxTicksLimit: 6 } }, y: { stacked: true, beginAtZero: true, ticks: { callback: value => bytes(Number(value)) } } }
}))
function setDate(event: Event) { date.value = (event.target as HTMLInputElement).value }
function applyFilters() {
  const user = Number(draftUserId.value), key = Number(draftKeyId.value)
  if ((draftUserId.value && (!Number.isSafeInteger(user) || user < 1)) || (draftKeyId.value && (!Number.isSafeInteger(key) || key < 1))) { error.value = t('traffic.invalidFilter'); return }
  filters.value = { ...(props.admin && user ? { user_id: user } : {}), ...(key ? { api_key_id: key } : {}) }
}
function reset() { draftUserId.value = ''; draftKeyId.value = ''; date.value = ''; period.value = 'month'; filters.value = {} }
function shift(direction: number) {
  if (!report.value) return
  const anchor = new Date(`${siteDate(report.value.start)}T00:00:00Z`)
  if (period.value === 'year') anchor.setUTCFullYear(anchor.getUTCFullYear() + direction)
  else if (period.value === 'month') anchor.setUTCMonth(anchor.getUTCMonth() + direction)
  else anchor.setUTCDate(anchor.getUTCDate() + direction * (period.value === 'week' ? 7 : 1))
  date.value = anchor.toISOString().slice(0, 10)
}
async function load() {
  controller?.abort()
  const current = new AbortController(); controller = current
  loading.value = true; error.value = ''; report.value = null
  try {
    const result = await getTraffic(props.admin, { period: period.value, date: date.value || undefined, page: page.value, page_size: 20, ...filters.value }, current.signal)
    if (!current.signal.aborted) report.value = result
  } catch {
    if (!current.signal.aborted) error.value = t('traffic.failed')
  } finally {
    if (!current.signal.aborted) loading.value = false
  }
}
watch([period, date, filters], () => { if (page.value !== 1) page.value = 1; else void load() }, { deep: true })
watch(page, () => { void load() })
void load()
onUnmounted(() => controller?.abort())
</script>
