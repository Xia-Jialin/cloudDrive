import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

// 从环境变量获取API地址，默认使用Kubernetes部署的API服务
// 使用Ingress地址
const apiBaseUrl = process.env.VITE_API_BASE_URL || 'http://192.168.194.191';

// 从环境变量获取Host头，默认为clouddrive.local
const apiHost = process.env.VITE_API_HOST || 'clouddrive.local';

export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      '/api': {
        target: apiBaseUrl,
        changeOrigin: true,
        headers: {
          Host: apiHost,
        },
      },
    },
    appType: 'spa',
  },
}); 