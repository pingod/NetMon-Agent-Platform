import { useNavigate } from 'react-router-dom';
import { AlertTriangle, Home, ArrowLeft } from 'lucide-react';

export default function NotFound() {
  const navigate = useNavigate();

  return (
    <div className="min-h-screen bg-gradient-to-br from-primary-soft via-surface to-surface flex items-center justify-center p-6">
      <div className="text-center max-w-lg">
        <div className="relative mb-8">
          <div className="w-32 h-32 mx-auto rounded-full bg-gradient-to-br from-red-500/20 to-yellow-500/20 flex items-center justify-center border border-red-500/30">
            <AlertTriangle className="w-16 h-16 text-yellow-600" />
          </div>
          <div className="absolute -bottom-2 left-1/2 -translate-x-1/2 px-4 py-1 bg-surface-2 rounded-full border border-border">
            <span className="text-3xl font-bold text-text-primary">404</span>
          </div>
        </div>

        <h1 className="text-2xl font-bold text-text-primary mb-2">页面未找到</h1>
        <p className="text-text-secondary mb-8">您访问的页面不存在或已被移除</p>

        <div className="flex items-center justify-center gap-4">
          <button
            onClick={() => navigate(-1)}
            className="flex items-center gap-2 px-6 py-3 bg-surface-2 hover:bg-surface-hover text-text-primary rounded-xl border border-border transition-all"
          >
            <ArrowLeft className="w-4 h-4" />
            返回上一页
          </button>
          <button
            onClick={() => navigate('/')}
            className="flex items-center gap-2 px-6 py-3 bg-blue-600 hover:bg-blue-700 text-white rounded-xl shadow-lg shadow-blue-600/30 transition-all"
          >
            <Home className="w-4 h-4" />
            返回首页
          </button>
        </div>
      </div>
    </div>
  );
}
