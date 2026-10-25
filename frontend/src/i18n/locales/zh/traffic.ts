export default {
  title: '流量记录', scope: '统计已认证 API 请求的 HTTP/SSE 正文读写量，包含失败与取消请求。暂不覆盖 WebSocket、Live/WebRTC、无 API Key 的能力链接下载和外部媒体下载。不含请求头及网络协议开销；响应写入量不代表客户端已完整接收。本期仅统计，不扣费。',
  period: '统计周期', periods: { day: '日', week: '周', month: '月', year: '年' }, date: '周期内日期', userId: '用户 ID（可选）', keyId: 'API Key ID（可选）', apply: '查询',
  previousPeriod: '上一周期', nextPeriod: '下一周期', upload: '上传流量', download: '下载流量', total: '总流量', requests: '请求数',
  since: '当前筛选下首条流量记录：{time}。', notRecorded: '当前筛选下尚无流量记录。', history: '历史未采集流量无法补全。单次请求按开始时间归属，结束后入账，统计可能延迟数秒。',
  trend: '流量趋势', trendDetails: '查看趋势明细', bucket: '时间', records: '请求明细', user: '用户', endpoint: '端点 / 请求 ID', result: '结果', partialRead: '请求体未完整读取',
  outcomes: { completed: 'HTTP 处理完成', http_error: 'HTTP 错误', canceled: '请求取消', write_error: '响应写入失败', panic: '处理异常' },
  empty: '所选周期暂无已记录流量', loading: '正在加载流量记录…', failed: '加载流量记录失败，请重试', invalidFilter: '筛选 ID 必须为正整数', page: '第 {page} 页', previous: '上一页', next: '下一页'
}
