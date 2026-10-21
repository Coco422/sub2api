export const DOCS_URL = 'https://doc.88888888.dog/'

export function getServiceLinks(locale: string) {
  const chinese = locale.startsWith('zh')
  return [
    { href: DOCS_URL, label: chinese ? '使用文档' : 'Documentation', icon: 'book' },
    { href: 'https://getplus.88888888.dog/', label: chinese ? '代充服务' : 'Recharge service', icon: 'creditCard' },
    { href: 'https://tutu.88888888.dog/create', label: chinese ? 'AI 绘画' : 'AI drawing', icon: 'sparkles' },
  ] as const
}
