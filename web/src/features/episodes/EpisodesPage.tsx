import { useEffect, useMemo, useRef, useState, type KeyboardEvent, type MouseEvent } from 'react'
import { ArrowDownOutlined, ArrowUpOutlined, DeleteOutlined, EditOutlined, HistoryOutlined, InfoCircleOutlined, PlusOutlined, ReloadOutlined } from '@ant-design/icons'
import { App, Button, Empty, Form, Input, Modal, Popconfirm, Skeleton, Space, Tag, Tooltip, Typography } from 'antd'
import { useIsFetching, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../../api/client'
import type { Episode, Project } from '../../api/types'
import { FloatingToolbar } from '../../components/FloatingToolbar'
import { MarkdownEditor, MarkdownPreview } from '../../components/Markdown'

type EpisodeView = 'story' | 'edit' | 'history' | 'details'

interface Props {
  project: Project
  episodes: Episode[]
  onEpisodeChange: (id: string) => void
  onError: (error: unknown) => void
}

export function EpisodesPage({ project, episodes, onEpisodeChange, onError }: Props) {
  const queryClient = useQueryClient()
  const { message } = App.useApp()
  const [createForm] = Form.useForm()
  const [createOpen, setCreateOpen] = useState(false)
  const [openEpisodeID, setOpenEpisodeID] = useState<string | null>(null)
  const [episodeViews, setEpisodeViews] = useState<Record<string, EpisodeView>>({})
  const refreshing = useIsFetching({ queryKey: ['episodes', project.id] }) > 0
  const closeCreate = () => {
    setCreateOpen(false)
    createForm.resetFields()
  }

  const refreshEpisodes = () => queryClient.invalidateQueries({ queryKey: ['episodes', project.id] })
  const refreshAll = async () => {
    await Promise.all([refreshEpisodes(), queryClient.invalidateQueries({ queryKey: ['scripts'] })])
    message.success('剧本已刷新')
  }
  const create = useMutation({
    mutationFn: (values: { title: string; script_body?: string }) => api.createEpisode(project.id, values),
    onSuccess: async (item) => {
      await refreshEpisodes()
      setCreateOpen(false)
      createForm.resetFields()
      onEpisodeChange(item.id)
      setEpisodeViews((current) => ({ ...current, [item.id]: 'story' }))
      setOpenEpisodeID(item.id)
      message.success('分集已创建')
    },
    onError,
  })
  const remove = useMutation({
    mutationFn: (id: string) => api.deleteEpisode(id),
    onSuccess: async (_, id) => {
      setOpenEpisodeID((current) => current === id ? null : current)
      setEpisodeViews((current) => {
        const next = { ...current }
        delete next[id]
        return next
      })
      await refreshEpisodes()
      message.success('分集已删除')
    },
    onError,
  })
  const reorder = useMutation({
    mutationFn: ({ episode, target }: { episode: Episode; target: Episode }) => api.updateEpisode(episode.id, {
      episode_number: target.episode_number,
      title: episode.title,
      notes: episode.notes,
      target_duration_seconds: episode.target_duration_seconds,
      target_shot_count: episode.target_shot_count,
    }),
    onSuccess: async () => {
      await refreshEpisodes()
      message.success('分集顺序已调整')
    },
    onError,
  })
  const openEpisode = (episode: Episode, view: EpisodeView) => {
    onEpisodeChange(episode.id)
    const shouldClose = openEpisodeID === episode.id && (episodeViews[episode.id] ?? 'story') === view
    setEpisodeViews((current) => ({ ...current, [episode.id]: view }))
    setOpenEpisodeID(shouldClose ? null : episode.id)
  }

  return <div className="page page-episodes">
    <FloatingToolbar ariaLabel="剧本工具栏" items={[
      { key: 'refresh', label: '刷新', icon: <ReloadOutlined/>, loading: refreshing, onClick: refreshAll },
      { key: 'create', label: '新建分集', icon: <PlusOutlined/>, onClick: () => setCreateOpen(true) },
    ]}/>

    <section className="script-page-content">
      <header className="script-project-header">
        <div className="script-project-title-row"><Typography.Title level={2}>{project.title}</Typography.Title><span className="script-project-count">{episodes.length} 集</span></div>
        <Typography.Paragraph>{project.description || '暂无项目描述'}</Typography.Paragraph>
      </header>

      {episodes.length === 0
        ? <Empty className="script-empty" image={Empty.PRESENTED_IMAGE_SIMPLE} description="还没有分集"><Button icon={<PlusOutlined/>} onClick={() => setCreateOpen(true)} type="primary">新建第一个分集</Button></Empty>
        : <div className="script-episode-list">{episodes.map((episode, index) => {
          const isExpanded = openEpisodeID === episode.id
          const view = episodeViews[episode.id] ?? 'story'
          const previousEpisode = episodes[index - 1]
          const nextEpisode = episodes[index + 1]
          return <article className={`script-episode${isExpanded ? ' expanded' : ''}`} key={episode.id}>
            <div aria-expanded={isExpanded} className="script-episode-row" onClick={() => openEpisode(episode, 'story')} onKeyDown={(event) => openEpisodeFromKeyboard(event, () => openEpisode(episode, 'story'))} role="button" tabIndex={0}>
              <div className="script-episode-name"><span>{String(episode.episode_number).padStart(2, '0')}</span><strong>{episode.title}</strong></div>
              <div className="script-episode-actions" onClick={(event) => event.stopPropagation()}>
                <EpisodeAction disabled={!previousEpisode || reorder.isPending} icon={<ArrowUpOutlined/>} label="上移" onClick={() => previousEpisode && reorder.mutate({ episode, target: previousEpisode })}/>
                <EpisodeAction disabled={!nextEpisode || reorder.isPending} icon={<ArrowDownOutlined/>} label="下移" onClick={() => nextEpisode && reorder.mutate({ episode, target: nextEpisode })}/>
                <EpisodeAction icon={<EditOutlined/>} label="编辑" onClick={() => openEpisode(episode, 'edit')}/>
                <EpisodeDeleteAction loading={remove.isPending} onConfirm={() => remove.mutate(episode.id)}/>
                <EpisodeAction icon={<HistoryOutlined/>} label="查看版本" onClick={() => openEpisode(episode, 'history')}/>
                <EpisodeAction icon={<InfoCircleOutlined/>} label="查看详情" onClick={() => openEpisode(episode, 'details')}/>
              </div>
            </div>
            <div aria-hidden={!isExpanded} className={`episode-expand-region${isExpanded ? ' open' : ''}`} inert={!isExpanded}>
              <div className="episode-expand-inner"><EpisodeExpandedContent active={isExpanded} episode={episode} onError={onError} onRefreshEpisodes={refreshEpisodes} view={view}/></div>
            </div>
          </article>
        })}</div>}
    </section>

    <Modal title="新建分集" open={createOpen} onCancel={closeCreate} onOk={() => createForm.submit()} confirmLoading={create.isPending} okText="创建" width={860}>
      <Form form={createForm} layout="vertical" onFinish={(values) => create.mutate(values)} requiredMark={false} initialValues={{ title: '第 x 集' }}>
        <Form.Item label="标题" name="title" rules={[{ required: true }]}><Input autoFocus/></Form.Item>
        <Form.Item label="剧情 Prompt" name="script_body"><MarkdownEditor height={360} placeholder="使用 Markdown 编写剧情 Prompt，也可以稍后继续编辑。"/></Form.Item>
      </Form>
    </Modal>
  </div>
}

function EpisodeAction({ icon, label, onClick, disabled = false }: { icon: React.ReactNode; label: string; onClick?: () => void; disabled?: boolean }) {
  const handleClick = (event: MouseEvent<HTMLButtonElement>) => {
    event.stopPropagation()
    onClick?.()
  }
  return <Tooltip title={label}><button aria-label={label} className="episode-action" disabled={disabled} onClick={handleClick} type="button">{icon}</button></Tooltip>
}

function EpisodeDeleteAction({ loading, onConfirm }: { loading: boolean; onConfirm: () => void }) {
  return <Popconfirm cancelText="取消" description="删除后，该分集的剧情版本和画布也会一并删除。" okButtonProps={{ danger: true, loading }} okText="确认删除" onConfirm={onConfirm} title="确认删除这个分集？">
    <button aria-label="删除" className="episode-action danger" onClick={(event) => event.stopPropagation()} title="删除" type="button"><DeleteOutlined/></button>
  </Popconfirm>
}

function openEpisodeFromKeyboard(event: KeyboardEvent<HTMLDivElement>, open: () => void) {
  if (event.target !== event.currentTarget || (event.key !== 'Enter' && event.key !== ' ')) return
  event.preventDefault()
  open()
}

function EpisodeExpandedContent({ active, episode, view, onError, onRefreshEpisodes }: { active: boolean; episode: Episode; view: EpisodeView; onError: (error: unknown) => void; onRefreshEpisodes: () => Promise<unknown> }) {
  const queryClient = useQueryClient()
  const { message } = App.useApp()
  const [script, setScript] = useState('')
  const [saveOpen, setSaveOpen] = useState(false)
  const [revisionNote, setRevisionNote] = useState('')
  const scriptsQuery = useQuery({ queryKey: ['scripts', episode.id], queryFn: () => api.scripts(episode.id), enabled: active })
  const scripts = useMemo(() => scriptsQuery.data ?? [], [scriptsQuery.data])
  const adopted = useMemo(() => scripts.find((item) => item.status === 'adopted') ?? scripts[0], [scripts])

  useEffect(() => { setScript(adopted?.body ?? '') }, [adopted?.id, adopted?.body])
  const saveRevision = useMutation({
    mutationFn: () => api.createScript(episode.id, { body: script, note: revisionNote.trim() || undefined, adopt: true }),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['scripts', episode.id] })
      setSaveOpen(false)
      setRevisionNote('')
      message.success('剧情新版本已采用')
    },
    onError,
  })
  const adoptRevision = useMutation({
    mutationFn: api.adoptScript,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['scripts', episode.id] })
      message.success('已切换采用版本')
    },
    onError,
  })

  if (scriptsQuery.isLoading) return <div className="episode-expanded-content"><Skeleton active paragraph={{ rows: 5 }}/></div>
  if (view === 'story') return <div className="episode-expanded-content episode-story-view"><MarkdownPreview emptyText="这一集还没有剧情内容。" value={adopted?.body}/></div>
  if (view === 'details') return <EpisodeDetailsEditor episode={episode} key={episode.updated_at} onError={onError} onRefreshEpisodes={onRefreshEpisodes}/>
  if (view === 'history') {
    return <div className="episode-expanded-content"><div className="episode-expanded-heading"><span>版本历史</span><em>{scripts.length} 个版本</em></div>{scripts.length ? <div className="version-list">{scripts.map((item) => <div className="revision-item" key={item.id}><div className="revision-item-head"><Space><Typography.Text strong>版本 {item.version}</Typography.Text><Tag color={item.status === 'adopted' ? 'orange' : 'default'}>{item.status}</Tag></Space><Typography.Text type="secondary">{new Date(item.created_at).toLocaleString()}</Typography.Text></div><details className="revision-body"><summary>正文内容</summary><MarkdownPreview emptyText="空剧情" value={item.body}/></details><div className="revision-item-foot"><Typography.Text type="secondary">{item.note || '无版本说明'}</Typography.Text>{item.status !== 'adopted' && <Button icon={<HistoryOutlined/>} loading={adoptRevision.isPending} size="small" onClick={() => adoptRevision.mutate(item.id)}>采用此版本</Button>}</div></div>)}</div> : <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无版本"/>}</div>
  }
  return <div className="episode-expanded-content episode-edit-view">
    <div className="episode-script-edit"><MarkdownEditor height={460} onChange={setScript} onSave={() => setSaveOpen(true)} placeholder="使用 Markdown 编写这一集的剧情正文。" saving={saveRevision.isPending} value={script}/></div>
    <Modal cancelText="取消" confirmLoading={saveRevision.isPending} okText="保存并采用" onCancel={() => setSaveOpen(false)} onOk={() => saveRevision.mutate()} open={saveOpen} title="保存剧情新版本">
      <div className="revision-save-dialog">
        <Typography.Paragraph>确认保存当前剧情，并将其设为这一集采用的版本。</Typography.Paragraph>
        <Input.TextArea autoFocus maxLength={300} onChange={(event) => setRevisionNote(event.target.value)} placeholder="填写本次修改说明（可选）" rows={4} showCount value={revisionNote}/>
      </div>
    </Modal>
  </div>
}

function EpisodeDetailsEditor({ episode, onError, onRefreshEpisodes }: { episode: Episode; onError: (error: unknown) => void; onRefreshEpisodes: () => Promise<unknown> }) {
  const { message } = App.useApp()
  const dirty = useRef(false)
  const draft = useRef(detailDraft(episode))
  const update = useMutation({
    mutationFn: () => api.updateEpisode(episode.id, {
      title: draft.current.title.trim() || episode.title,
      episode_number: positiveNumber(draft.current.episode_number, episode.episode_number),
      target_duration_seconds: optionalPositiveNumber(draft.current.target_duration_seconds),
      target_shot_count: optionalPositiveNumber(draft.current.target_shot_count),
      notes: draft.current.notes,
    }),
    onSuccess: async () => {
      await onRefreshEpisodes()
      message.success('分集详情已保存')
    },
    onError: (error) => {
      dirty.current = true
      onError(error)
    },
  })
  const change = (key: keyof ReturnType<typeof detailDraft>, value: string) => {
    dirty.current = true
    draft.current[key] = value
  }
  const save = () => {
    if (!dirty.current || update.isPending) return
    dirty.current = false
    update.mutate()
  }
  return <div className="episode-expanded-content episode-details-editor" onBlurCapture={(event) => {
    if (event.relatedTarget instanceof Node && event.currentTarget.contains(event.relatedTarget)) return
    save()
  }}>
    <div className="episode-expanded-heading"><span>分集详情</span><em>{update.isPending ? '正在保存…' : '点击内容直接编辑，点击空白区域自动保存'}</em></div>
    <div className="episode-detail-grid">
      <InlineDetail label="标题" onChange={(value) => change('title', value)} value={draft.current.title} wide/>
      <InlineDetail label="集数" onChange={(value) => change('episode_number', value)} value={draft.current.episode_number}/>
      <InlineDetail label="目标时长（秒）" onChange={(value) => change('target_duration_seconds', value)} placeholder="未设置" value={draft.current.target_duration_seconds}/>
      <InlineDetail label="目标镜头数" onChange={(value) => change('target_shot_count', value)} placeholder="未设置" value={draft.current.target_shot_count}/>
      <div className="episode-detail"><span>更新时间</span><div className="episode-detail-value readonly">{new Date(episode.updated_at).toLocaleString()}</div></div>
      <InlineDetail label="制作备注" onChange={(value) => change('notes', value)} placeholder="暂无备注" value={draft.current.notes} wide/>
    </div>
  </div>
}

function InlineDetail({ label, value, onChange, placeholder = '点击编辑', wide = false }: { label: string; value: string; onChange: (value: string) => void; placeholder?: string; wide?: boolean }) {
  const element = useRef<HTMLDivElement | null>(null)
  useEffect(() => {
    if (document.activeElement !== element.current && element.current && element.current.textContent !== value) element.current.textContent = value
  }, [value])
  return <div className={`episode-detail${wide ? ' wide' : ''}`}><span>{label}</span><div className="episode-detail-value" contentEditable data-placeholder={placeholder} onInput={(event) => onChange(event.currentTarget.textContent ?? '')} ref={element} role="textbox" suppressContentEditableWarning tabIndex={0}>{value}</div></div>
}

function detailDraft(episode: Episode) {
  return {
    title: episode.title,
    episode_number: String(episode.episode_number),
    target_duration_seconds: episode.target_duration_seconds ? String(episode.target_duration_seconds) : '',
    target_shot_count: episode.target_shot_count ? String(episode.target_shot_count) : '',
    notes: episode.notes || '',
  }
}

function positiveNumber(value: string, fallback: number) {
  const parsed = Number.parseInt(value, 10)
  return Number.isFinite(parsed) && parsed > 0 ? parsed : fallback
}

function optionalPositiveNumber(value: string) {
  if (!value.trim()) return null
  const parsed = Number.parseInt(value, 10)
  return Number.isFinite(parsed) && parsed > 0 ? parsed : null
}
