import { defineStore } from 'pinia'
import { authApi, getToken, setToken } from '@/api/client'

// AuthMode 表示当前后端鉴权模式。
type AuthMode = 'unknown' | 'enabled' | 'disabled'

export const useAuthStore = defineStore('auth', {
  state: () => ({
    token: getToken(),
    mode: 'unknown' as AuthMode,
    authenticated: false,
    username: '',
    credentialType: '',
    canBootstrap: false,
    initialized: false,
  }),
  getters: {
    hasToken: (state) => state.token.length > 0,
    // 开发免鉴权模式。
    devNoAuth: (state) => state.mode === 'disabled',
    // 是否已具备进入后台的条件。
    canEnter: (state) => state.mode === 'disabled' || state.authenticated,
  },
  actions: {
    // 保存访问凭证到本地。
    saveToken(token: string) {
      this.token = token
      setToken(token)
    },
    clearSession() {
      this.saveToken('')
      this.authenticated = false
      this.username = ''
      this.credentialType = ''
    },
    // 从后端刷新当前身份状态（公开端点，登录前也可调用）。
    async fetchMe() {
      const me = await authApi.me()
      this.mode = me.auth_enabled ? 'enabled' : 'disabled'
      this.authenticated = me.authenticated
      this.username = me.username ?? ''
      this.credentialType = me.credential_type ?? ''
      this.canBootstrap = me.can_bootstrap
      this.initialized = true
      return me
    },
    // 首次访问时确保已识别鉴权模式。
    async ensureInitialized() {
      if (!this.initialized) {
        await this.fetchMe()
      }
    },
    // 校验并登录，成功后保存凭证并刷新身份。
    async login(username: string, password: string) {
      const res = await authApi.login(username, password)
      this.saveToken(res.token)
      await this.fetchMe()
    },
    // 创建首个管理员，成功后保存会话 token 并刷新身份。
    async bootstrap(username: string, password: string) {
      const created = await authApi.bootstrapAdmin(username, password)
      this.saveToken(created.token)
      await this.fetchMe()
      return created
    },
    async changePassword(currentPassword: string, newPassword: string) {
      await authApi.changePassword(currentPassword, newPassword)
      this.clearSession()
    },
    // 退出登录：清理本地凭证并回到未登录态。
    async logout() {
      try {
        await authApi.logout()
      } catch {
        // 后端不可达时仍清理本地凭证。
      }
      this.clearSession()
    },
  },
})
