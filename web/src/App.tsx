import { lazy, Suspense, useEffect, useMemo, useRef, useState, useTransition } from 'react'
import { App as AntApp, Avatar, Button, Form, Input, Layout, Modal, Popover, Select, Skeleton, Space, Typography } from 'antd'
import { AppstoreOutlined, BgColorsOutlined, BranchesOutlined, FolderOpenOutlined, GithubOutlined, HomeOutlined, LogoutOutlined, PlusOutlined, SettingOutlined, ThunderboltOutlined, ToolOutlined } from '@ant-design/icons'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from './api/client'
import type { Episode, Principal, Project } from './api/types'
import { LoginScreen } from './features/auth/LoginScreen'
import { EmptyState } from './components/EmptyState'
import { PageScrollOrb } from './components/PageScrollOrb'
import { HomePage } from './features/home/HomePage'

type ModuleKey = 'home' | 'episodes' | 'assets' | 'generation' | 'canvas' | 'tools' | 'models' | 'settings'
type LazyModuleKey = Exclude<ModuleKey, 'home'>
const moduleLoaders = {
  episodes: () => import('./features/episodes/EpisodesPage'),
  assets: () => import('./features/assets/AssetLibraryPage'),
  generation: () => import('./features/generation/GenerationStudioPage'),
  canvas: () => import('./features/canvas/CanvasComposer'),
  tools: () => import('./features/tools/LocalToolsPage'),
  models: () => import('./features/models/ModelHubPage'),
  settings: () => import('./features/settings/SettingsPage'),
} satisfies Record<LazyModuleKey, () => Promise<unknown>>
const EpisodesPage = lazy(() => moduleLoaders.episodes().then((module) => ({ default: module.EpisodesPage })))
const AssetLibraryPage = lazy(() => moduleLoaders.assets().then((module) => ({ default: module.AssetLibraryPage })))
const CanvasComposer = lazy(() => moduleLoaders.canvas().then((module) => ({ default: module.CanvasComposer })))
const ModelHubPage = lazy(() => moduleLoaders.models().then((module) => ({ default: module.ModelHubPage })))
const SettingsPage = lazy(() => moduleLoaders.settings().then((module) => ({ default: module.SettingsPage })))
const GenerationStudioPage = lazy(() => moduleLoaders.generation().then((module) => ({ default: module.GenerationStudioPage })))
const LocalToolsPage = lazy(() => moduleLoaders.tools().then((module) => ({ default: module.LocalToolsPage })))

function preloadModule(key: ModuleKey) {
  if (key !== 'home') void moduleLoaders[key]().catch(() => undefined)
}

const moduleMeta: Record<ModuleKey, { label: string; icon: React.ReactNode }> = {
  home: { label: '主页', icon: <HomeOutlined/> }, episodes: { label: '剧本', icon: <FolderOpenOutlined/> }, assets: { label: '资产', icon: <AppstoreOutlined/> }, generation: { label: '生成', icon: <ThunderboltOutlined/> }, canvas: { label: '画布', icon: <BranchesOutlined/> }, tools: { label: '工具', icon: <ToolOutlined/> }, models: { label: '模型', icon: <BgColorsOutlined/> }, settings: { label: '设置', icon: <SettingOutlined/> },
}

export default function App() {
  const { message } = AntApp.useApp(); const queryClient = useQueryClient()
  const auth = useQuery({ queryKey: ['auth'], queryFn: api.authStatus, staleTime: 0 })
	const resetUserQueries = async () => {
		await queryClient.cancelQueries()
		queryClient.removeQueries({ predicate: (query) => query.queryKey[0] !== 'auth' })
	}
	const login = useMutation({ mutationFn: ({ username, password }: { username: string; password: string }) => api.login(username, password), onSuccess: async () => { await resetUserQueries(); await queryClient.invalidateQueries({ queryKey: ['auth'] }) } })
	const setup = useMutation({ mutationFn: ({ username, displayName, password }: { username: string; displayName: string; password: string }) => api.setupAccount(username, displayName, password), onSuccess: async () => { await queryClient.invalidateQueries({ queryKey: ['auth'] }) } })
  if (auth.isLoading) return <div className="boot-screen"><img alt="SagaFlow" className="brand-mark-image large" src="/logo-mark.png"/><Skeleton active paragraph={{ rows: 2 }} style={{ width: 280 }}/></div>
  if (auth.error) return <div className="boot-screen"><Typography.Title level={3}>无法连接 SagaFlow API</Typography.Title><Typography.Text type="danger">{auth.error.message}</Typography.Text></div>
	if (auth.data?.enabled && !auth.data.authenticated) return <LoginScreen error={(login.error ?? setup.error)?.message} loading={login.isPending || setup.isPending} onLogin={(username, password) => login.mutate({ username, password })} onSetup={(username, displayName, password) => setup.mutate({ username, displayName, password })} status={auth.data}/>
	return <Workspace key={auth.data?.user?.account_id} onError={(error) => message.error(error instanceof Error ? error.message : '操作失败')} sessionUser={auth.data!.user!}/>
}

function Workspace({ onError, sessionUser }: { onError: (error: unknown) => void; sessionUser: Principal }) {
  const queryClient = useQueryClient(); const { message } = AntApp.useApp(); const [form] = Form.useForm();
  const [active, setActive] = useState<ModuleKey>('home'); const [projectModal, setProjectModal] = useState(false)
  const [isSwitchingModule, startModuleTransition] = useTransition()
  const contentRef = useRef<HTMLElement | null>(null)
	const selectModule = (key: ModuleKey) => {
		preloadModule(key)
		startModuleTransition(() => setActive(key))
	}
	const projectStorageKey = `sagaflow.${sessionUser.account_id}.project`
	const episodeStorageKey = `sagaflow.${sessionUser.account_id}.episode`
  const [projectID, setProjectID] = useState(() => localStorage.getItem(projectStorageKey) ?? '')
	const [episodeID, setEpisodeID] = useState(() => localStorage.getItem(episodeStorageKey) ?? '')
	const meQuery = useQuery({ queryKey: ['me'], queryFn: api.me })
	const currentUser = meQuery.data?.user ?? sessionUser
	const userInitial = (currentUser?.display_name || currentUser?.username || 'S').slice(0, 1).toUpperCase()
  const projectsQuery = useQuery({ queryKey: ['projects'], queryFn: api.projects })
  const projects = useMemo(() => projectsQuery.data ?? [], [projectsQuery.data]); const project = projects.find((item) => item.id === projectID) ?? null
  const episodesQuery = useQuery({ queryKey: ['episodes', projectID], queryFn: () => api.episodes(projectID), enabled: Boolean(project) })
  const episodes = useMemo(() => episodesQuery.data ?? [], [episodesQuery.data]); const episode = episodes.find((item) => item.id === episodeID) ?? null
  useEffect(() => { if (projects.length && !project) setProjectID(projects[0].id); if (!projects.length) setProjectID('') }, [projects, project])
  useEffect(() => { localStorage.setItem(projectStorageKey, projectID); setEpisodeID((current) => episodes.some((item) => item.id === current) ? current : (episodes[0]?.id ?? '')) }, [episodes, projectID, projectStorageKey])
	useEffect(() => { localStorage.setItem(episodeStorageKey, episodeID) }, [episodeID, episodeStorageKey])
  useEffect(() => { contentRef.current?.scrollTo({ top: 0 }) }, [active, projectID])
	useEffect(() => {
		const commonTimer = window.setTimeout(() => (['episodes', 'assets', 'generation', 'tools', 'settings'] as ModuleKey[]).forEach(preloadModule), 0)
		const remainingTimer = window.setTimeout(() => (['canvas', 'models'] as ModuleKey[]).forEach(preloadModule), 800)
		return () => { window.clearTimeout(commonTimer); window.clearTimeout(remainingTimer) }
	}, [])
  const createProject = useMutation({ mutationFn: api.createProject, onSuccess: async (created) => { await queryClient.invalidateQueries({ queryKey: ['projects'] }); setProjectID(created.id); selectModule('episodes'); setProjectModal(false); form.resetFields(); message.success('项目已创建，开始编写第一集吧') }, onError })
  const logout = useMutation({ mutationFn: api.logout, onSuccess: async () => { await queryClient.cancelQueries(); queryClient.removeQueries({ predicate: (query) => query.queryKey[0] !== 'auth' }); await queryClient.invalidateQueries({ queryKey: ['auth'] }) }, onError })
	const projectOptions = useMemo(() => projects.map((item) => ({ label: item.title, value: item.id })), [projects])
  const selectEpisode = (value: string) => setEpisodeID(value)
  const openProject = (id: string) => { setProjectID(id); selectModule('episodes') }
  return <Layout className="app-layout">
    <nav aria-label="主菜单" className="floating-nav">
      {(Object.keys(moduleMeta) as ModuleKey[]).map((key) => <button aria-current={active === key ? 'page' : undefined} className={`floating-nav-button${active === key ? ' active' : ''}`} key={key} onClick={() => selectModule(key)} onFocus={() => preloadModule(key)} onPointerEnter={() => preloadModule(key)} type="button"><span className="floating-nav-icon">{moduleMeta[key].icon}</span><span>{moduleMeta[key].label}</span></button>)}
      <a className="floating-nav-button floating-nav-open-source" href="https://github.com/gofurry/sagaflow" rel="noreferrer" target="_blank"><span className="floating-nav-icon"><GithubOutlined/></span><span>开源</span></a>
    </nav>
    <Layout>
      <Layout.Header className="app-header">
        <div className="header-left">
          <button aria-label="返回主页" className="header-brand-logo" onClick={() => selectModule('home')} type="button"><img alt="SagaFlow" src="/logo-header.png"/></button>
          <div className="header-title"><strong>{moduleMeta[active].label}</strong></div>
          {active !== 'home' && <div className="header-project">
            <Select className="project-select" options={projectOptions} placeholder="选择项目" value={projectID || undefined} onChange={setProjectID} popupMatchSelectWidth={300}/>
            <Button className="header-create" icon={<PlusOutlined/>} onClick={() => setProjectModal(true)}>新建项目</Button>
            {project && <Typography.Text type="secondary" className="project-description">{project.description || '暂无项目说明'}</Typography.Text>}
          </div>}
        </div>
        <Space className="header-actions">
          {active !== 'home' && episodes.length > 0 && (
            <Select className="episode-select" options={episodes.map((item) => ({ label: `EP ${String(item.episode_number).padStart(2, '0')} · ${item.title}`, value: item.id }))} value={episodeID || undefined} onChange={selectEpisode}/>
          )}
          <Popover
            arrow={false}
            content={<div className="user-status-card">
			  <div className="user-status-profile"><div><strong>{currentUser?.display_name || currentUser?.username || '当前用户'}</strong><span>本地账号</span></div></div>
			  <div className="user-session-state">项目与素材默认保存在本机</div>
              <div className="user-status-actions">
                <button onClick={() => selectModule('settings')} onPointerEnter={() => preloadModule('settings')} type="button"><SettingOutlined/><span>设置</span></button>
                <button disabled={logout.isPending} onClick={() => logout.mutate()} type="button"><LogoutOutlined/><span>{logout.isPending ? '正在退出…' : '退出登录'}</span></button>
              </div>
            </div>}
            placement="bottomRight"
            trigger={['hover', 'click']}
          >
			<button aria-label="打开用户菜单" className="user-orb-button" type="button"><Avatar className="user-avatar">{userInitial}</Avatar></button>
          </Popover>
        </Space>
      </Layout.Header>
      <Layout.Content aria-busy={isSwitchingModule} className={`app-content module-${active}`} ref={contentRef}>
        <Suspense fallback={<div className="module-loading"><Skeleton active paragraph={{ rows: 5 }}/></div>}>
          {active === 'home'
            ? <HomePage activeProjectID={projectID} loading={projectsQuery.isLoading} onCreate={() => setProjectModal(true)} onOpenProject={openProject} projects={projects}/>
            : active === 'settings'
            ? <SettingsPage onError={onError} project={project ?? undefined}/>
            : active === 'models'
            ? <ModelHubPage onError={onError}/>
            : !project
            ? <EmptyState actionLabel="创建第一个项目" description="项目用于组织剧情分集、资产和生成画布。" onAction={() => setProjectModal(true)} title="开始一部新漫剧"/>
            : <ModuleContent active={active} episode={episode} episodes={episodes} onEpisodeChange={setEpisodeID} onError={onError} project={project}/>
          }
        </Suspense>
      </Layout.Content>
      <PageScrollOrb refreshKey={`${active}:${projectID}`} scrollerRef={contentRef}/>
    </Layout>
    <Modal forceRender title="新建漫剧项目" open={projectModal} onCancel={() => setProjectModal(false)} onOk={() => form.submit()} confirmLoading={createProject.isPending} okText="创建">
		<Form form={form} layout="vertical" onFinish={(values) => createProject.mutate(values)} requiredMark={false}><Form.Item label="项目名称" name="title" rules={[{ required: true }]}><Input autoFocus placeholder="例如：深夜办公室"/></Form.Item><Form.Item label="项目说明" name="description"><Input.TextArea rows={3} placeholder="一句话描述故事方向或制作目标"/></Form.Item></Form>
    </Modal>
  </Layout>
}

function ModuleContent({ active, project, episode, episodes, onEpisodeChange, onError }: { active: Exclude<ModuleKey, 'home' | 'settings'>; project: Project; episode: Episode | null; episodes: Episode[]; onEpisodeChange: (id: string) => void; onError: (error: unknown) => void }) {
  if (active === 'episodes') return <EpisodesPage episodes={episodes} onEpisodeChange={onEpisodeChange} onError={onError} project={project}/>
  if (active === 'assets') return <AssetLibraryPage episode={episode} onError={onError} project={project}/>
  if (active === 'generation') return <GenerationStudioPage episode={episode} onError={onError} project={project}/>
  if (active === 'canvas') return episode ? <CanvasComposer episode={episode} onError={onError} project={project}/> : <EmptyState description="Composer 需要关联一个分集。" title="请先创建分集"/>
  if (active === 'tools') return <LocalToolsPage onError={onError} project={project}/>
  return <ModelHubPage onError={onError}/>
}
