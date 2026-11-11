/* eslint-disable @typescript-eslint/no-explicit-any */
import { useState } from 'react';
import { useSearchParams, useNavigate } from 'react-router-dom';
import { Shield as _Shield, Lock, AlertCircle, CheckCircle2, Loader2, Globe } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { useAuth } from '../../../../contexts/AuthContext';
import { validatePassword, getPasswordStrength } from '../../../../utils/passwordValidator';
import api from '../../../../lib/api';
import { getAxiosErrorMessage } from '@/lib/errorHandler';

export default function SecuritySettings() {
  const { t: _t } = useTranslation();
  const [searchParams] = useSearchParams();
  const { user, updateUser } = useAuth();
  const navigate = useNavigate();

  const [currentPassword, setCurrentPassword] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [passwordStatus, setPasswordStatus] = useState<'idle' | 'saving' | 'saved' | 'error'>('idle');
  const [passwordError, setPasswordError] = useState('');

  const handlePasswordChange = async () => {
    setPasswordError('');
    setPasswordStatus('saving');

    if (newPassword !== confirmPassword) {
      setPasswordError('两次输入的新密码不一致');
      setPasswordStatus('error');
      setTimeout(() => setPasswordStatus('idle'), 3000);
      return;
    }

    const passwordCheck = validatePassword(newPassword);
    if (!passwordCheck.valid) {
      setPasswordError(passwordCheck.message);
      setPasswordStatus('error');
      setTimeout(() => setPasswordStatus('idle'), 3000);
      return;
    }

    try {
      const response = await api.post('/auth/change-password', {
        currentPassword,
        newPassword
      });

      if (response.data.success) {
        setPasswordStatus('saved');
        setCurrentPassword('');
        setNewPassword('');
        setConfirmPassword('');

        if (user) {
          const updatedUser = { ...user, passwordMustChange: false };
          updateUser(updatedUser);
        }

        navigate('/settings', { replace: true });

        setTimeout(() => setPasswordStatus('idle'), 3000);
      } else {
        setPasswordError(response.data.error || response.data.message || '密码修改失败');
        setPasswordStatus('error');
        setTimeout(() => setPasswordStatus('idle'), 3000);
      }
    } catch (err: unknown) {
      setPasswordError(getAxiosErrorMessage(err, '密码修改失败'));
      setPasswordStatus('error');
      setTimeout(() => setPasswordStatus('idle'), 3000);
    }
  };

  return (
    <div className="space-y-6">
      {/* 修改密码 */}
      <div className="bg-surface border border-border rounded-xl p-6 shadow-sm">
        <h4 className="font-semibold text-text-primary mb-4 flex items-center gap-2">
          <Lock className="w-5 h-5 text-blue-600" />
          修改密码
        </h4>
        {searchParams.get('changePassword') === 'true' && (
          <div className="mb-4 p-3 bg-amber-500/10 border border-amber-500/30 rounded-xl flex items-start gap-2 text-amber-600">
            <AlertCircle className="w-5 h-5 flex-shrink-0 mt-0.5" />
            <div>
              <p className="text-sm font-medium">首次登录需要修改密码</p>
              <p className="text-xs mt-1 text-amber-600/80">为了您的账户安全，请修改默认初始密码后继续使用系统</p>
            </div>
          </div>
        )}
        <div className="space-y-4 max-w-md">
          <div>
            <label className="block text-sm font-medium text-text-secondary mb-2">当前密码</label>
            <input
              type="password"
              value={currentPassword}
              onChange={(e) => setCurrentPassword(e.target.value)}
              placeholder="请输入当前密码"
              className="w-full px-4 py-2.5 bg-background border border-border rounded-xl text-text-primary placeholder-slate-500 focus:outline-none focus:border-blue-500/50 focus:ring-2 focus:ring-blue-500/20 transition-all"
            />
          </div>
          <div>
            <label className="block text-sm font-medium text-text-secondary mb-2">新密码</label>
            <input
              type="password"
              value={newPassword}
              onChange={(e) => setNewPassword(e.target.value)}
              placeholder="请输入新密码（至少8位）"
              className="w-full px-4 py-2.5 bg-background border border-border rounded-xl text-text-primary placeholder-slate-500 focus:outline-none focus:border-blue-500/50 focus:ring-2 focus:ring-blue-500/20 transition-all"
            />
            {newPassword && (
              <div className="mt-2 space-y-2">
                <div className="flex items-center justify-between">
                  <span className="text-xs text-text-secondary">密码强度</span>
                  <span className={`text-xs font-semibold ${getPasswordStrength(newPassword).color}`}>
                    {getPasswordStrength(newPassword).label}
                  </span>
                </div>
                <div className="flex gap-1">
                  {[1, 2, 3, 4, 5].map((i) => (
                    <div
                      key={i}
                      className={`h-1.5 flex-1 rounded-full transition-all ${
                        i <= getPasswordStrength(newPassword).score
                          ? getPasswordStrength(newPassword).color.replace('text-', 'bg-')
                          : 'bg-surface-3'
                      }`}
                    />
                  ))}
                </div>
                <div className="grid grid-cols-2 gap-1.5 text-xs pt-1">
                  {Object.entries(validatePassword(newPassword).details).map(([key, value]) => (
                    <div key={key} className={`flex items-center gap-1.5 ${value ? 'text-emerald-600' : 'text-text-tertiary'}`}>
                      {value ? <CheckCircle2 className="w-3.5 h-3.5" /> : <div className="w-3.5 h-3.5 rounded-full border border-border" />}
                      <span>
                        {key === 'minLength' && '至少8位'}
                        {key === 'uppercase' && '包含大写字母'}
                        {key === 'lowercase' && '包含小写字母'}
                        {key === 'number' && '包含数字'}
                        {key === 'special' && '包含特殊字符'}
                      </span>
                    </div>
                  ))}
                </div>
              </div>
            )}
          </div>
          <div>
            <label className="block text-sm font-medium text-text-secondary mb-2">确认新密码</label>
            <input
              type="password"
              value={confirmPassword}
              onChange={(e) => setConfirmPassword(e.target.value)}
              placeholder="请再次输入新密码"
              className="w-full px-4 py-2.5 bg-background border border-border rounded-xl text-text-primary placeholder-slate-500 focus:outline-none focus:border-blue-500/50 focus:ring-2 focus:ring-blue-500/20 transition-all"
            />
            {confirmPassword && newPassword && (
              <div className="mt-1.5 flex items-center gap-1.5 text-xs">
                {newPassword === confirmPassword ? (
                  <>
                    <CheckCircle2 className="w-3.5 h-3.5 text-emerald-600" />
                    <span className="text-emerald-600 font-medium">密码一致</span>
                  </>
                ) : (
                  <>
                    <AlertCircle className="w-3.5 h-3.5 text-red-600" />
                    <span className="text-red-600 font-medium">两次输入的密码不一致</span>
                  </>
                )}
              </div>
            )}
          </div>
          {passwordError && (
            <p className="text-sm text-red-600 flex items-center gap-1.5 bg-red-500/10 p-2.5 rounded-xl border border-red-500/20">
              <AlertCircle className="w-4 h-4 flex-shrink-0" />
              {passwordError}
            </p>
          )}
          <div className="flex items-center gap-3 pt-2">
            <button
              onClick={handlePasswordChange}
              disabled={passwordStatus === 'saving'}
              className="px-5 py-2.5 bg-blue-600 hover:bg-blue-500 text-white font-medium rounded-xl transition-all shadow-lg shadow-blue-500/20 disabled:opacity-50 disabled:cursor-not-allowed flex items-center gap-2"
            >
              {passwordStatus === 'saving' && (
                <Loader2 className="w-4 h-4 animate-spin" />
              )}
              修改密码
            </button>
            {passwordStatus === 'saved' && (
              <p className="text-sm text-emerald-600 flex items-center gap-1.5 font-medium">
                <CheckCircle2 className="w-4 h-4" />
                密码修改成功
              </p>
            )}
          </div>
        </div>
      </div>

      {/* CORS 跨域配置 */}
      <div className="bg-surface border border-border rounded-xl p-6 shadow-sm">
        <h4 className="font-semibold text-text-primary mb-2 flex items-center gap-2">
          <Globe className="w-5 h-5 text-indigo-600" />
          CORS 跨域安全配置
        </h4>
        <p className="text-sm text-text-secondary mb-4">
          允许访问本平台的前端跨域域名或 IP 白名单（逗号分隔）
        </p>
        <div className="max-w-md">
          <input
            type="text"
            defaultValue="http://localhost:3000, http://localhost:8088"
            className="w-full px-4 py-2.5 bg-background border border-border rounded-xl text-text-primary focus:outline-none focus:border-blue-500/50 focus:ring-2 focus:ring-blue-500/20 transition-all font-mono text-sm"
          />
        </div>
      </div>
    </div>
  );
}
