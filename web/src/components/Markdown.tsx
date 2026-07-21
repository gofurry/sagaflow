import { useMemo } from 'react'
import { SaveOutlined } from '@ant-design/icons'
import MDEditor, { commands, type ICommand } from '@uiw/react-md-editor'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import '@uiw/react-md-editor/markdown-editor.css'
import '@uiw/react-markdown-preview/markdown.css'

interface EditorProps {
  value?: string
  onChange?: (value: string) => void
  height?: number
  placeholder?: string
  onSave?: () => void
  saving?: boolean
}

export function MarkdownEditor({ value = '', onChange, height = 420, placeholder = '使用 Markdown 编写内容…', onSave, saving = false }: EditorProps) {
  const extraCommands = useMemo<ICommand[] | undefined>(() => {
    if (!onSave) return undefined
    const saveCommand: ICommand = {
      name: 'save-revision',
      keyCommand: 'save-revision',
      icon: <SaveOutlined/>,
      buttonProps: {
        'aria-label': saving ? '正在保存新版本' : '保存新版本',
        disabled: saving,
        title: saving ? '正在保存…' : '保存新版本',
      },
      execute: onSave,
    }
    return [saveCommand, commands.divider, ...commands.getExtraCommands()]
  }, [onSave, saving])

  return <div className="markdown-editor" data-color-mode="light"><MDEditor extraCommands={extraCommands} height={height} onChange={(next) => onChange?.(next ?? '')} preview="live" textareaProps={{ placeholder }} value={value} visibleDragbar={false}/></div>
}

export function MarkdownPreview({ value, emptyText = '暂无内容' }: { value?: string; emptyText?: string }) {
  if (!value?.trim()) return <p className="markdown-empty">{emptyText}</p>
  return <div className="markdown-preview"><ReactMarkdown remarkPlugins={[remarkGfm]} skipHtml>{value}</ReactMarkdown></div>
}
