interface JobState {
  id: string
  status: string
}

export function completedJobTransition<T extends JobState>(
  previous: ReadonlySet<string> | null,
  jobs: readonly T[],
  hasOutput: (job: T) => boolean,
) {
  const current = new Set(jobs.filter((job) => job.status === 'succeeded' && hasOutput(job)).map((job) => job.id))
  const added = previous === null ? [] : [...current].filter((id) => !previous.has(id))
  return { current, added }
}
