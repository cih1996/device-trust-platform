<script setup>
import { ref, computed, watch } from 'vue'
import { api } from '../api.js'
import { route, href, go } from '../router.js'
import { apps, loadApps, can } from '../store.js'
import { fullTime } from '../format.js'
import Icon from '../components/Icon.vue'

const app = computed(() => apps.value.find(a => a.id === route.id))
const policy = ref(null), security = ref(null), signers = ref(''), iosIds = ref('')
const saving = ref(''), saved = ref(''), message = ref(''), newKey = ref('')

watch(app, a => {
  if (!a) return
  policy.value = { ...a.policy }
  security.value = { ...a.security }
  signers.value = a.security.signingDigests.join('\n')
  iosIds.value = (a.security.iosAppIds || []).join('\n')
}, { immediate: true })

const thresholds = [
  ['reviewScore', '建议核验分数', '风险分达到后建议二次核验'],
  ['holdScore', '建议暂缓分数', '营销与支付场景，命中两类业务线索时暂缓'],
]
const rules = [
  ['registrationAccounts', '同设备注册账号数', '10 分钟'],
  ['ipRegistrations', '同网络注册账号数', '10 分钟'],
  ['campaignAccounts', '同设备参与活动账号数', '24 小时'],
  ['paymentMethods', '账号使用支付工具数', '24 小时'],
  ['paymentAccounts', '支付工具关联账号数', '24 小时'],
  ['paymentFailures', '支付失败次数', '1 小时'],
]

async function save(kind, fn) {
  saving.value = kind
  saved.value = ''
  message.value = ''
  try {
    await fn()
    await loadApps()
    saved.value = kind
  } catch (e) {
    message.value = e.message
  } finally {
    saving.value = ''
  }
}
const savePolicy = () => save('policy', () => api('apps/' + app.value.id + '/policy', 'PUT', policy.value))
const saveSecurity = () => save('security', () => api('apps/' + app.value.id + '/security', 'PUT', { ...security.value, signingDigests: signers.value.split(/\s+/).filter(Boolean), iosAppIds: iosIds.value.split(/\s+/).filter(Boolean) }))
function rotate() {
  if (!window.confirm('重置「' + app.value.name + '」的应用密钥？原密钥会立即失效，业务服务端需要同步更新。')) return
  save('key', async () => { newKey.value = (await api('apps/' + app.value.id + '/rotate-key', 'POST', {})).key })
}
</script>

<template>
  <div v-if="app && policy">
    <div class="crumbs"><a :href="href('apps')"><Icon name="back" />应用管理</a><span>/</span><span>{{ app.name }}</span></div>
    <div class="page-head">
      <div class="row-gap" style="gap:14px">
        <div class="app-icon">{{ app.name.slice(0, 1) }}</div>
        <div><h1>{{ app.name }}</h1><p class="mono">{{ app.package }}</p></div>
      </div>
      <div class="row-gap">
        <button @click="go('devices', '', { app: app.id })">设备</button>
        <button class="primary" @click="go('overview', '', { app: app.id })">查看风险</button>
      </div>
    </div>
    <p v-if="message" class="banner" role="alert">{{ message }}</p>

    <div class="stack" style="max-width:920px">
      <section class="card">
        <div class="card-head"><h2>接入信息</h2></div>
        <div class="card-body">
          <dl class="kv">
            <dt>应用 ID</dt><dd class="mono">{{ app.id }}</dd>
            <dt>包名</dt><dd class="mono">{{ app.package }}</dd>
            <dt>创建时间</dt><dd>{{ fullTime(app.createdAt) }}</dd>
          </dl>
        </div>
      </section>

      <form class="card" @submit.prevent="savePolicy">
        <div class="card-head"><div><h2>风险策略</h2><span class="sub">版本 {{ app.policy.version }}</span></div></div>
        <div class="card-body stack">
          <div>
            <div class="section-title">运行模式</div>
            <div class="form-grid">
              <label class="check"><input type="radio" value="shadow" v-model="policy.mode"><span>观察模式<small>只评分和记录，业务照常放行</small></span></label>
              <label class="check"><input type="radio" value="enforce" v-model="policy.mode"><span>执行模式<small>业务需按核验、暂缓建议处理后才能执行</small></span></label>
            </div>
          </div>
          <div>
            <div class="section-title">评分阈值</div>
            <div class="form-grid">
              <label v-for="[key, label, hint] in thresholds" :key="key" class="field">{{ label }}<input type="number" min="1" max="100" required v-model.number="policy[key]"><small>{{ hint }}</small></label>
            </div>
          </div>
          <div>
            <div class="section-title">关联规则阈值</div>
            <div class="form-grid">
              <label v-for="[key, label, window] in rules" :key="key" class="field"><span>{{ label }} <small>· {{ window }}</small></span><input type="number" min="1" required v-model.number="policy[key]"></label>
            </div>
          </div>
          <div class="row-gap"><button v-if="can('admin')" class="primary" :disabled="saving === 'policy'">保存策略</button><span v-if="saved === 'policy'" class="muted">已保存</span></div>
        </div>
      </form>

      <form class="card" @submit.prevent="saveSecurity">
        <div class="card-head"><h2>接入保护</h2></div>
        <div class="card-body stack">
          <div class="stack" style="gap:8px">
            <label class="check"><input type="checkbox" v-model="security.requireSignedReports"><span>签名上报<small>设备上报必须由硬件密钥（Keystore / Secure Enclave）签名，拒绝未签名数据</small></span></label>
            <label class="check"><input type="checkbox" v-model="security.requireHardware"><span>硬件密钥验证<small>要求 TEE / StrongBox 或 App Attest 硬件证明，未通过的设备无法登记</small></span></label>
            <label class="check"><input type="checkbox" v-model="security.allowAppAttestDevelopment"><span>接受 iOS 开发版证明<small>Xcode 开发签名的测试包；正式上线后建议关闭</small></span></label>
            <label class="check"><input type="checkbox" v-model="security.requireRequestBinding"><span>业务请求绑定<small>上报需携带业务请求摘要，防止设备数据被挪用到其他请求</small></span></label>
          </div>
          <div class="form-grid">
            <label class="field">应用证书 SHA-256 指纹<textarea v-model="signers" rows="3" placeholder="每行一个；留空表示不校验" class="mono"></textarea></label>
            <label class="field">iOS App ID<textarea v-model="iosIds" rows="3" placeholder="团队ID.包名，每行一个，例如 ABCDE12345.com.example.app" class="mono"></textarea></label>
            <label class="field">硬件证明有效期（小时）<input type="number" v-model.number="security.maxEnrollmentHours" min="1" max="168" required></label>
          </div>
          <div class="row-gap"><button v-if="can('admin')" class="primary" :disabled="saving === 'security'">保存接入保护</button><span v-if="saved === 'security'" class="muted">已保存</span></div>
        </div>
      </form>

      <section class="card">
        <div class="card-head"><h2>应用密钥</h2></div>
        <div class="card-body stack">
          <p class="muted" style="margin:0">业务服务端调用事件、结果和处置接口时使用。重置后原密钥立即失效。</p>
          <div v-if="newKey"><div class="section-title">新密钥（只显示一次）</div><code class="secret">{{ newKey }}</code></div>
          <div><button v-if="can('admin')" class="danger" :disabled="saving === 'key'" @click="rotate">重置密钥</button></div>
        </div>
      </section>
    </div>
  </div>
  <div v-else-if="apps.length" class="empty"><strong>应用不存在</strong><a :href="href('apps')">返回应用管理</a></div>
</template>
