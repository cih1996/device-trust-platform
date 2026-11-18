<script setup>
import { ref, computed, watch, onMounted, onBeforeUnmount } from 'vue'
import { api } from '../api.js'
import { href } from '../router.js'
import { appId, appName, changed, can } from '../store.js'
import { fullTime, levelNames } from '../format.js'
import { sceneNames, actionNames, reviewNames, statusNames, reasonName, sourceNames, fieldNames, outcomeNames, signed } from '../risk-display.js'
import ScoreRing from '../components/ScoreRing.vue'
import LevelTag from '../components/LevelTag.vue'
import ResourcePanel from '../components/ResourcePanel.vue'

const props = defineProps({ id: { type: String, required: true } })
const emit = defineEmits(['close'])
const e = ref(null)
const note = ref('')
const busy = ref(false)
const message = ref('')

async function load() {
  message.value = ''
  try {
    e.value = await api('events/' + encodeURIComponent(props.id))
    note.value = e.value.feedback?.note || ''
  } catch (err) {
    message.value = err.message
  }
}
watch(() => props.id, load, { immediate: true })

const proof = computed(() => (!e.value?.reportProof ? '未签名' : ['tee', 'strongbox'].includes(e.value.reportProof.level) ? '硬件证明通过' : '签名已验证'))
const canResolve = computed(() => e.value && e.value.mode === 'enforce' && e.value.action !== 'observe' && !e.value.resolution)
const context = computed(() => {
  const c = e.value?.context || {}
  return [['活动', c.campaignId], ['领取对象', c.recipientId], ['订单', c.orderId], ['支付工具', c.paymentMethodId], ['金额', c.amountMinor ? (c.amountMinor / 100).toFixed(2) + ' ' + (c.currency || '') : ''], ['页面', c.pageName], ['页面停留', c.pageDurationMs > 0 ? Math.round(c.pageDurationMs / 1000) + ' 秒' : '']].filter(([, v]) => v)
})

async function act(fn) {
  busy.value = true
  message.value = ''
  try {
    await fn()
    await load()
    changed()
  } catch (err) {
    message.value = err.message
  } finally {
    busy.value = false
  }
}
const review = label => act(() => api('events/' + e.value.id + '/reviews', 'POST', { label, note: note.value }))
const resolve = status => act(() => api('events/' + e.value.id + '/resolve', 'POST', { resolutionId: crypto.randomUUID(), status, note: note.value }))

const onKey = ev => ev.key === 'Escape' && emit('close')
onMounted(() => window.addEventListener('keydown', onKey))
onBeforeUnmount(() => window.removeEventListener('keydown', onKey))
</script>

<template>
  <div class="overlay" @click.self="emit('close')">
    <aside class="drawer" role="dialog" aria-label="事件详情">
      <div class="drawer-head">
        <div>
          <h2>事件详情</h2>
          <small v-if="e">{{ sceneNames[e.scene] || e.type }} · {{ fullTime(e.receivedAt) }}</small>
        </div>
        <button class="ghost" @click="emit('close')" aria-label="关闭">✕</button>
      </div>
      <div class="drawer-body" v-if="e">
        <div class="ring-wrap">
          <ScoreRing :score="e.score" />
          <div class="ring-text">
            <LevelTag :level="e.riskLevel" />
            <span class="row-gap">
              <span class="tag">{{ actionNames[e.action] || '观察' }}</span>
              <span :class="['tag', { warn: e.status === 'pending' }]">{{ statusNames[e.status] }}</span>
              <span class="tag">{{ proof }}</span>
            </span>
          </div>
        </div>

        <dl class="kv">
          <dt>应用</dt><dd>{{ appName(e.appId) }}</dd>
          <dt>账号</dt><dd>{{ e.accountId }}</dd>
          <dt>事件</dt><dd>{{ e.type }} <small class="mono">{{ e.eventId }}</small></dd>
          <dt>设备</dt><dd><a class="mono" :href="href('devices', e.profileId, { app: appId })" @click="emit('close')">{{ e.deviceId || e.installationId }}</a></dd>
          <dt>业务 IP</dt><dd class="mono">{{ e.clientIp || '未提供' }}</dd>
          <template v-for="[k, v] in context" :key="k"><dt>{{ k }}</dt><dd>{{ v }}</dd></template>
          <template v-if="e.outcome"><dt>支付结果</dt><dd>{{ outcomeNames[e.outcome.status] }}</dd></template>
          <template v-if="e.execution"><dt>业务执行</dt><dd>{{ e.execution.status === 'succeeded' ? '已完成' : '失败' }} · {{ e.execution.reference }}</dd></template>
        </dl>

        <div>
          <div class="section-title">命中线索 <small>阈值：核验 {{ e.policy.reviewScore }} · 暂缓 {{ e.policy.holdScore }}</small></div>
          <div v-for="(h, i) in e.hits" :key="i" class="factor">
            <span>{{ reasonName(h.code) }}</span><span class="pts">{{ signed(h.points) }}</span>
            <small>{{ sourceNames[h.source] || h.source }}<template v-if="h.threshold"> · 观测 {{ h.observed }} / 阈值 {{ h.threshold }}</template></small>
          </div>
          <p v-if="!e.hits.length" class="muted" style="margin:0">未命中规则</p>
          <p v-if="e.missingFields?.length" class="notice" style="margin:8px 0 0;font-size:13px">业务未提供：{{ e.missingFields.map(k => fieldNames[k] || k).join('、') }}，相关规则未参与评分</p>
        </div>

        <div>
          <div class="section-title">环境验真</div>
          <ResourcePanel :assessment="e.resources" />
        </div>

        <div>
          <div class="section-title">处理 <small v-if="e.feedback">已标记：{{ reviewNames[e.feedback.label] }}</small></div>
          <textarea v-model="note" rows="2" placeholder="处理备注（可选）" aria-label="处理备注"></textarea>
          <div v-if="canResolve && can('analyst')" class="row-gap" style="margin-top:8px">
            <button class="primary" :disabled="busy" @click="resolve('approved')">批准放行</button>
            <button class="danger" :disabled="busy" @click="resolve('rejected')">拒绝</button>
          </div>
          <div v-if="can('analyst')" class="row-gap" style="margin-top:8px">
            <button :disabled="busy" @click="review('fraud')">标记为异常</button>
            <button :disabled="busy" @click="review('legitimate')">标记为正常</button>
            <button class="ghost" :disabled="busy" @click="review('unknown')">待确认</button>
          </div>
          <p v-if="e.resolution" class="muted" style="font-size:13px;margin:8px 0 0">{{ e.resolution.status === 'approved' ? '已批准放行' : '已拒绝' }} · {{ fullTime(e.resolution.receivedAt) }}{{ e.resolution.note ? ' · ' + e.resolution.note : '' }}</p>
        </div>
        <p v-if="message" class="error-text" role="alert">{{ message }}</p>
      </div>
      <div class="drawer-body" v-else-if="message"><p class="error-text">{{ message }}</p></div>
    </aside>
  </div>
</template>
