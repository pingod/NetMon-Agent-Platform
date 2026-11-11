import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { User, AlertCircle, Lock, ArrowRight, X } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { useAuth } from '../../../contexts/AuthContext';
import api from '../../../lib/api';

/** 根据后端错误响应提取用户友好的错误提示 */
function parseLoginError(err: unknown): string {
  // 优先取后端返回的 message 字段（RespondFailStatus 返回 {success:false, message:"..."}）
  if (typeof err === 'object' && err !== null && 'response' in err) {
    const response = err.response;
    if (typeof response === 'object' && response !== null && 'data' in response) {
      const data = response.data;
      if (typeof data === 'object' && data !== null) {
        const message =
          'message' in data && typeof data.message === 'string' ? data.message : '';
        const errorMessage =
          'error' in data && typeof data.error === 'string' ? data.error : '';
        if (message || errorMessage) return message || errorMessage;
      }
    }
  }
  if (err instanceof Error) return err.message;
  return '网络连接失败，请检查网络后重试';
}

export default function Login() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { login } = useAuth();
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError('');
    setLoading(true);

    try {
      const response = await api.post('/auth/login', { username, password });

      if (response.data.success) {
        login(response.data.data.token, response.data.data.user, response.data.data.refreshToken);

        // 检查是否需要强制修改密码
        if (response.data.data.user.passwordMustChange) {
          navigate('/force-password-change', { replace: true });
        } else {
          navigate('/dashboard');
        }
      } else {
        // success=false 但 HTTP 200（不太可能，防御性处理）
        setError(response.data.message || response.data.error || '登录失败，请检查用户名和密码');
      }
    } catch (err: unknown) {
      // 直接从后端响应提取错误消息，不再走 axios 拦截器的通用处理
      setError(parseLoginError(err));
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="auth-bg relative flex min-h-screen items-center justify-center overflow-hidden px-4">
      <div className="pointer-events-none absolute -left-24 -top-24 h-80 w-80 rounded-full bg-primary/20 blur-3xl" />
      <div className="pointer-events-none absolute -bottom-24 -right-24 h-80 w-80 rounded-full bg-accent/10 blur-3xl" />

      <div className="relative z-10 flex min-h-screen w-full max-w-md flex-col">
        <div className="flex flex-1 flex-col justify-center py-10">
          <div className="mb-9 text-center">
            <div className="mb-5 inline-flex h-[76px] w-[76px] items-center justify-center rounded-2xl bg-gradient-to-br from-purple-500 to-primary-strong shadow-[0_18px_50px_-18px_rgba(109,94,252,.9)]">
              <img src="/favicon.svg" alt="NetMon" className="h-12 w-12" />
            </div>
            <h1 className="text-[28px] font-semibold tracking-tight text-text-primary">NetMon</h1>
            <p className="mt-1 text-[12.5px] tracking-[0.14em] text-text-tertiary">
              {t('app.subtitle')}
            </p>
          </div>

          <div className="surface-card p-8 shadow-lift">
            <div className="mb-7 text-center">
              <h2 className="text-[19px] font-semibold text-text-primary">{t('login.title')}</h2>
              <p className="mt-1 text-[12.5px] text-text-tertiary">{t('login.hint')}</p>
            </div>

            {error && (
              <div className="mb-5 flex items-center gap-2.5 rounded-xl border border-rose-400/25 bg-rose-500/10 px-3.5 py-3 text-[12.5px] text-rose-600">
                <AlertCircle className="h-4 w-4 shrink-0" />
                <span className="flex-1 leading-relaxed">{error}</span>
                <button
                  type="button"
                  onClick={() => setError('')}
                  className="shrink-0 text-rose-600/70 transition-colors hover:text-rose-700"
                >
                  <X className="h-4 w-4" />
                </button>
              </div>
            )}

            <form onSubmit={handleSubmit} className="space-y-5">
              <div>
                <label className="mb-2 block text-[12.5px] font-medium text-text-secondary">
                  {t('login.username')}
                </label>
                <div className="group relative">
                  <User className="pointer-events-none absolute left-3.5 top-1/2 h-4 w-4 -translate-y-1/2 text-text-tertiary transition-colors group-focus-within:text-primary" />
                  <input
                    type="text"
                    value={username}
                    onChange={(e) => setUsername(e.target.value)}
                    placeholder={t('login.usernamePlaceholder')}
                    className="input-base h-11 pl-10"
                    required
                  />
                </div>
              </div>

              <div>
                <label className="mb-2 block text-[12.5px] font-medium text-text-secondary">
                  {t('login.password')}
                </label>
                <div className="group relative">
                  <Lock className="pointer-events-none absolute left-3.5 top-1/2 h-4 w-4 -translate-y-1/2 text-text-tertiary transition-colors group-focus-within:text-primary" />
                  <input
                    type="password"
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                    placeholder={t('login.passwordPlaceholder')}
                    className="input-base h-11 pl-10"
                    required
                  />
                </div>
              </div>

              <button type="submit" disabled={loading} className="btn btn-primary h-11 w-full">
                {loading ? (
                  <>
                    <svg className="h-4 w-4 animate-spin" viewBox="0 0 24 24">
                      <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" fill="none" />
                      <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z" />
                    </svg>
                    {t('login.submitting')}
                  </>
                ) : (
                  <>
                    {t('login.submit')}
                    <ArrowRight className="h-4 w-4" />
                  </>
                )}
              </button>
            </form>
          </div>
        </div>

        <div className="pb-8 text-center text-[11.5px] text-text-tertiary">
          © 2026 NetMon · NetMon Agent Platform
        </div>
      </div>
    </div>
  );
}
