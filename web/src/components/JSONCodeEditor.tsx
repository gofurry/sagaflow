import { useMemo, useState } from 'react'
import { CheckOutlined, FormatPainterOutlined } from '@ant-design/icons'
import { json, jsonParseLinter } from '@codemirror/lang-json'
import { lintGutter, linter } from '@codemirror/lint'
import { EditorView } from '@codemirror/view'
import CodeMirror from '@uiw/react-codemirror'
import { Button } from 'antd'

interface JSONCodeEditorProps {
  height?: number | string
  onChange?: (value: string) => void
  value?: string
}

export function JSONCodeEditor({ height = 280, onChange, value = '' }: JSONCodeEditorProps) {
  const [formatState, setFormatState] = useState<'idle' | 'done' | 'error'>('idle')
  const extensions = useMemo(() => [json(), lintGutter(), linter(jsonParseLinter()), EditorView.lineWrapping], [])
  const format = () => {
    try {
      onChange?.(JSON.stringify(JSON.parse(value || '{}'), null, 2))
      setFormatState('done')
      window.setTimeout(() => setFormatState('idle'), 1200)
    } catch {
      setFormatState('error')
      window.setTimeout(() => setFormatState('idle'), 1800)
    }
  }

  return <div className={`json-code-editor${formatState === 'error' ? ' invalid' : ''}`}>
    <div className="json-code-toolbar">
      <span>JSON</span>
      <Button icon={formatState === 'done' ? <CheckOutlined/> : <FormatPainterOutlined/>} onClick={format} size="small" type="text">
        {formatState === 'error' ? '格式错误' : formatState === 'done' ? '已格式化' : '格式化'}
      </Button>
    </div>
    <CodeMirror
      basicSetup={{ autocompletion: false, bracketMatching: true, closeBrackets: true, foldGutter: true, highlightActiveLine: true, highlightActiveLineGutter: true, lineNumbers: true }}
      extensions={extensions}
      height={typeof height === 'number' ? `${height}px` : height}
      onChange={(next) => onChange?.(next)}
      value={value}
    />
  </div>
}
