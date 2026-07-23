import { useEffect, useState } from 'react'
import { Checkbox, Input, InputNumber, Select } from 'antd'
import type { JSONSchema } from '../api/types'

type ParameterSchema = NonNullable<JSONSchema['properties']>[string]

export function ModelParameterEditor({ definition, value, onChange, hiddenKeys = [] }: { definition: { parameter_schema: JSONSchema }; value: Record<string, unknown>; onChange: (value: Record<string, unknown>) => void; hiddenKeys?: string[] }) {
	const properties = Object.fromEntries(Object.entries(definition.parameter_schema?.properties ?? {}).filter(([key, schema]) => !hiddenKeys.includes(key) && !schema.readOnly))
  const set = (key: string, next: unknown) => onChange({ ...value, [key]: next })
  return <div className="parameter-editor">
    <div className="parameter-editor-heading">
      <strong className="parameter-editor-title">模型参数</strong>
      <span>{Object.keys(properties).length} 项</span>
    </div>
    {Object.entries(properties).map(([key, schema]) => <ParameterField key={key} name={key} onChange={(next) => set(key, next)} schema={schema} value={value[key]}/>)}
  </div>
}

function ParameterField({ name, schema, value, onChange }: { name: string; schema: ParameterSchema; value: unknown; onChange: (value: unknown) => void }) {
  return <div className="parameter-field">
    <label className="field-label">{schema.title ?? name}</label>
    {schema.description && <small>{schema.description}</small>}
    {schema.enum
      ? <Select options={schema.enum.map((item) => ({ value: item as string | number, label: item === '' ? '默认' : String(item) }))} value={value as string | number} onChange={onChange}/>
      : schema.type === 'boolean'
        ? <Checkbox checked={Boolean(value)} onChange={(event) => onChange(event.target.checked)}>启用</Checkbox>
        : schema.type === 'number' || schema.type === 'integer'
          ? <InputNumber max={schema.maximum} min={schema.minimum} precision={schema.type === 'integer' ? 0 : undefined} step={schema.multipleOf ?? (schema.type === 'integer' ? 1 : 0.1)} value={value as number} onChange={onChange} style={{ width: '100%' }}/>
          : schema.type === 'object' || schema.type === 'array'
            ? <JSONParameterInput expected={schema.type} onChange={onChange} value={value}/>
            : schema.format === 'textarea'
              ? <Input.TextArea autoSize={{ minRows: 3, maxRows: 8 }} value={String(value ?? '')} onChange={(event) => onChange(event.target.value)}/>
              : <Input value={String(value ?? '')} onChange={(event) => onChange(event.target.value)}/>}
  </div>
}

function JSONParameterInput({ expected, value, onChange }: { expected: 'object' | 'array'; value: unknown; onChange: (value: unknown) => void }) {
  const [text, setText] = useState(() => JSON.stringify(value ?? (expected === 'array' ? [] : {}), null, 2))
  const [invalid, setInvalid] = useState(false)
  useEffect(() => {
    setText(JSON.stringify(value ?? (expected === 'array' ? [] : {}), null, 2))
    setInvalid(false)
  }, [expected, value])
  const commit = () => {
    const fallback = expected === 'array' ? [] : {}
    try {
      const parsed = JSON.parse(text || JSON.stringify(fallback)) as unknown
      if ((expected === 'array' && !Array.isArray(parsed)) || (expected === 'object' && (Array.isArray(parsed) || typeof parsed !== 'object' || parsed === null))) {
        setInvalid(true)
        return
      }
      setInvalid(false)
      onChange(parsed)
    } catch {
      setInvalid(true)
    }
  }
  return <>
    <Input.TextArea className="code-input" status={invalid ? 'error' : undefined} autoSize={{ minRows: 3, maxRows: 9 }} value={text} onBlur={commit} onChange={(event) => setText(event.target.value)}/>
    {invalid && <small className="parameter-json-error">请输入有效的 JSON {expected === 'array' ? '数组' : '对象'}</small>}
  </>
}
