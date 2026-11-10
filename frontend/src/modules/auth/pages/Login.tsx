import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { User, AlertCircle, Lock, ArrowRight, X } from 'lucide-react';
import { useAuth } from '../../../contexts/AuthContext';
import api from '../../../lib/api';

/** 根据后端错误响应提取用户友好的错误提示 */
function parseLoginError(err: unknown): string {
  // 优先取后端返回的 message 字段（RespondFailStatus 返回 {success:false, message:"..."}）
  if (err && typeof err === 'object' && 'response' in err) {
    const resp = (err as { response?: { status?: number; data?: { error?: string; message?: string } } }).response;
    if (resp) {
      const serverMsg = resp.data?.message || resp.data?.error;
      if (serverMsg) return serverMsg;
    }
  }
  if (err instanceof Error) return err.message;
  return '网络连接失败，请检查网络后重试';
}

export default function Login() {
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
    <div className="min-h-screen relative flex items-center justify-center overflow-hidden">
      {/* 背景 - 浅紫渐变白 */}
      <div className="absolute inset-0 bg-gradient-to-br from-purple-100 via-white to-purple-50">
        <div className="absolute top-0 left-0 w-96 h-96 bg-purple-300/30 rounded-full blur-3xl -translate-x-1/2 -translate-y-1/2 animate-pulse" />
        <div className="absolute bottom-0 right-0 w-96 h-96 bg-purple-400/20 rounded-full blur-3xl translate-x-1/2 translate-y-1/2 animate-pulse" style={{ animationDelay: '1s' }} />
        <div className="absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 w-[600px] h-[600px] bg-purple-200/20 rounded-full blur-3xl animate-pulse" style={{ animationDelay: '2s' }} />
      </div>

      {/* 主内容 */}
      <div className="relative z-10 w-full max-w-lg px-4 flex flex-col min-h-screen">
        {/* 主要内容区域 - 可伸缩 */}
        <div className="flex-1 flex flex-col justify-center">
          {/* Logo和标题 */}
          <div className="text-center mb-10">
            <div className="inline-flex items-center justify-center w-24 h-24 rounded-3xl bg-gradient-to-br from-purple-400 to-purple-600 shadow-2xl shadow-purple-500/30 mb-6">
              <img src="/favicon.svg" alt="NetMon" className="w-16 h-16" />
            </div>
            <h1 className="text-3xl font-bold text-gray-900 mb-2 tracking-tight">
              NetMon
            </h1>
            <p className="text-purple-500 text-sm tracking-wide">
              网络监控平台
            </p>
          </div>

          {/* 登录卡片 - 白色毛玻璃 */}
          <div className="bg-white/80 backdrop-blur-2xl rounded-3xl p-10 border border-purple-100 shadow-xl shadow-purple-200/30">
            <h2 className="text-2xl font-semibold text-gray-800 mb-8 text-center">
              用户登录
            </h2>

            {/* 错误提示 - 明显醒目的错误横幅 */}
            {error && (
              <div className="mb-6 p-4 bg-red-50 border-2 border-red-300 rounded-2xl flex items-center gap-3 text-red-700 shadow-sm shadow-red-100">
                <AlertCircle className="w-5 h-5 flex-shrink-0" />
                <span className="text-sm font-medium leading-relaxed flex-1">{error}</span>
                <button
                  type="button"
                  onClick={() => setError('')}
                  className="flex-shrink-0 text-red-400 hover:text-red-600 transition-colors"
                >
                  <X className="w-4 h-4" />
                </button>
              </div>
            )}

            <form onSubmit={handleSubmit} className="space-y-6">
              {/* 用户名 */}
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-3 ml-1">
                  用户名
                </label>
                <div className="relative group">
                  <div className="absolute inset-y-0 left-0 pl-4 flex items-center pointer-events-none">
                    <User className="w-5 h-5 text-gray-400 group-focus-within:text-purple-500 transition-colors" />
                  </div>
                  <input
                    type="text"
                    value={username}
                    onChange={(e) => setUsername(e.target.value)}
                    placeholder="请输入用户名"
                    className="w-full pl-12 pr-5 py-4 bg-white/60 border border-gray-200 rounded-2xl text-gray-900 placeholder-gray-400 focus:outline-none focus:ring-2 focus:ring-purple-500/30 focus:border-purple-400 transition-all duration-300 hover:border-purple-200"
                    required
                  />
                </div>
              </div>

              {/* 密码 */}
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-3 ml-1">
                  密码
                </label>
                <div className="relative group">
                  <div className="absolute inset-y-0 left-0 pl-4 flex items-center pointer-events-none">
                    <Lock className="w-5 h-5 text-gray-400 group-focus-within:text-purple-500 transition-colors" />
                  </div>
                  <input
                    type="password"
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                    placeholder="请输入密码"
                    className="w-full pl-12 pr-5 py-4 bg-white/60 border border-gray-200 rounded-2xl text-gray-900 placeholder-gray-400 focus:outline-none focus:ring-2 focus:ring-purple-500/30 focus:border-purple-400 transition-all duration-300 hover:border-purple-200"
                    required
                  />
                </div>
              </div>

              {/* 登录按钮 */}
              <button
                type="submit"
                disabled={loading}
                className="w-full py-4 bg-gradient-to-r from-purple-500 via-purple-600 to-purple-700 hover:from-purple-600 hover:via-purple-700 hover:to-purple-800 text-white font-semibold rounded-2xl transition-all duration-300 disabled:opacity-50 disabled:cursor-not-allowed shadow-xl shadow-purple-500/25 hover:shadow-xl hover:shadow-purple-500/40 transform hover:-translate-y-0.5 active:translate-y-0 flex items-center justify-center gap-2"
              >
                {loading ? (
                  <>
                    <svg className="animate-spin h-5 w-5" viewBox="0 0 24 24">
                      <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" fill="none" />
                      <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z" />
                    </svg>
                    登录中...
                  </>
                ) : (
                  <>
                    登录
                    <ArrowRight className="w-5 h-5" />
                  </>
                )}
              </button>
            </form>
          </div>
        </div>

        {/* 底部版权 */}
        <div className="py-8">
          <div className="flex items-center justify-center">
            <p className="text-gray-400 text-sm">
              © 2026 NetMon
            </p>
          </div>
        </div>
      </div>
    </div>
  );
}
