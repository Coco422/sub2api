<template>
  <BaseDialog :show="show" :title="tr('manageBlacklist')" width="wide" @close="emit('close')">
    <form id="ip-risk-blacklist-form" class="space-y-4" data-testid="risk-blacklist-form" @submit.prevent="apply">
      <p class="text-sm text-gray-500">{{ tr('blacklistDraftHint') }}</p>
      <div class="flex flex-wrap items-center justify-between gap-2">
        <span class="text-sm text-gray-500">{{ tr('ruleCount', { count: rules.length }) }}</span>
        <button type="button" class="btn btn-secondary btn-sm" data-testid="risk-add-rule" @click="addRule">{{ tr('addRule') }}</button>
      </div>
      <div v-for="(rule, index) in rules" :key="rule.key" class="grid gap-3 rounded-lg border border-gray-200 p-4 sm:grid-cols-2 dark:border-dark-700" :data-testid="`risk-rule-row-${index}`">
        <label class="text-sm">{{ tr('network') }}<input v-model.trim="rule.network" class="input mt-1 w-full font-mono" placeholder="198.51.100.0/24" :data-testid="`risk-rule-${index}`" required /></label>
        <label class="text-sm">{{ tr('note') }}<input v-model="rule.note" class="input mt-1 w-full" maxlength="256" :data-testid="`risk-rule-note-${index}`" /></label>
        <label class="text-sm">{{ tr('expires') }}<input v-model="rule.expiration" type="datetime-local" class="input mt-1 w-full" :data-testid="`risk-rule-expiry-${index}`" /></label>
        <div class="flex items-end justify-between gap-3">
          <label class="flex items-center gap-2 text-sm"><input v-model="rule.enabled" type="checkbox" :data-testid="`risk-rule-enabled-${index}`" />{{ tr('ruleEnabled') }}</label>
          <button type="button" class="btn btn-secondary btn-sm text-red-600" :aria-label="`${tr('remove')} ${rule.network || index + 1}`" :data-testid="`risk-rule-remove-${index}`" @click="rules.splice(index, 1)">{{ tr('remove') }}</button>
        </div>
      </div>
      <p v-if="!rules.length" class="rounded-lg bg-gray-50 p-6 text-center text-sm text-gray-500 dark:bg-dark-800">{{ tr('noRules') }}</p>
    </form>
    <template #footer>
      <button type="button" class="btn btn-secondary" data-testid="risk-blacklist-cancel" @click="emit('close')">{{ tr('cancel') }}</button>
      <button type="submit" form="ip-risk-blacklist-form" class="btn btn-primary" data-testid="risk-blacklist-apply">{{ tr('applyBlacklist') }}</button>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import type { IPBlockRule } from './api'

const props = defineProps<{ show: boolean; blacklist: IPBlockRule[] }>()
const emit = defineEmits<{ close: []; apply: [rules: IPBlockRule[]] }>()
const { t } = useI18n()
const tr = (key: string, values: Record<string, number> = {}) => t(`admin.settings.ipRisk.${key}`, values)
type RuleDraft = IPBlockRule & { key: number; expiration: string; originalExpiration: string }
const rules = ref<RuleDraft[]>([])
let nextKey = 0

function localDate(seconds: number) {
  if (!seconds) return ''
  const value = new Date(seconds * 1000)
  return new Date(value.getTime() - value.getTimezoneOffset() * 60000).toISOString().slice(0, 16)
}
watch(() => props.show, show => {
  if (show) rules.value = props.blacklist.map(rule => {
    const expiration = localDate(rule.expires_at)
    return { ...rule, key: nextKey++, expiration, originalExpiration: expiration }
  })
}, { immediate: true })

function addRule() {
  rules.value.push({ key: nextKey++, network: '', note: '', enabled: true, expires_at: 0, expiration: '', originalExpiration: '' })
}
function apply() {
  emit('apply', rules.value.map(rule => ({
    network: rule.network.trim(), note: rule.note, enabled: rule.enabled,
    expires_at: rule.expiration === rule.originalExpiration ? rule.expires_at : rule.expiration ? Math.floor(new Date(rule.expiration).getTime() / 1000) : 0
  })))
}
</script>
