<script setup>
import { ref } from 'vue'
import { api } from '../api.js'
import { go } from '../router.js'
import { apps, loadApps, report, can } from '../store.js'
import { fullTime } from '../format.js'

const creating = ref(false)
const name = ref(''), pkg = ref('')
const busy = ref(false), message = ref('')
const created = ref(null)

async function create() {
  busy.value = true
  message.value = ''
  try {
    const data = await api('apps', 'POST', { name: name.value.trim(), package: pkg.value.trim() })
    created.value = data
    name.value = ''
    pkg.value = ''
    creating.value = false
    await loadApps()
  } catch (e) {
    message.value = e.message
  } finally {
    busy.value = false
  }
}
function copy(text) {
  navigator.clipboard?.writeText(text).catch(report)
}
</script>

<template>
  <div>
    <div class="page-head">
      <div><h1>应用管理</h1><p>每个接入的 App 拥有独立的策略、接入保护和应用密钥</p></div>
      <button v-if="can('admin')" class="primary" @click="creating = true">＋ 新建应用</button>
    </div>

    <div v-if="created" class="card" style="padding:16px;margin-bottom:16px;border-color:var(--accent)">
      <div class="row-gap" style="justify-content:space-between"><h2>「{{ created.app.name }}」已创建，请保存应用密钥</h2><button class="ghost small" @click="created = null">我已保存</button></div>
      <p class="muted" style="margin:6px 0 10px">密钥只显示这一次，请配置到业务服务端，不要放进 App 安装包。</p>
      <dl class="kv" style="margin-bottom:10px"><dt>应用 ID</dt><dd class="mono">{{ created.app.id }}</dd></dl>
      <div class="row-gap"><code class="secret" style="flex:1">{{ created.key }}</code><button @click="copy(created.key)">复制</button></div>
    </div>

    <div class="grid g-3">
      <article v-for="a in apps" :key="a.id" class="card app-card" @click="go('overview', '', { app: a.id })">
        <div class="top">
          <div class="app-icon">{{ a.name.slice(0, 1) }}</div>
          <div style="min-width:0"><h2>{{ a.name }}</h2><small class="mono">{{ a.package }}</small></div>
        </div>
        <div class="row-gap">
          <span :class="['tag', a.policy.mode === 'enforce' ? 'accent' : '']">{{ a.policy.mode === 'enforce' ? '执行模式' : '观察模式' }}</span>
          <span :class="['tag', a.security.requireSignedReports ? 'good' : 'warn']">{{ a.security.requireSignedReports ? '签名上报' : '签名未开启' }}</span>
          <span v-if="a.security.requireHardware" class="tag good">硬件证明</span>
          <span v-if="a.security.requireRequestBinding" class="tag">请求绑定</span>
        </div>
        <small>创建于 {{ fullTime(a.createdAt) }}</small>
        <div class="row-gap">
          <button class="primary small" @click.stop="go('overview', '', { app: a.id })">查看风险</button>
          <button class="small" @click.stop="go('devices', '', { app: a.id })">设备</button>
          <button class="small" @click.stop="go('apps', a.id)">设置</button>
        </div>
      </article>
    </div>
    <div v-if="!apps.length" class="card empty"><strong>还没有应用</strong>新建应用后，把应用 ID 配置到 SDK、把密钥配置到业务服务端即可开始接入</div>

    <div v-if="creating" class="overlay center" @click.self="creating = false">
      <form class="dialog" @submit.prevent="create">
        <h2>新建应用</h2>
        <label class="field">应用名称<input v-model="name" required autofocus placeholder="例如：商城 App"></label>
        <label class="field">Android 包名<input v-model="pkg" required placeholder="com.example.shop" pattern="[A-Za-z][\w]*(\.[A-Za-z][\w]*)+" title="例如 com.example.shop"></label>
        <p class="muted" style="margin:0;font-size:13px">默认开启签名上报和业务请求绑定，可在应用设置中调整。</p>
        <p v-if="message" class="error-text" role="alert">{{ message }}</p>
        <div class="row-gap" style="justify-content:flex-end">
          <button type="button" @click="creating = false">取消</button>
          <button class="primary" :disabled="busy">创建</button>
        </div>
      </form>
    </div>
  </div>
</template>
