<script setup>
import { ref, watch } from 'vue'
import { api } from '../api.js'
import { me, loadMe, roleNames, report, revision } from '../store.js'
import { fullTime, relative } from '../format.js'

const data = ref(null)
const inviteRole = ref('analyst')
const created = ref(null)
const busy = ref(false)
const teamName = ref('')
const roles = [['admin', '管理员', '管理应用、密钥、策略和成员'], ['analyst', '运营', '查看数据，复核和处置风险事件'], ['viewer', '只读', '只能查看']]

async function load() {
  try {
    data.value = await api('team')
    teamName.value = data.value.team.name
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
const link = code => location.origin + location.pathname + '#/join?code=' + code
const invite = () => act(async () => { created.value = await api('team/invites', 'POST', { role: inviteRole.value }) })
const revoke = code => act(() => api('team/invites/' + code, 'DELETE'))
const update = (m, patch) => act(() => api('team/members/' + encodeURIComponent(m.username), 'PUT', { role: m.role, disabled: !!m.disabled, ...patch }))
const rename = () => act(async () => { await api('team', 'PUT', { name: teamName.value }); await loadMe() })
function copy(text) {
  navigator.clipboard?.writeText(text).catch(report)
}
</script>

<template>
  <div>
    <div class="page-head">
      <div><h1>团队成员</h1><p>{{ me?.team?.name }} · 团队内成员共享本团队的应用和数据，其他团队看不到</p></div>
    </div>

    <template v-if="data">
      <div class="grid g-2-1" style="margin-bottom:16px">
        <section class="card">
          <div class="card-head"><div><h2>成员</h2><span class="sub">{{ data.members.length }} 人</span></div></div>
          <div class="table-wrap">
            <table>
              <thead><tr><th>账号</th><th>角色</th><th>状态</th><th>加入时间</th><th>最近登录</th></tr></thead>
              <tbody>
                <tr v-for="m in data.members" :key="m.username">
                  <td><strong>{{ m.username }}</strong><span v-if="m.username === me?.username" class="sub">我</span></td>
                  <td>
                    <span v-if="m.username === me?.username">{{ roleNames[m.role] }}</span>
                    <select v-else :value="m.role" :disabled="busy" aria-label="角色" @change="update(m, { role: $event.target.value })">
                      <option v-for="[k, l] in roles" :key="k" :value="k">{{ l }}</option>
                    </select>
                  </td>
                  <td>
                    <span v-if="m.username === me?.username" class="tag good">正常</span>
                    <button v-else :class="['small', { danger: !m.disabled }]" :disabled="busy" @click="update(m, { disabled: !m.disabled })">{{ m.disabled ? '恢复' : '停用' }}</button>
                  </td>
                  <td>{{ fullTime(m.createdAt) }}</td>
                  <td>{{ m.lastLoginAt ? relative(m.lastLoginAt) : '从未登录' }}</td>
                </tr>
              </tbody>
            </table>
          </div>
        </section>

        <section class="card">
          <div class="card-head"><div><h2>邀请成员</h2><span class="sub">链接 7 天内有效，只能使用一次</span></div></div>
          <div class="card-body stack">
            <label v-for="[k, l, hint] in roles" :key="k" class="check"><input v-model="inviteRole" type="radio" :value="k"><span>{{ l }}<small>{{ hint }}</small></span></label>
            <div><button class="primary" :disabled="busy" @click="invite">生成邀请链接</button></div>
            <div v-if="created" class="stack" style="gap:6px">
              <div class="section-title">把链接发给对方（{{ roleNames[created.role] }}）</div>
              <div class="row-gap"><code class="secret" style="flex:1;word-break:break-all">{{ link(created.code) }}</code><button class="small" @click="copy(link(created.code))">复制</button></div>
            </div>
          </div>
        </section>
      </div>

      <div class="grid g-2-1">
        <section class="card">
          <div class="card-head"><div><h2>未使用的邀请</h2><span class="sub">{{ data.invites.length }} 个</span></div></div>
          <div class="table-wrap">
            <table v-if="data.invites.length">
              <thead><tr><th>角色</th><th>创建人</th><th>过期时间</th><th></th></tr></thead>
              <tbody>
                <tr v-for="i in data.invites" :key="i.code">
                  <td>{{ roleNames[i.role] }}</td><td>{{ i.createdBy }}</td><td>{{ fullTime(i.expiresAt) }}</td>
                  <td class="right"><button class="small" @click="copy(link(i.code))">复制链接</button> <button class="small danger" :disabled="busy" @click="revoke(i.code)">作废</button></td>
                </tr>
              </tbody>
            </table>
            <div v-else class="empty">没有未使用的邀请</div>
          </div>
        </section>
        <form class="card" @submit.prevent="rename">
          <div class="card-head"><h2>团队信息</h2></div>
          <div class="card-body stack">
            <label class="field">团队名称<input v-model="teamName" required maxlength="40"></label>
            <div><button class="primary" :disabled="busy">保存</button></div>
          </div>
        </form>
      </div>
    </template>
  </div>
</template>
