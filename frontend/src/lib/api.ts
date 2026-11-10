import axios from 'axios';

interface TokenRefreshResult {
  token: string;
  refreshToken: string;
}

let isRefreshing = false;
let failedQueue: Array<{
  resolve: (value: TokenRefreshResult | PromiseLike<TokenRefreshResult>) => void;
  reject: (reason?: unknown) => void;
}> = [];

const processQueue = (error: Error | null, result: TokenRefreshResult | null = null) => {
  failedQueue.forEach((prom) => {
    if (error) {
      prom.reject(error);
    } else if (result) {
      prom.resolve(result);
    }
  });
  failedQueue = [];
};

const api = axios.create({
  baseURL: '/api/v1',
  timeout: 120000,
  headers: {
    'Content-Type': 'application/json',
  },
});

const refreshApi = axios.create({
  baseURL: '/api/v1',
  timeout: 30000,
  headers: {
    'Content-Type': 'application/json',
  },
});

api.interceptors.request.use(
  (config) => {
    const token = localStorage.getItem('token');
    if (token) {
      config.headers.Authorization = `Bearer ${token}`;
    }
    return config;
  },
  (error) => {
    return Promise.reject(error);
  }
);

api.interceptors.response.use(
  (response) => {
    // 保持原始响应结构，由各组件自行解包 response.data.data
    return response;
  },
  async (error) => {
    const originalRequest = error.config;

    if (error.response?.status === 401 && !originalRequest._retry) {
      if (originalRequest.url?.includes('/auth/login')) {
        // 登录请求本身 401 = 账号密码错误，不清 localStorage，不跳转，直接抛出让 Login 组件处理
        return Promise.reject(error);
      }

      if (originalRequest.url?.includes('/auth/refresh')) {
        localStorage.removeItem('token');
        localStorage.removeItem('user');
        localStorage.removeItem('refreshToken');
        window.location.href = '/login';
        return Promise.reject(error);
      }

      if (isRefreshing) {
        return new Promise<TokenRefreshResult>((resolve, reject) => {
          failedQueue.push({ resolve, reject });
        })
          .then((result) => {
            originalRequest.headers.Authorization = `Bearer ${result.token}`;
            return api(originalRequest);
          })
          .catch((err) => {
            return Promise.reject(err);
          });
      }

      originalRequest._retry = true;
      isRefreshing = true;

      const refreshToken = localStorage.getItem('refreshToken');

      if (!refreshToken) {
        localStorage.removeItem('token');
        localStorage.removeItem('user');
        window.location.href = '/login';
        return Promise.reject(error);
      }

      try {
        const { data } = await refreshApi.post('/auth/refresh', { refreshToken });

        if (data.success) {
          const { token: newToken, refreshToken: newRefreshToken } = data.data;
          localStorage.setItem('token', newToken);
          localStorage.setItem('refreshToken', newRefreshToken);

          processQueue(null, { token: newToken, refreshToken: newRefreshToken });
          originalRequest.headers.Authorization = `Bearer ${newToken}`;
          return api(originalRequest);
        } else {
          throw new Error('Token refresh failed');
        }
      } catch (refreshError) {
        processQueue(refreshError as Error, null);
        localStorage.removeItem('token');
        localStorage.removeItem('user');
        localStorage.removeItem('refreshToken');
        
        window.location.href = '/login';
        return Promise.reject(refreshError);
      } finally {
        isRefreshing = false;
      }
    }

    const message = error.response?.data?.message || error.response?.data?.error || '网络错误';
    return Promise.reject(new Error(message));
  }
);

export default api;
