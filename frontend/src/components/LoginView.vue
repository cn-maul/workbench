<script setup lang="ts">
import { nextTick, onMounted, ref } from 'vue'
import { api } from '../api'
import { errToast } from '../useToast'

const emit = defineEmits<{ done: [] }>()
const pwd = ref('')
const busy = ref(false)
const input = ref<HTMLInputElement | null>(null)

onMounted(() => nextTick(() => input.value?.focus()))

async function submit() {
  if (!pwd.value || busy.value) return
  busy.value = true
  try {
    await api.login(pwd.value)
    emit('done')
  } catch (e) {
    errToast(e)
    pwd.value = ''
    input.value?.focus()
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="login-wrap">
    <div class="login-card">
      <div class="login-title">工作台</div>
      <div class="login-sub">此工作台已加密，请输入访问密码</div>
      <input ref="input" class="input" type="password" v-model="pwd" placeholder="密码" @keydown.enter="submit" />
      <button class="btn btn-primary" style="width:100%;margin-top:10px" :disabled="!pwd || busy" @click="submit">
        {{ busy ? '验证中…' : '进入' }}
      </button>
    </div>
  </div>
</template>

<style scoped>
.login-wrap {
  position: fixed; inset: 0; z-index: 200;
  display: flex; align-items: center; justify-content: center;
  background: var(--ui-page);
}
.login-card {
  width: 320px; padding: 24px;
  background: var(--ui-surface);
  border-radius: var(--radius-large); box-shadow: var(--ui-surface-shadow);
  display: flex; flex-direction: column; gap: 12px;
}
.login-title { font-size: 20px; font-weight: 600; text-align: center; }
.login-sub { font-size: 12px; color: var(--ui-muted); text-align: center; }
</style>
