import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useMutation } from '@tanstack/react-query';
import { Lock, AlertCircle, CheckCircle2, Loader2 } from 'lucide-react';
import { useAuth } from '../../../contexts/AuthContext';
import api from '../../../lib/api';
import { getAxiosErrorMessage } from '../../../lib/errorHandler';
import { validatePassword, getPasswordStrength } from '../../../utils/passwordValidator';

export default function ForcePasswordChange() {
  const navigate = useNavigate();
  const { updateUser } = useAuth();
  
  const [currentPassword, setCurrentPassword] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [passwordError, setPasswordError] = useState('');

  const changePasswordMutation = useMutation({
    mutationFn: async () => {
      const res = await api.post('/auth/change-password', {
        currentPassword,
        newPassword
      });
      return res.data;
    },
    onSuccess: () => {
      updateUser({ passwordMustChange: false });
      navigate('/dashboard', { replace: true });
    },
    onError: (err: unknown) => {
      setPasswordError(getAxiosErrorMessage(err, '密码修改失败'));
    }
  });

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    setPasswordError('');
    
    if (newPassword !== confirmPassword) {
      setPasswordError('两次输入的新密码不一致');
      return;
    }
    
    const passwordCheck = validatePassword(newPassword);
    if (!passwordCheck.valid) {
      setPasswordError(passwordCheck.message);
      return;
    }
    
    changePasswordMutation.mutate();
  };

  return (
    <div className="auth-bg relative flex min-h-screen items-center justify-center overflow-hidden px-4">
      <div className="pointer-events-none absolute -left-24 -top-24 h-80 w-80 rounded-full bg-primary/20 blur-3xl" />
      <div className="pointer-events-none absolute -bottom-24 -right-24 h-80 w-80 rounded-full bg-accent/10 blur-3xl" />

      <div className="relative z-10 w-full max-w-md px-4">
        <div className="auth-card p-8">
          <div className="text-center mb-8">
            <div className="w-16 h-16 bg-gradient-to-br from-yellow-500 to-orange-600 rounded-2xl flex items-center justify-center mx-auto mb-4 shadow-lg shadow-yellow-500/30">
              <Lock className="w-8 h-8 text-white" />
            </div>
            <h1 className="mb-2 text-xl font-semibold text-text-primary">需要修改密码</h1>
            <p className="text-[12.5px] text-text-secondary">首次登录，请修改初始密码</p>
          </div>

          <div className="mb-6 flex items-start gap-3 rounded-xl border border-amber-400/25 bg-amber-500/10 p-4">
            <AlertCircle className="mt-0.5 h-4 w-4 shrink-0 text-amber-400" />
            <p className="text-[12.5px] text-amber-300">
              为了您的账户安全，修改密码后才能继续使用
            </p>
          </div>

          <form onSubmit={handleSubmit} className="space-y-5">
            <div>
              <label className="mb-2 block text-[12.5px] font-medium text-text-secondary">
                当前密码
              </label>
              <input
                type="password"
                value={currentPassword}
                onChange={(e) => setCurrentPassword(e.target.value)}
                placeholder="请输入当前密码"
                className="input-base h-11"
                required
              />
            </div>

            <div>
              <label className="mb-2 block text-[12.5px] font-medium text-text-secondary">
                新密码
              </label>
              <input
                type="password"
                value={newPassword}
                onChange={(e) => setNewPassword(e.target.value)}
                placeholder="请输入新密码（至少8位）"
                className="input-base h-11"
                required
              />
              {newPassword && (
                <div className="mt-3 space-y-2">
                  <div className="flex items-center justify-between">
                    <span className="text-xs text-text-tertiary">密码强度</span>
                    <span className={`text-xs font-medium ${getPasswordStrength(newPassword).color}`}>
                      {getPasswordStrength(newPassword).label}
                    </span>
                  </div>
                  <div className="flex gap-1">
                    {[1, 2, 3, 4, 5].map((i) => (
                      <div
                        key={i}
                        className={`h-1.5 flex-1 rounded-full ${
                          i <= getPasswordStrength(newPassword).score
                            ? getPasswordStrength(newPassword).color.replace('text-', 'bg-')
                            : 'bg-surface-3'
                        }`}
                      />
                    ))}
                  </div>
                </div>
              )}
            </div>

            <div>
              <label className="mb-2 block text-[12.5px] font-medium text-text-secondary">
                确认新密码
              </label>
              <input
                type="password"
                value={confirmPassword}
                onChange={(e) => setConfirmPassword(e.target.value)}
                placeholder="请再次输入新密码"
                className="input-base h-11"
                required
              />
              {confirmPassword && newPassword && (
                <div className="mt-2 flex items-center gap-1 text-xs">
                  {newPassword === confirmPassword ? (
                    <>
                      <CheckCircle2 className="w-4 h-4 text-green-400" />
                      <span className="text-green-400">密码匹配</span>
                    </>
                  ) : (
                    <>
                      <AlertCircle className="w-4 h-4 text-red-400" />
                      <span className="text-red-400">密码不匹配</span>
                    </>
                  )}
                </div>
              )}
            </div>

            {passwordError && (
              <div className="flex items-center gap-2 rounded-xl border border-rose-400/25 bg-rose-500/10 p-3 text-[12.5px] text-rose-600">
                <AlertCircle className="w-4 h-4 flex-shrink-0" />
                {passwordError}
              </div>
            )}

            <button
              type="submit"
              disabled={changePasswordMutation.isPending}
              className="btn btn-primary h-11 w-full"
            >
              {changePasswordMutation.isPending ? (
                <>
                  <Loader2 className="w-5 h-5 animate-spin" />
                  修改中...
                </>
              ) : (
                '修改密码并继续'
              )}
            </button>
          </form>
        </div>
      </div>
    </div>
  );
}
