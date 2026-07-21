import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { App as AntApp, ConfigProvider, theme } from 'antd'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import zhCN from 'antd/locale/zh_CN'
import App from './App'
import '@xyflow/react/dist/style.css'
import './styles.css'

const queryClient = new QueryClient({ defaultOptions: { queries: { staleTime: 20_000, retry: 1, refetchOnWindowFocus: false }, mutations: { retry: 0 } } })
createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <ConfigProvider locale={zhCN} theme={{ algorithm: theme.defaultAlgorithm, token: { colorPrimary: '#c8753f', colorInfo: '#c8753f', colorSuccess: '#84945f', colorText: '#4e3c32', colorTextSecondary: '#8d7b6f', colorBgLayout: '#f4efe6', colorBgContainer: '#fffaf2', colorFillAlter: '#f6efe6', colorBorderSecondary: '#e4d8ca', borderRadius: 12, borderRadiusLG: 16, fontFamily: 'Inter, "PingFang SC", "Microsoft YaHei", sans-serif' }, components: { Layout: { headerBg: 'rgba(250, 246, 239, .9)', bodyBg: '#f4efe6' }, Button: { primaryShadow: '0 8px 22px rgba(181, 96, 44, .18)' }, Card: { headerBg: 'transparent' } } }}>
      <AntApp><QueryClientProvider client={queryClient}><App /></QueryClientProvider></AntApp>
    </ConfigProvider>
  </StrictMode>,
)
