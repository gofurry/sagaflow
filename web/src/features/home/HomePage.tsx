import { useEffect, useMemo, useState } from 'react'
import { ArrowRightOutlined, PlusOutlined, RocketOutlined, SearchOutlined } from '@ant-design/icons'
import { Empty, Input, Pagination, Select, Skeleton, Typography } from 'antd'
import type { Project } from '../../api/types'
import { FloatingToolbar } from '../../components/FloatingToolbar'

interface Props {
  activeProjectID: string
  loading: boolean
  projects: Project[]
  onCreate: () => void
  onOpenProject: (id: string) => void
}

type ProjectSort = 'updated_desc' | 'updated_asc' | 'created_desc' | 'created_asc'
const PAGE_SIZE = 6

export function HomePage({ activeProjectID, loading, projects, onCreate, onOpenProject }: Props) {
  const [search, setSearch] = useState('')
  const [sort, setSort] = useState<ProjectSort>('updated_desc')
  const [page, setPage] = useState(1)
  const filteredProjects = useMemo(() => {
    const keyword = search.trim().toLocaleLowerCase()
    const filtered = keyword ? projects.filter((project) => `${project.title} ${project.description}`.toLocaleLowerCase().includes(keyword)) : [...projects]
    const [field, direction] = sort.split('_') as ['updated' | 'created', 'asc' | 'desc']
    return filtered.sort((left, right) => {
      const leftTime = new Date(field === 'updated' ? left.updated_at : left.created_at).getTime()
      const rightTime = new Date(field === 'updated' ? right.updated_at : right.created_at).getTime()
      return direction === 'desc' ? rightTime - leftTime : leftTime - rightTime
    })
  }, [projects, search, sort])
  const pageProjects = filteredProjects.slice((page - 1) * PAGE_SIZE, page * PAGE_SIZE)
  useEffect(() => {
    const lastPage = Math.max(1, Math.ceil(filteredProjects.length / PAGE_SIZE))
    if (page > lastPage) setPage(lastPage)
  }, [filteredProjects.length, page])

  return <div className="home-page">
    <FloatingToolbar ariaLabel="主页工具栏" items={[
      { key: 'new', label: '新建项目', icon: <PlusOutlined/>, active: true, onClick: onCreate },
      { key: 'continue', label: '继续项目', icon: <RocketOutlined/>, disabled: projects.length === 0, onClick: () => {
        const projectID = activeProjectID || projects[0]?.id
        if (projectID) onOpenProject(projectID)
      } },
    ]}/>

    <section className="project-section">
      <div className="project-section-head"><Typography.Title level={3}>我的项目</Typography.Title><div className="project-count"><strong>{filteredProjects.length}</strong><span>{search.trim() ? `/ ${projects.length} 个项目` : '个项目'}</span></div></div>
      <div className="project-filter-bar">
        <Input allowClear onChange={(event) => { setSearch(event.target.value); setPage(1) }} placeholder="搜索项目名称或说明" prefix={<SearchOutlined/>} value={search}/>
        <Select<ProjectSort> onChange={(value) => { setSort(value); setPage(1) }} options={[
          { value: 'updated_desc', label: '最近更新' },
          { value: 'updated_asc', label: '最早更新' },
          { value: 'created_desc', label: '最新创建' },
          { value: 'created_asc', label: '最早创建' },
        ]} value={sort}/>
      </div>
      {loading ? <div className="project-grid"><Skeleton.Node active className="project-skeleton"/><Skeleton.Node active className="project-skeleton"/></div> : projects.length === 0
        ? <Empty className="project-empty" image={Empty.PRESENTED_IMAGE_SIMPLE} description="还没有项目，从右侧工具栏新建一个项目"/>
        : filteredProjects.length === 0
        ? <Empty className="project-empty" image={Empty.PRESENTED_IMAGE_SIMPLE} description="没有符合搜索条件的项目"/>
        : <div className="project-grid">
        {pageProjects.map((project, index) => {
          const isActive = project.id === activeProjectID
          const projectIndex = String((page - 1) * PAGE_SIZE + index + 1).padStart(2, '0')
          return <button className={`project-card${isActive ? ' active' : ''}`} key={project.id} onClick={() => onOpenProject(project.id)} type="button">
          <div className="project-card-top">
            <strong className="project-card-title">{project.title}</strong>
            <span aria-label={isActive ? `最近使用的项目，编号 ${projectIndex}` : `项目编号 ${projectIndex}`} className={`project-index${isActive ? ' recent' : ''}`}>{projectIndex}</span>
          </div>
          <div className="project-card-body"><p>{project.description || '还没有项目说明，进入项目完善你的故事设定。'}</p></div>
          <div className="project-card-foot"><span>更新于 {formatDate(project.updated_at)}</span><span className="project-enter">进入项目 <ArrowRightOutlined/></span></div>
        </button>
        })}
      </div>}
      {!loading && filteredProjects.length > PAGE_SIZE && <Pagination className="project-pagination" current={page} hideOnSinglePage onChange={setPage} pageSize={PAGE_SIZE} showSizeChanger={false} total={filteredProjects.length}/>}
    </section>
  </div>
}

function formatDate(value: string) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '刚刚'
  return date.toLocaleDateString('zh-CN', { month: '2-digit', day: '2-digit' })
}
