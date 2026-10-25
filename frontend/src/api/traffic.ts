import { apiClient } from './client'

export type TrafficPeriod = 'day' | 'week' | 'month' | 'year'
export interface TrafficTotals {
  upload_bytes: number
  download_bytes: number
  total_bytes: number
  requests: number
}
export interface TrafficEvent {
  event_id: string
  user_id: number
  api_key_id: number
  group_id: number
  request_id: string
  endpoint: string
  method: string
  started_at: string
  finished_at: string
  upload_bytes: number
  download_bytes: number
  status_code: number
  outcome: string
  request_complete: boolean
  measurement_version: number
}
export interface TrafficReport extends TrafficTotals {
  items: TrafficEvent[]
  trend: (TrafficTotals & { bucket: string })[]
  has_more: boolean
  page: number
  page_size: number
  period: TrafficPeriod
  start: string
  end: string
  timezone: string
  recorded_since: string | null
  measurement_version: number
}
export interface TrafficQuery {
  period: TrafficPeriod
  date?: string
  user_id?: number
  api_key_id?: number
  page: number
  page_size: number
}
export async function getTraffic(admin: boolean, params: TrafficQuery, signal?: AbortSignal) {
  const { data } = await apiClient.get<TrafficReport>(admin ? '/admin/usage/traffic' : '/usage/traffic', { params, signal })
  return data
}

export function formatTrafficBytes(value: number): string {
  if (value < 1024) return `${value} B`
  const units = ['KiB', 'MiB', 'GiB', 'TiB', 'PiB']
  let scaled = value / 1024
  let index = 0
  while (scaled >= 1024 && index < units.length - 1) { scaled /= 1024; index++ }
  return `${scaled.toFixed(2)} ${units[index]}`
}
