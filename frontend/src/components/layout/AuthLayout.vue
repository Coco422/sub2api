<template>
  <div class="brand-auth relative flex min-h-screen items-center justify-center overflow-hidden p-4">
    <!-- Content Container -->
    <div class="relative z-10 w-full max-w-md">
      <!-- Logo/Brand -->
      <div class="mb-8 text-center">
        <!-- Custom Logo or Default Logo -->
        <template v-if="settingsLoaded">
          <div class="mb-4 flex justify-center">
            <img
              :src="siteLogo || (useDefaultBrand ? '/branding/88dog-wordmark.png' : '/logo.svg?v=88dog-orange-20261021')"
              :alt="siteName"
              :class="useDefaultBrand ? 'brand-wordmark h-20 w-60' : 'h-16 w-16 rounded-2xl object-contain'"
            />
          </div>
          <h1 :class="useDefaultBrand ? 'sr-only' : 'mb-2 text-3xl font-bold text-gray-900 dark:text-white'">
            {{ siteName }}
          </h1>
          <p class="text-sm text-gray-500 dark:text-dark-400">
            {{ siteSubtitle }}
          </p>
        </template>
      </div>

      <!-- Card Container -->
      <div class="card-glass rounded-2xl p-8">
        <slot />
      </div>

      <!-- Footer Links -->
      <div class="mt-6 text-center text-sm">
        <slot name="footer" />
      </div>

      <!-- Copyright -->
      <div class="mt-8 text-center text-xs text-gray-400 dark:text-dark-500">
        &copy; {{ currentYear }} {{ siteName }}. All rights reserved.
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useAppStore } from '@/stores'
import { sanitizeUrl } from '@/utils/url'
import { DEFAULT_SITE_NAME, resolveSiteName, resolveSiteSubtitle } from '@/utils/branding'

const appStore = useAppStore()

const siteName = computed(() => resolveSiteName(appStore.siteName))
const siteLogo = computed(() => sanitizeUrl(appStore.siteLogo || '', { allowRelative: true, allowDataUrl: true }))
const useDefaultBrand = computed(() => !siteLogo.value && siteName.value === DEFAULT_SITE_NAME)
const siteSubtitle = computed(() => resolveSiteSubtitle(appStore.cachedPublicSettings?.site_subtitle))
const settingsLoaded = computed(() => appStore.publicSettingsLoaded)

const currentYear = computed(() => new Date().getFullYear())

onMounted(() => {
  appStore.fetchPublicSettings()
})
</script>
