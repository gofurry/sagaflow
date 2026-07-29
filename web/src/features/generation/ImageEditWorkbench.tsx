import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { ClearOutlined, DeleteOutlined, HighlightOutlined, RedoOutlined, UndoOutlined } from '@ant-design/icons'
import { Alert, Button, Modal, Slider, Tooltip } from 'antd'
import type { GenerationImageTask } from '../../api/types'
import { binarizeMaskAlpha } from './imageMask'

export type ImageEditMode = 'outpaint' | 'inpaint'

export interface ImageEditResult {
  task: GenerationImageTask
  maskFile?: File
}

interface Point { x: number; y: number }
interface MaskStroke { erase: boolean; size: number; points: Point[] }
interface OutpaintDrag {
  x: number
  y: number
  edges: { left: boolean; right: boolean; top: boolean; bottom: boolean }
  scales: OutpaintScales
  previewScale: number
}
interface OutpaintScales { top: number; bottom: number; left: number; right: number }

const defaultScales: OutpaintScales = { top: 1.2, bottom: 1.2, left: 1.25, right: 1.25 }

export function ImageEditWorkbench({ initialTask, mode, onApply, onCancel, open, sourceName, sourceURL }: {
  initialTask?: GenerationImageTask
  mode: ImageEditMode
  onApply: (result: ImageEditResult) => Promise<void>
  onCancel: () => void
  open: boolean
  sourceName: string
  sourceURL: string
}) {
  const baseCanvasRef = useRef<HTMLCanvasElement>(null)
  const overlayCanvasRef = useRef<HTMLCanvasElement>(null)
  const maskCanvasRef = useRef<HTMLCanvasElement | null>(null)
  const brushCursorRef = useRef<HTMLDivElement>(null)
  const imageRef = useRef<HTMLImageElement | null>(null)
  const outpaintDragRef = useRef<OutpaintDrag | null>(null)
  const activeStrokeRef = useRef<MaskStroke | null>(null)
  const [imageSize, setImageSize] = useState({ width: 0, height: 0 })
  const [loadError, setLoadError] = useState('')
  const [applyError, setApplyError] = useState('')
  const [applying, setApplying] = useState(false)
  const [scales, setScales] = useState<OutpaintScales>(defaultScales)
  const [maskTool, setMaskTool] = useState<'paint' | 'erase'>('paint')
  const [brushSize, setBrushSize] = useState(72)
  const [strokes, setStrokes] = useState<MaskStroke[]>([])
  const [redoStrokes, setRedoStrokes] = useState<MaskStroke[]>([])

  useEffect(() => {
    if (!open) return
    setLoadError('')
    setApplyError('')
    setImageSize({ width: 0, height: 0 })
    setStrokes([])
    setRedoStrokes([])
    const image = new Image()
    image.onload = () => {
      if (image.naturalWidth < 512 || image.naturalWidth > 4096 || image.naturalHeight < 512 || image.naturalHeight > 4096) {
        setLoadError(`源图尺寸为 ${image.naturalWidth} × ${image.naturalHeight}，精确编辑要求宽高均在 512–4096 像素之间。`)
        return
      }
      imageRef.current = image
      setImageSize({ width: image.naturalWidth, height: image.naturalHeight })
      const mask = document.createElement('canvas')
      mask.width = image.naturalWidth
      mask.height = image.naturalHeight
      maskCanvasRef.current = mask
      if (initialTask?.type === 'outpaint') {
        setScales({
          top: initialTask.top_scale ?? defaultScales.top,
          bottom: initialTask.bottom_scale ?? defaultScales.bottom,
          left: initialTask.left_scale ?? defaultScales.left,
          right: initialTask.right_scale ?? defaultScales.right,
        })
      } else {
        setScales(defaultScales)
      }
    }
    image.onerror = () => setLoadError('无法读取源图，请重新选择 PNG 或 JPG 图片。')
    image.src = sourceURL
    return () => { image.onload = null; image.onerror = null }
  }, [initialTask, open, sourceURL])

  const outpaintGeometry = useMemo(() => {
    if (!imageSize.width || !imageSize.height) return null
    const width = Math.round(imageSize.width * (scales.left + scales.right - 1))
    const height = Math.round(imageSize.height * (scales.top + scales.bottom - 1))
    return {
      width,
      height,
      sourceX: Math.round(imageSize.width * (scales.left - 1)),
      sourceY: Math.round(imageSize.height * (scales.top - 1)),
    }
  }, [imageSize, scales])

  const drawOutpaint = useCallback(() => {
    const canvas = baseCanvasRef.current
    const image = imageRef.current
    if (!canvas || !image || !outpaintGeometry) return
    const previewScale = Math.min(920 / outpaintGeometry.width, 560 / outpaintGeometry.height, 1)
    canvas.width = Math.max(1, Math.round(outpaintGeometry.width * previewScale))
    canvas.height = Math.max(1, Math.round(outpaintGeometry.height * previewScale))
    canvas.dataset.previewScale = String(previewScale)
    const context = canvas.getContext('2d')!
    context.fillStyle = '#fff'
    context.fillRect(0, 0, canvas.width, canvas.height)
    context.drawImage(image,
      outpaintGeometry.sourceX * previewScale,
      outpaintGeometry.sourceY * previewScale,
      imageSize.width * previewScale,
      imageSize.height * previewScale,
    )
    context.strokeStyle = 'rgba(200, 117, 63, .82)'
    context.lineWidth = 2
    context.setLineDash([7, 5])
    context.strokeRect(
      outpaintGeometry.sourceX * previewScale + 1,
      outpaintGeometry.sourceY * previewScale + 1,
      imageSize.width * previewScale - 2,
      imageSize.height * previewScale - 2,
    )
  }, [imageSize, outpaintGeometry])

  const drawMask = useCallback((values: MaskStroke[]) => {
    const mask = maskCanvasRef.current
    if (!mask) return
    const context = mask.getContext('2d')!
    context.clearRect(0, 0, mask.width, mask.height)
    for (const stroke of values) drawStroke(context, stroke)
    drawMaskOverlay(mask, overlayCanvasRef.current)
  }, [])

  const drawInpaint = useCallback(() => {
    const canvas = baseCanvasRef.current
    const overlay = overlayCanvasRef.current
    const image = imageRef.current
    if (!canvas || !overlay || !image) return
    const previewScale = Math.min(920 / imageSize.width, 560 / imageSize.height, 1)
    canvas.width = Math.max(1, Math.round(imageSize.width * previewScale))
    canvas.height = Math.max(1, Math.round(imageSize.height * previewScale))
    overlay.width = canvas.width
    overlay.height = canvas.height
    overlay.dataset.previewScale = String(previewScale)
    canvas.getContext('2d')!.drawImage(image, 0, 0, canvas.width, canvas.height)
    drawMask(strokes)
  }, [drawMask, imageSize, strokes])

  useEffect(() => {
    if (!imageSize.width) return
    if (mode === 'outpaint') drawOutpaint()
    else drawInpaint()
  }, [drawInpaint, drawOutpaint, imageSize, mode])

  const outpaintPointerDown = (event: React.PointerEvent<HTMLCanvasElement>) => {
    const canvas = event.currentTarget
    const edges = outpaintEdges(canvas, event)
    if (!edges.left && !edges.right && !edges.top && !edges.bottom) return
    canvas.setPointerCapture(event.pointerId)
    outpaintDragRef.current = {
      x: event.clientX, y: event.clientY, edges, scales: { ...scales },
      previewScale: Number(canvas.dataset.previewScale) || 1,
    }
  }
  const outpaintPointerMove = (event: React.PointerEvent<HTMLCanvasElement>) => {
    const drag = outpaintDragRef.current
    if (!drag) {
      event.currentTarget.style.cursor = outpaintCursor(outpaintEdges(event.currentTarget, event))
      return
    }
    const dx = (event.clientX - drag.x) / drag.previewScale / imageSize.width
    const dy = (event.clientY - drag.y) / drag.previewScale / imageSize.height
    setScales({
      left: clampScale(drag.scales.left + (drag.edges.left ? -dx : 0)),
      right: clampScale(drag.scales.right + (drag.edges.right ? dx : 0)),
      top: clampScale(drag.scales.top + (drag.edges.top ? -dy : 0)),
      bottom: clampScale(drag.scales.bottom + (drag.edges.bottom ? dy : 0)),
    })
  }
  const outpaintPointerUp = (event: React.PointerEvent<HTMLCanvasElement>) => {
    if (event.currentTarget.hasPointerCapture(event.pointerId)) event.currentTarget.releasePointerCapture(event.pointerId)
    outpaintDragRef.current = null
  }

  const maskPointer = (event: React.PointerEvent<HTMLCanvasElement>, phase: 'start' | 'move' | 'end') => {
    const canvas = event.currentTarget
    const mask = maskCanvasRef.current
    if (!mask) return
    updateBrushCursor(brushCursorRef.current, canvas, event, brushSize, maskTool, mask.width)
    if (phase === 'start') {
      canvas.setPointerCapture(event.pointerId)
      const stroke: MaskStroke = { erase: maskTool === 'erase', size: brushSize, points: [maskPoint(canvas, event, mask)] }
      activeStrokeRef.current = stroke
      drawStroke(mask.getContext('2d')!, stroke)
      drawMaskOverlay(mask, canvas)
      return
    }
    const stroke = activeStrokeRef.current
    if (!stroke) return
    if (phase === 'move') {
      stroke.points.push(maskPoint(canvas, event, mask))
      drawStrokeSegment(mask.getContext('2d')!, stroke, stroke.points.length - 2)
      drawMaskOverlay(mask, canvas)
      return
    }
    if (canvas.hasPointerCapture(event.pointerId)) canvas.releasePointerCapture(event.pointerId)
    activeStrokeRef.current = null
    setStrokes((current) => [...current, stroke])
    setRedoStrokes([])
  }

  const apply = async () => {
    if (!imageSize.width || !outpaintGeometry) return
    setApplyError('')
    setApplying(true)
    try {
      if (mode === 'outpaint') {
        await onApply({ task: {
          type: 'outpaint', source_width: imageSize.width, source_height: imageSize.height,
          target_width: outpaintGeometry.width, target_height: outpaintGeometry.height,
          source_x: outpaintGeometry.sourceX, source_y: outpaintGeometry.sourceY,
          top_scale: scales.top, bottom_scale: scales.bottom, left_scale: scales.left, right_scale: scales.right,
        } })
        return
      }
      const mask = maskCanvasRef.current
      if (!mask || !maskHasPaint(mask)) throw new Error('请先涂抹需要重绘的区域')
      const output = document.createElement('canvas')
      output.width = mask.width
      output.height = mask.height
      const context = output.getContext('2d')!
      const sourcePixels = mask.getContext('2d')!.getImageData(0, 0, mask.width, mask.height)
      const binaryPixels = context.createImageData(output.width, output.height)
      binaryPixels.data.set(binarizeMaskAlpha(sourcePixels.data))
      context.putImageData(binaryPixels, 0, 0)
      const blob = await canvasBlob(output)
      await onApply({
        task: { type: 'inpaint', source_width: imageSize.width, source_height: imageSize.height },
        maskFile: new File([blob], `${safeName(sourceName)}-mask.png`, { type: 'image/png' }),
      })
    } catch (error) {
      setApplyError(error instanceof Error ? error.message : '应用编辑设置失败，请稍后重试。')
    } finally {
      setApplying(false)
    }
  }

  return <Modal
    cancelText="取消"
    className="image-edit-workbench-modal"
    confirmLoading={applying}
    destroyOnHidden
    okButtonProps={{ disabled: Boolean(loadError) || !imageSize.width }}
    okText="应用编辑"
    onCancel={onCancel}
    onOk={() => void apply()}
    open={open}
    title={mode === 'outpaint' ? '设置扩图画幅' : '绘制重绘区域'}
    width={1180}
  >
    <div className="image-edit-workbench">
      <div className="image-edit-toolbar">
        {mode === 'outpaint'
          ? <>
              <span>拖动画布四边或四角改变扩展范围</span>
              <Button onClick={() => setScales(defaultScales)} size="small">重置画幅</Button>
            </>
          : <>
              <div className="image-edit-mask-tools">
                <Tooltip title="涂抹重绘区域">
                  <Button aria-label="涂抹重绘区域" icon={<HighlightOutlined/>} onClick={() => setMaskTool('paint')} type={maskTool === 'paint' ? 'primary' : 'default'}/>
                </Tooltip>
                <Tooltip title="擦除重绘区域">
                  <Button aria-label="擦除重绘区域" icon={<ClearOutlined/>} onClick={() => setMaskTool('erase')} type={maskTool === 'erase' ? 'primary' : 'default'}/>
                </Tooltip>
              </div>
              <label><span>画笔大小</span><Slider max={240} min={12} onChange={setBrushSize} value={brushSize}/><em>{brushSize}px</em></label>
              <Button disabled={!strokes.length} icon={<UndoOutlined/>} onClick={() => {
                const last = strokes.at(-1)
                if (!last) return
                setStrokes((current) => current.slice(0, -1))
                setRedoStrokes((current) => [...current, last])
              }} size="small">撤销</Button>
              <Button disabled={!redoStrokes.length} icon={<RedoOutlined/>} onClick={() => {
                const next = redoStrokes.at(-1)
                if (!next) return
                setRedoStrokes((current) => current.slice(0, -1))
                setStrokes((current) => [...current, next])
              }} size="small">重做</Button>
              <Button danger disabled={!strokes.length} icon={<DeleteOutlined/>} onClick={() => { setStrokes([]); setRedoStrokes([]) }} size="small">清空</Button>
            </>}
      </div>
      {loadError && <Alert message={loadError} showIcon type="error"/>}
      {applyError && <Alert message={applyError} showIcon type="error"/>}
      <div className={`image-edit-stage ${mode}`}>
        <canvas
          aria-label={mode === 'outpaint' ? '扩图画幅预览' : '源图预览'}
          className="image-edit-base-canvas"
          onPointerDown={mode === 'outpaint' ? outpaintPointerDown : undefined}
          onPointerMove={mode === 'outpaint' ? outpaintPointerMove : undefined}
          onPointerUp={mode === 'outpaint' ? outpaintPointerUp : undefined}
          ref={baseCanvasRef}
        />
        {mode === 'inpaint' && <canvas
          aria-label="绘制重绘区域"
          className="image-edit-mask-canvas"
          onPointerDown={(event) => maskPointer(event, 'start')}
          onPointerEnter={(event) => {
            const sourceWidth = maskCanvasRef.current?.width
            if (sourceWidth) updateBrushCursor(brushCursorRef.current, event.currentTarget, event, brushSize, maskTool, sourceWidth)
          }}
          onPointerLeave={() => hideBrushCursor(brushCursorRef.current)}
          onPointerMove={(event) => maskPointer(event, 'move')}
          onPointerUp={(event) => maskPointer(event, 'end')}
          ref={overlayCanvasRef}
        />}
        {mode === 'inpaint' && <div aria-hidden className={`image-edit-brush-cursor ${maskTool}`} ref={brushCursorRef}/>}
      </div>
      {imageSize.width > 0 && <div className="image-edit-status">
        <span>源图 {imageSize.width} × {imageSize.height}</span>
        {mode === 'outpaint' && outpaintGeometry
          ? <><span>输出 {outpaintGeometry.width} × {outpaintGeometry.height}</span><span>原图位置 {outpaintGeometry.sourceX}, {outpaintGeometry.sourceY}</span></>
          : <span>红色区域会被重绘；提交时自动转换为同尺寸黑白引导图</span>}
      </div>}
    </div>
  </Modal>
}

function updateBrushCursor(cursor: HTMLDivElement | null, canvas: HTMLCanvasElement, event: React.PointerEvent<HTMLCanvasElement>, brushSize: number, tool: 'paint' | 'erase', sourceWidth: number) {
  if (!cursor) return
  const canvasRect = canvas.getBoundingClientRect()
  const stageRect = canvas.parentElement?.getBoundingClientRect()
  if (!stageRect || !sourceWidth) return
  const diameter = Math.max(4, brushSize * canvasRect.width / sourceWidth)
  cursor.style.width = `${diameter}px`
  cursor.style.height = `${diameter}px`
  cursor.style.left = `${event.clientX - stageRect.left}px`
  cursor.style.top = `${event.clientY - stageRect.top}px`
  cursor.classList.toggle('erase', tool === 'erase')
  cursor.classList.toggle('paint', tool === 'paint')
  cursor.style.opacity = '1'
}

function hideBrushCursor(cursor: HTMLDivElement | null) {
  if (cursor) cursor.style.opacity = '0'
}

function clampScale(value: number) { return Math.min(2, Math.max(1, Math.round(value * 1000) / 1000)) }

function outpaintEdges(canvas: HTMLCanvasElement, event: React.PointerEvent<HTMLCanvasElement>) {
  const rect = canvas.getBoundingClientRect()
  const x = event.clientX - rect.left
  const y = event.clientY - rect.top
  const hit = 24
  return { left: x <= hit, right: x >= rect.width - hit, top: y <= hit, bottom: y >= rect.height - hit }
}

function outpaintCursor(edges: ReturnType<typeof outpaintEdges>) {
  if ((edges.left && edges.top) || (edges.right && edges.bottom)) return 'nwse-resize'
  if ((edges.right && edges.top) || (edges.left && edges.bottom)) return 'nesw-resize'
  if (edges.left || edges.right) return 'ew-resize'
  if (edges.top || edges.bottom) return 'ns-resize'
  return 'default'
}

function maskPoint(canvas: HTMLCanvasElement, event: React.PointerEvent<HTMLCanvasElement>, mask: HTMLCanvasElement): Point {
  const rect = canvas.getBoundingClientRect()
  return { x: (event.clientX - rect.left) * mask.width / rect.width, y: (event.clientY - rect.top) * mask.height / rect.height }
}

function drawStroke(context: CanvasRenderingContext2D, stroke: MaskStroke) {
  if (!stroke.points.length) return
  context.save()
  context.globalCompositeOperation = stroke.erase ? 'destination-out' : 'source-over'
  context.strokeStyle = '#fff'
  context.fillStyle = '#fff'
  context.lineCap = 'round'
  context.lineJoin = 'round'
  context.lineWidth = stroke.size
  if (stroke.points.length === 1) {
    context.beginPath()
    context.arc(stroke.points[0].x, stroke.points[0].y, stroke.size / 2, 0, Math.PI * 2)
    context.fill()
  } else {
    context.beginPath()
    context.moveTo(stroke.points[0].x, stroke.points[0].y)
    stroke.points.slice(1).forEach((point) => context.lineTo(point.x, point.y))
    context.stroke()
  }
  context.restore()
}

function drawStrokeSegment(context: CanvasRenderingContext2D, stroke: MaskStroke, index: number) {
  const points = stroke.points.slice(Math.max(0, index), index + 2)
  drawStroke(context, { ...stroke, points })
}

function drawMaskOverlay(mask: HTMLCanvasElement, overlay?: HTMLCanvasElement | null) {
  if (!overlay) return
  const context = overlay.getContext('2d')!
  context.clearRect(0, 0, overlay.width, overlay.height)
  context.drawImage(mask, 0, 0, overlay.width, overlay.height)
  context.globalCompositeOperation = 'source-in'
  context.fillStyle = 'rgba(220, 70, 54, .48)'
  context.fillRect(0, 0, overlay.width, overlay.height)
  context.globalCompositeOperation = 'source-over'
}

function maskHasPaint(mask: HTMLCanvasElement) {
  const data = mask.getContext('2d')!.getImageData(0, 0, mask.width, mask.height).data
  for (let index = 3; index < data.length; index += 4) if (data[index] > 0) return true
  return false
}

function canvasBlob(canvas: HTMLCanvasElement) {
  return new Promise<Blob>((resolve, reject) => canvas.toBlob((blob) => blob ? resolve(blob) : reject(new Error('无法导出重绘区域')), 'image/png'))
}

function safeName(value: string) {
  return value.replace(/\.[^.]+$/, '').replace(/[^a-zA-Z0-9\u4e00-\u9fff_-]+/g, '-').slice(0, 80) || 'image'
}
