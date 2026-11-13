<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { isAxiosError } from 'axios'
import { useMessage, type FormInst, type FormRules } from 'naive-ui'
import { useAuthStore } from '@/stores/auth'
import { errText } from '@/utils/error'

const auth = useAuthStore()
const router = useRouter()
const message = useMessage()
const formRef = ref<FormInst | null>(null)
const saving = ref(false)
const form = reactive({ currentPassword: '', newPassword: '', confirmPassword: '' })
const canChangePassword = computed(() => !auth.devNoAuth && auth.authenticated && auth.credentialType === 'session')

const rules: FormRules = {
  currentPassword: { required: true, message: '请输入当前密码', trigger: 'blur' },
  newPassword: {
    required: true,
    trigger: 'blur',
    validator: (_rule, value: string) => {
      if (!value?.trim() || Array.from(value).length < 16) return new Error('新密码至少需要 16 个字符')
      if (new TextEncoder().encode(value).length > 72) return new Error('新密码不能超过 72 字节；中文等字符会占用多个字节')
      if (value === form.currentPassword) return new Error('新密码不能与当前密码相同')
      return true
    },
  },
  confirmPassword: {
    required: true,
    trigger: 'blur',
    validator: (_rule, value: string) => {
      if (!value) return new Error('请再次输入新密码')
      return value === form.newPassword || new Error('两次输入的新密码不一致')
    },
  },
}

async function changePassword(): Promise<void> {
  if (saving.value || !canChangePassword.value) return
  try {
    await formRef.value?.validate()
  } catch {
    return
  }
  saving.value = true
  try {
    await auth.changePassword(form.currentPassword, form.newPassword)
    Object.assign(form, { currentPassword: '', newPassword: '', confirmPassword: '' })
    message.success('密码已修改，全部登录会话已失效，请重新登录')
    await router.replace({ name: 'login' })
  } catch (error) {
    message.error(errText(error))
    if (isAxiosError(error) && error.response?.status === 401) {
      auth.clearSession()
      await router.replace({ name: 'login' })
    }
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <n-card title="账户安全">
    <n-alert v-if="auth.devNoAuth" type="warning" :show-icon="true">
      当前已关闭管理 API 鉴权。请先开启鉴权并使用管理员账号登录，再修改密码。
    </n-alert>
    <n-alert v-else-if="!canChangePassword" type="info" :show-icon="true">
      修改密码需要管理员账号登录。请先退出，再使用用户名和密码登录。
    </n-alert>
    <n-space v-else vertical size="large">
      <n-text>当前账号：{{ auth.username }}。修改后，所有设备上的该账号都需要重新登录。</n-text>
      <n-form
        ref="formRef"
        class="password-form"
        :model="form"
        :rules="rules"
        :disabled="saving"
        label-placement="top"
        @submit.prevent="changePassword"
      >
        <n-form-item label="当前密码" path="currentPassword" :label-props="{ for: 'account-current-password' }">
          <n-input
            v-model:value="form.currentPassword"
            type="password"
            show-password-on="click"
            :input-props="{ id: 'account-current-password', autocomplete: 'current-password' }"
            placeholder="输入当前登录密码"
          />
        </n-form-item>
        <n-form-item label="新密码" path="newPassword" :label-props="{ for: 'account-new-password' }">
          <n-input
            v-model:value="form.newPassword"
            type="password"
            show-password-on="click"
            :input-props="{ id: 'account-new-password', autocomplete: 'new-password' }"
            placeholder="至少 16 个字符，最多 72 字节"
          />
        </n-form-item>
        <n-form-item label="确认新密码" path="confirmPassword" :label-props="{ for: 'account-confirm-password' }">
          <n-input
            v-model:value="form.confirmPassword"
            type="password"
            show-password-on="click"
            :input-props="{ id: 'account-confirm-password', autocomplete: 'new-password' }"
            placeholder="再次输入新密码"
          />
        </n-form-item>
        <n-space vertical size="large">
          <n-text depth="3">建议使用密码管理器生成 24～32 位随机密码，并避免与其他网站重复。</n-text>
          <n-button type="primary" attr-type="submit" :loading="saving">修改密码并重新登录</n-button>
        </n-space>
      </n-form>
      <n-alert type="info" :show-icon="true">
        API Token 不会随改密失效。如怀疑账号被入侵，请在下方 Token 管理中吊销不明凭证，并更换已泄露的 AI Key。
      </n-alert>
    </n-space>
  </n-card>
</template>

<style scoped>
.password-form {
  width: 100%;
  max-width: 480px;
}
</style>
