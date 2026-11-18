<script setup>
import { ref, computed, onMounted } from 'vue'
import { api, setToken, setUser, username } from '../api.js'
import { go } from '../router.js'
const emit = defineEmits(['done'])

// An invite link opens #/join?code=…; otherwise sign in or create a team.
const inviteCode = new URLSearchParams(location.hash.split('?')[1] || '').get('code') || ''
const mode = ref(location.hash.startsWith('#/join') && inviteCode ? 'join' : 'login')
const signupOpen = ref(false)
const name = ref(mode.value === 'login' ? username.value : '')
const password = ref('')
const confirm = ref('')
const team = ref('')
const busy = ref(false)
const message = ref('')

onMounted(async () => {
  try {
    signupOpen.value = (await api('auth/options')).signupOpen
  } catch {
    signupOpen.value = false
  }
})

const titles = { login: '登录', register: '注册新团队', join: '加入团队' }
const title = computed(() => titles[mode.value])
function switchTo(next) {
  mode.value = next
  message.value = ''
  if (next !== 'login') name.value = ''
  password.value = confirm.value = ''
}

async function submit() {
  message.value = ''
  if (mode.value !== 'login' && password.value !== confirm.value) {
    message.value = '两次输入的密码不一致'
    return
  }
  busy.value = true
  try {
    const body = { username: name.value.trim(), password: password.value }
    const data = mode.value === 'login' ? await api('auth/login', 'POST', body)
      : mode.value === 'register' ? await api('auth/register', 'POST', { ...body, team: team.value.trim() })
      : await api('auth/join', 'POST', { ...body, code: inviteCode })
    setToken(data.token)
    setUser(data.username || name.value)
    password.value = confirm.value = ''
    // A new account lands on a page it may open; a new team starts at app creation.
    if (mode.value !== 'login') go(mode.value === 'register' ? 'apps' : 'overview')
    emit('done')
  } catch (e) {
    message.value = e.message
  } finally {
    busy.value = false
  }
}
</script>
<template>
  <div class="login">
    <form @submit.prevent="submit">
      <span class="mark">衡</span>
      <div><h1>知衡</h1><p class="muted" style="margin:2px 0 0">设备风控工作台 · {{ title }}</p></div>
      <p v-if="mode === 'join'" class="notice" style="margin:0">你收到了团队邀请，设置账号和密码后即可加入。</p>
      <label v-if="mode === 'register'" class="field">团队 / 公司名称<input v-model="team" required maxlength="40" placeholder="例如：某某科技" autofocus></label>
      <label class="field">账号<input v-model="name" autocomplete="username" required :autofocus="mode !== 'register'" :placeholder="mode === 'login' ? '' : '3–40 位小写字母、数字或 _ . @ -'"></label>
      <label class="field">密码<input v-model="password" type="password" :autocomplete="mode === 'login' ? 'current-password' : 'new-password'" required :minlength="mode === 'login' ? null : 6" :placeholder="mode === 'login' ? '' : '至少 6 位，含字母和数字'"></label>
      <label v-if="mode !== 'login'" class="field">确认密码<input v-model="confirm" type="password" autocomplete="new-password" required minlength="6"></label>
      <p v-if="message" class="error-text" role="alert">{{ message }}</p>
      <button class="primary" :disabled="busy">{{ busy ? '请稍候…' : mode === 'login' ? '登录' : mode === 'register' ? '注册并创建团队' : '加入团队' }}</button>
      <p class="muted" style="margin:0;font-size:13px;text-align:center">
        <template v-if="mode === 'login' && signupOpen">还没有账号？<a href="#" @click.prevent="switchTo('register')">注册新团队</a></template>
        <template v-else-if="mode !== 'login'">已有账号？<a href="#" @click.prevent="switchTo('login')">直接登录</a></template>
        <template v-else>账号由团队管理员邀请开通</template>
      </p>
    </form>
  </div>
</template>
