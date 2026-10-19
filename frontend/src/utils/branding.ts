import { sanitizeUrl } from '@/utils/url'

// Keep fork-specific defaults here so upstream business pages stay unchanged.
export const DEFAULT_SITE_NAME = '狗站'
export const DEFAULT_SITE_SUBTITLE = '88dog · 你的 AI 接口小站'

export function resolveSiteName(value?: string): string {
  const name = value?.trim()
  return !name || name === 'Sub2API' ? DEFAULT_SITE_NAME : name
}

export function resolveSiteSubtitle(value?: string): string {
  const subtitle = value?.trim()
  return !subtitle || ['Subscription to API Conversion Platform', 'AI API Gateway Platform'].includes(subtitle)
    ? DEFAULT_SITE_SUBTITLE
    : subtitle
}

export function updateFavicon(logoUrl: string): void {
  const sanitizedLogoUrl = sanitizeUrl(logoUrl, {
    allowRelative: true,
    allowDataUrl: true,
  })
  if (!sanitizedLogoUrl) {
    return
  }

  let link = document.querySelector<HTMLLinkElement>('link[rel="icon"]')
  if (!link) {
    link = document.createElement('link')
    link.rel = 'icon'
    document.head.appendChild(link)
  }

  link.type = sanitizedLogoUrl.endsWith('.svg') ? 'image/svg+xml' : 'image/x-icon'
  link.href = sanitizedLogoUrl
}
