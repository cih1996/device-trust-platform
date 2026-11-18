<script setup>
import { ref, computed, watch } from 'vue'
import { api } from '../api.js'
import { route, setQuery } from '../router.js'
import { roleNames, report, revision } from '../store.js'
import { fullTime, relative } from '../format.js'

const teams = ref(null)
const settings = ref(null)
const busy = ref(false)
const saved = ref(false)
const reset = ref(null)
const selected = computed(() => route.query.team ?? '')

async function load() {
  try {
    const [t, s] = await Promise.all([api('platform/teams'), api('platform/settings')])
    teams.value = t
    settings.value = s
  } catch (e) {
    report(e)
  }
}
watch(revision, load, { immediate: true })

async function act(fn) {
  busy.value = true
  try {
    await fn()
    await load()
  } catch (e) {
    report(e)
  } finally {
    busy.value = false
  }
}
const saveSettings = () => act(async () => { await api('platform/settings', 'PUT', settings.value); saved.value = true; setTimeout(() => { saved.value = false }, 2000) })
const toggleTeam = t => act(() => api('platform/teams/' + t.id, 'PUT', { disabled: !t.disabled }))
const toggleUser = u => act(() => api('platform/users/' + encodeURIComponent(u.username), 'PUT', { disabled: !u.disabled }))
const resetPassword = u => act(async () => { reset.value = await api('platform/users/' + encodeURIComponent(u.username) + '/reset-password', 'POST', {}) })

const stats = computed(() => {
  const list = (teams.value || []).filter(t => t.id)
  return [
    ['团队', list.length],
    ['停用团队', list.filter(t => t.disabled).length],
    ['账号', (teams.value || []).reduce((s, t) => s + t.members.length, 0)],
    ['应用', (teams.value || []).reduce((s, t) => s + t.apps.length, 0)],
  ]
})
const current = computed(() => (teams.value || []).find(t => (t.id || 'platform') === selected.value) || null)
const lastLogin = t => t.members.map(m => m.lastLoginAt).filter(Boolean).sort().pop()
</script>

<template>
  <div>
    <div class="page-head">
      <div><h1>平台管理</h1><p>管理所有接入团队、账号和注册策略。团队之间数据隔离，只共享“同一手机在其他接入方是否高风险”的结论。</p></div>
    </div>

    <template v-if="teams && settings">
      <div class="grid g-4" style="margin-bottom:16px">
        <div v-for="[name, value] in stats" :key="name" class="card tile"><div class="label">{{ name }}</div><div class="value">{{ value }}</div></div>
      </div>

      <div class="grid g-2-1" style="margin-bottom:16px">
        <section class="card">
          <div class="card-head"><div><h2>团队</h2><span class="sub">点击查看成员</span></div></div>
          <div class="table-wrap">
            <table>
              <thead><tr><th>团队</th><th>创建人</th><th class="right">应用</th><th class="right">成员</th><th>创建时间</th><th>最近登录</th><th>状态</th></tr></thead>
              <tbody>
                <tr v-for="t in teams" :key="t.id || 'platform'" :class="['click', { on: selected === (t.id || 'platform') }]" @click="setQuery({ team: t.id || 'platform' })">
                  <td><strong>{{ t.name }}</strong><span v-if="t.apps.length" class="sub">{{ t.apps.map(a => a.name).join('、') }}</span></td>
                  <td>{{ t.owner || '—' }}</td>
                  <td class="right num">{{ t.apps.length }}</td>
                  <td class="right num">{{ t.members.length }}</td>
                  <td>{{ t.id ? fullTime(t.createdAt) : '—' }}</td>
                  <td>{{ lastLogin(t) ? relative(lastLogin(t)) : '—' }}</td>
                  <td>
                    <span v-if="!t.id" class="tag">平台</span>
                    <button v-else :class="['small', { danger: !t.disabled }]" :disabled="busy" @click.stop="toggleTeam(t)">{{ t.disabled ? '已停用 · 恢复' : '停用' }}</button>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </section>

        <form class="card" @submit.prevent="saveSettings">
          <div class="card-head"><h2>注册与配额</h2></div>
          <div class="card-body stack">
            <label class="check"><input v-model="settings.signupOpen" type="checkbox"><span>开放注册<small>任何人都可以在登录页注册新团队；关闭后只能通过团队邀请加入</small></span></label>
            <label class="field">每个团队的应用上限<input v-model.number="settings.maxAppsPerTeam" type="number" min="1" max="1000" required></label>
            <p class="muted" style="margin:0;font-size:13px">停用团队后，其成员立即退出登录，其应用的 SDK 上报和业务接口返回 403。</p>
            <div class="row-gap"><button class="primary" :disabled="busy">保存</button><span v-if="saved" class="muted">已保存</span></div>
          </div>
        </form>
      </div>

      <section v-if="current" class="card">
        <div class="card-head"><div><h2>{{ current.name }} · 成员</h2><span class="sub">{{ current.members.length }} 人</span></div></div>
        <div v-if="reset" class="card-body" style="padding-bottom:0">
          <p class="notice" style="margin:0">{{ reset.username }} 的新临时密码（只显示一次，原登录已失效）：<code class="mono">{{ reset.password }}</code></p>
        </div>
        <div class="table-wrap">
          <table>
            <thead><tr><th>账号</th><th>角色</th><th>加入时间</th><th>最近登录</th><th></th></tr></thead>
            <tbody>
              <tr v-for="m in current.members" :key="m.username">
                <td><strong>{{ m.username }}</strong><span v-if="m.disabled" class="tag bad" style="margin-left:6px">已停用</span></td>
                <td>{{ m.platform ? '平台管理员' : roleNames[m.role] || m.role }}</td>
                <td>{{ m.createdAt && !m.createdAt.startsWith('0001') ? fullTime(m.createdAt) : '—' }}</td>
                <td>{{ m.lastLoginAt ? relative(m.lastLoginAt) : '从未登录' }}</td>
                <td class="right">
                  <button class="small" :disabled="busy" @click="resetPassword(m)">重置密码</button>
                  <button v-if="!m.platform" :class="['small', { danger: !m.disabled }]" :disabled="busy" @click="toggleUser(m)">{{ m.disabled ? '恢复' : '停用' }}</button>
                </td>
              </tr>
            </tbody>
          </table>
          <div v-if="!current.members.length" class="empty">没有成员</div>
        </div>
      </section>
    </template>
  </div>
</template>
