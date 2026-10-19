import { beforeEach, describe, expect, it } from 'vitest'
import { resolveSiteName, resolveSiteSubtitle, updateFavicon } from '@/utils/branding'

describe('site branding', () => {
  it('uses 狗站 / 88dog for missing and upstream default branding', () => {
    expect(resolveSiteName()).toBe('狗站')
    expect(resolveSiteName('   ')).toBe('狗站')
    expect(resolveSiteName(' Sub2API ')).toBe('狗站')
    expect(resolveSiteSubtitle()).toBe('88dog · 你的 AI 接口小站')
    expect(resolveSiteSubtitle('Subscription to API Conversion Platform')).toBe('88dog · 你的 AI 接口小站')
    expect(resolveSiteSubtitle('AI API Gateway Platform')).toBe('88dog · 你的 AI 接口小站')
  })

  it('preserves administrator-provided branding', () => {
    expect(resolveSiteName(' 自定义站名 ')).toBe('自定义站名')
    expect(resolveSiteSubtitle(' 自定义介绍 ')).toBe('自定义介绍')
  })
})

describe('updateFavicon', () => {
  beforeEach(() => {
    document.head.innerHTML = '<link rel="icon" href="/logo.svg">'
  })

  it('replaces the default favicon with the configured logo', () => {
    updateFavicon('https://example.com/custom-logo.png')

    const link = document.querySelector<HTMLLinkElement>('link[rel="icon"]')
    expect(link?.href).toBe('https://example.com/custom-logo.png')
  })

  it('ignores unsafe logo URLs', () => {
    updateFavicon('javascript:alert(1)')

    const link = document.querySelector<HTMLLinkElement>('link[rel="icon"]')
    expect(link?.getAttribute('href')).toBe('/logo.svg')
  })
})
