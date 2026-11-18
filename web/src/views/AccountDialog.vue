<script setup>
import { ref } from 'vue'
import { api, username } from '../api.js'
const emit = defineEmits(['close'])
const current = ref(''), next = ref(''), confirm = ref('')
const busy = ref(false), message = ref(''), done = ref(false)
async function save() {
  message.value = ''
  if (next.value !== confirm.value) { message.value = '两次输入的新密码不一致'; return }
  busy.value = true
  try {
    await api('auth/password', 'PUT', { currentPassword: current.value, newPassword: next.value })
    done.value = true
  } catch (e) {
    message.value = e.message
  } finally {
    busy.value = false
  }
}
</script>
<template>
  <div class="overlay center" @click.self="emit('close')">
    <form class="dialog" @submit.prevent="save">
      <h2>修改密码</h2>
      <p class="muted" style="margin:0">当前账号：{{ username }}</p>
      <template v-if="!done">
        <label class="field">当前密码<input v-model="current" type="password" autocomplete="current-password" required></label>
        <label class="field">新密码<input v-model="next" type="password" autocomplete="new-password" minlength="6" required placeholder="至少 6 位，含字母和数字"></label>
        <label class="field">确认新密码<input v-model="confirm" type="password" autocomplete="new-password" minlength="6" required></label>
        <p v-if="message" class="error-text" role="alert">{{ message }}</p>
        <div class="row-gap" style="justify-content:flex-end">
          <button type="button" @click="emit('close')">取消</button>
          <button class="primary" :disabled="busy">保存</button>
        </div>
      </template>
      <template v-else>
        <p class="notice">密码已更新。</p>
        <div class="row-gap" style="justify-content:flex-end"><button type="button" class="primary" @click="emit('close')">完成</button></div>
      </template>
    </form>
  </div>
</template>
