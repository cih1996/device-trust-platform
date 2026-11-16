<script setup>
import { ref } from 'vue'
import { api, setToken, setUser, username } from '../api.js'
const emit = defineEmits(['done'])
const name = ref(username.value)
const password = ref('')
const busy = ref(false)
const message = ref('')
async function submit() {
  busy.value = true
  message.value = ''
  try {
    const data = await api('auth/login', 'POST', { username: name.value, password: password.value })
    setToken(data.token)
    setUser(data.username || name.value)
    password.value = ''
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
      <div><h1>知衡</h1><p class="muted" style="margin:2px 0 0">设备风控工作台</p></div>
      <label class="field">账号<input v-model="name" autocomplete="username" required autofocus></label>
      <label class="field">密码<input v-model="password" type="password" autocomplete="current-password" required></label>
      <p v-if="message" class="error-text" role="alert">{{ message }}</p>
      <button class="primary" :disabled="busy">{{ busy ? '登录中…' : '登录' }}</button>
    </form>
  </div>
</template>
