export default {
  title: 'Traffic records', scope: 'HTTP/SSE payload bytes read and written for authenticated API requests, including failures and cancellations. Excludes WebSocket, Live/WebRTC, capability-link downloads without API keys, external media downloads, headers and network overhead. Written bytes do not prove client receipt. Statistics only; no traffic charges.',
  period: 'Period', periods: { day: 'Day', week: 'Week', month: 'Month', year: 'Year' }, date: 'Date within period', apply: 'Apply',
  previousPeriod: 'Previous period', nextPeriod: 'Next period', upload: 'Upload', download: 'Download', total: 'Total traffic', requests: 'Requests',
  since: 'First traffic record for these filters: {time}.', notRecorded: 'No traffic has been recorded for these filters.', history: 'Previously uncollected traffic cannot be recovered. Requests belong to their start time and are recorded after completion; statistics may lag a few seconds.',
  trend: 'Traffic trend', trendDetails: 'View trend details', bucket: 'Time', records: 'Request records', user: 'User', endpoint: 'Endpoint / request ID', result: 'Result', partialRead: 'Request body partially read',
  outcomes: { completed: 'HTTP handling completed', http_error: 'HTTP error', canceled: 'Canceled', write_error: 'Response write failed', panic: 'Handler exception' },
  empty: 'No recorded traffic in this period', loading: 'Loading traffic records…', failed: 'Failed to load traffic records. Please retry.', page: 'Page {page}', previous: 'Previous', next: 'Next'
}
