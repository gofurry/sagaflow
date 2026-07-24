import { describe, expect, it } from 'vitest'
import { completedJobTransition } from './completionTransitions'

interface TestJob {
  id: string
  status: string
  output?: string
}

const hasOutput = (job: TestJob) => Boolean(job.output)

describe('completedJobTransition', () => {
  it('does not replay historical completed jobs on first observation', () => {
    const result = completedJobTransition(null, [{ id: 'old', status: 'succeeded', output: 'asset' }], hasOutput)
    expect(result.added).toEqual([])
    expect([...result.current]).toEqual(['old'])
  })

  it('reports a running job exactly once when it completes', () => {
    const initial = completedJobTransition(null, [
      { id: 'old', status: 'succeeded', output: 'asset' },
      { id: 'new', status: 'running' },
    ], hasOutput)
    const completed = completedJobTransition(initial.current, [
      { id: 'old', status: 'succeeded', output: 'asset' },
      { id: 'new', status: 'succeeded', output: 'asset' },
    ], hasOutput)
    const polledAgain = completedJobTransition(completed.current, [
      { id: 'old', status: 'succeeded', output: 'asset' },
      { id: 'new', status: 'succeeded', output: 'asset' },
    ], hasOutput)

    expect(completed.added).toEqual(['new'])
    expect(polledAgain.added).toEqual([])
  })
})
