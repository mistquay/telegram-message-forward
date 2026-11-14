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
const usernameFormRef = ref<FormInst | null>(null)
const saving = ref(false)
const changingUsername = ref(false)
const busy = computed(() => saving.value || changingUsername.value)
const form = reactive({ currentPassword: '', newPassword: '', confirmPassword: '' })
const usernameForm = reactive({ currentPassword: '', newUsername: '' })
const canManageAccount = computed(() => !auth.devNoAuth && auth.authenticated && auth.credentialType === 'session')

const usernameRules: FormRules = {
  currentPassword: { required: true, message: '请输入当前密码', trigger: 'blur' },
  newUsername: {
    required: true,
    trigger: 'blur',
    validator: (_rule, value: string) => {
      const username = value?.trim() ?? ''
      const length = Array.from(username).length
      if (length < 3 || length > 64 || /[\s\p{Cc}]/u.test(username)) return new Error('用户名需要 3～64 个字符，不能包含空白或控制字符')
      if (username === auth.username) return new Error('新用户名不能与当前用户名相同')
      return true
    },
  },
}

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
  if (busy.value || !canManageAccount.value) return
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
    await showCredentialError(error)
  } finally {
    saving.value = false
  }
}

async function changeUsername(): Promise<void> {
  if (busy.value || !canManageAccount.value) return
  try {
    await usernameFormRef.value?.validate()
  } catch {
    return
  }
  changingUsername.value = true
  try {
    await auth.changeUsername(usernameForm.currentPassword, usernameForm.newUsername.trim())
    Object.assign(usernameForm, { currentPassword: '', newUsername: '' })
    message.success('用户名已修改，全部登录会话已失效，请使用新用户名重新登录')
    await router.replace({ name: 'login' })
  } catch (error) {
    await showCredentialError(error)
  } finally {
    changingUsername.value = false
  }
}

async function showCredentialError(error: unknown): Promise<void> {
  message.error(errText(error))
  if (isAxiosError(error) && error.response?.status === 401) {
    auth.clearSession()
    await router.replace({ name: 'login' })
  }
}
</script>

<template>
  <n-card title="账户安全">
    <n-alert v-if="auth.devNoAuth" type="warning" :show-icon="true">
      当前已关闭管理 API 鉴权。请先开启鉴权并使用管理员账号登录，再修改用户名或密码。
    </n-alert>
    <n-alert v-else-if="!canManageAccount" type="info" :show-icon="true">
      修改用户名或密码需要管理员账号登录。请先退出，再使用用户名和密码登录。
    </n-alert>
    <n-space v-else vertical size="large">
      <n-text>当前账号：{{ auth.username }}。修改后，所有设备上的该账号都需要重新登录。</n-text>
      <div class="credential-forms">
        <n-form
          ref="usernameFormRef"
          class="credential-form"
          :model="usernameForm"
          :rules="usernameRules"
          :disabled="busy"
          label-placement="top"
          aria-label="修改用户名"
          @submit.prevent="changeUsername"
        >
          <h3 class="credential-heading">修改用户名</h3>
          <n-form-item label="新用户名" path="newUsername" :label-props="{ for: 'account-new-username' }">
            <n-input
              v-model:value="usernameForm.newUsername"
              :input-props="{ id: 'account-new-username', autocomplete: 'username' }"
              placeholder="3～64 个字符，不能包含空白"
            />
          </n-form-item>
          <n-form-item label="当前密码（验证身份）" path="currentPassword" :label-props="{ for: 'account-username-password' }">
            <n-input
              v-model:value="usernameForm.currentPassword"
              type="password"
              show-password-on="click"
              :input-props="{ id: 'account-username-password', autocomplete: 'current-password' }"
              placeholder="输入当前登录密码"
            />
          </n-form-item>
          <n-space vertical size="large">
            <n-text depth="3">修改后，请使用新用户名和原密码重新登录。</n-text>
            <n-button type="primary" attr-type="submit" :loading="changingUsername">修改用户名并重新登录</n-button>
          </n-space>
        </n-form>
        <n-form
          ref="formRef"
          class="credential-form"
          :model="form"
          :rules="rules"
          :disabled="busy"
          label-placement="top"
          aria-label="修改密码"
          @submit.prevent="changePassword"
        >
          <h3 class="credential-heading">修改密码</h3>
          <input type="text" :value="auth.username" name="username" autocomplete="username" hidden />
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
      </div>
      <n-alert type="info" :show-icon="true">
        API Token 不会随用户名或密码修改失效。如怀疑账号被入侵，请在下方 Token 管理中吊销不明凭证，并更换已泄露的 AI Key。
      </n-alert>
    </n-space>
  </n-card>
</template>

<style scoped>
.credential-forms {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 32px;
}

.credential-form {
  width: 100%;
  max-width: 480px;
  min-width: 0;
}

.credential-heading {
  margin: 0 0 16px;
  font-size: 16px;
  font-weight: 600;
}

@media (max-width: 900px) {
  .credential-forms {
    grid-template-columns: 1fr;
  }
}
</style>
