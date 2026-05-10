import { useCallback, useEffect, useMemo, useState, type Dispatch, type SetStateAction } from 'react'
import { ApiError, fetchUsageQuotaRefreshTask, refreshUsageQuotas } from '@/lib/api'
import type { UsageQuotaRow } from '@/lib/types'
import { CREDENTIALS_PAGE_SIZE } from './credentialViewModels'

export interface QuotaState {
  loading?: boolean
  error?: string
  refreshTaskId?: string
  refreshStatus?: 'queued' | 'running' | 'completed' | 'failed'
}

interface PendingRefreshTask {
  authIndex: string
  taskId: string
  source: 'batch' | 'row'
}

interface UseQuotaRefreshTasksOptions {
  enabled: boolean
  currentAuthIndexes: string[]
  setQuotaByAuthIndex: Dispatch<SetStateAction<Record<string, UsageQuotaRow[]>>>
  onAuthRequired?: () => void
}

export interface QuotaRefreshTasksState {
  quotaStateByAuthIndex: Record<string, QuotaState>
  quotaRefreshing: boolean
  quotaRefreshError: string
  refreshQuotaForCurrentAuthFilePage: () => Promise<void>
  refreshQuotaForAuthIndex: (authIndex: string) => Promise<void>
}

export function useQuotaRefreshTasks({ enabled, currentAuthIndexes, setQuotaByAuthIndex, onAuthRequired }: UseQuotaRefreshTasksOptions): QuotaRefreshTasksState {
  const [quotaStateByAuthIndex, setQuotaStateByAuthIndex] = useState<Record<string, QuotaState>>({})
  const [pendingRefreshTasks, setPendingRefreshTasks] = useState<PendingRefreshTask[]>([])
  const [batchRefreshSubmitting, setBatchRefreshSubmitting] = useState(false)
  const [quotaRefreshError, setQuotaRefreshError] = useState('')
  const quotaRefreshing = useMemo(
    () => batchRefreshSubmitting || pendingRefreshTasks.some((task) => task.source === 'batch'),
    [batchRefreshSubmitting, pendingRefreshTasks],
  )

  useEffect(() => {
    if (!enabled || pendingRefreshTasks.length === 0) {
      return
    }
    let cancelled = false
    let timer: number | undefined
    const controller = new AbortController()
    const poll = async () => {
      const settledAuthIndexes = new Set<string>()
      const stateUpdates: Record<string, QuotaState> = {}
      const quotaUpdates: Record<string, UsageQuotaRow[]> = {}

      await Promise.all(pendingRefreshTasks.map(async (task) => {
        try {
          const response = await fetchUsageQuotaRefreshTask(task.taskId, controller.signal)
          if (cancelled) {
            return
          }
          stateUpdates[task.authIndex] = {
            refreshTaskId: task.taskId,
            refreshStatus: response.status,
            error: response.status === 'failed' ? quotaRefreshDisplayError(response.error) : undefined,
          }
          if (response.status === 'completed' || response.status === 'failed') {
            settledAuthIndexes.add(task.authIndex)
          }
          if (response.status === 'completed' && response.quota) {
            quotaUpdates[task.authIndex] = response.quota.quota ?? []
          }
        } catch (nextError) {
          if (cancelled || controller.signal.aborted) {
            return
          }
          if (nextError instanceof ApiError && nextError.status === 401) {
            onAuthRequired?.()
            settledAuthIndexes.add(task.authIndex)
            return
          }
          settledAuthIndexes.add(task.authIndex)
          stateUpdates[task.authIndex] = {
            refreshTaskId: task.taskId,
            refreshStatus: 'failed',
            error: quotaErrorMessage(nextError),
          }
        }
      }))

      if (cancelled) {
        return
      }
      if (Object.keys(quotaUpdates).length > 0) {
        setQuotaByAuthIndex((current) => ({ ...current, ...quotaUpdates }))
      }
      if (Object.keys(stateUpdates).length > 0) {
        setQuotaStateByAuthIndex((current) => mergeQuotaStates(current, stateUpdates))
      }
      if (settledAuthIndexes.size > 0) {
        setPendingRefreshTasks((current) => current.filter((task) => !settledAuthIndexes.has(task.authIndex)))
      }
      timer = window.setTimeout(() => {
        void poll()
      }, 5_000)
    }

    void poll()

    return () => {
      cancelled = true
      controller.abort()
      if (timer !== undefined) {
        window.clearTimeout(timer)
      }
    }
  }, [enabled, onAuthRequired, pendingRefreshTasks, setQuotaByAuthIndex])

  const startQuotaRefresh = useCallback(async (authIndexes: string[], source: PendingRefreshTask['source']) => {
    if (authIndexes.length === 0) {
      return
    }
    setQuotaRefreshError('')
    if (source === 'batch') {
      setBatchRefreshSubmitting(true)
    }
    try {
      const response = await refreshUsageQuotas(authIndexes)
      setPendingRefreshTasks((current) => {
        const nextByAuthIndex = new Map(current.map((task) => [task.authIndex, task]))
        for (const task of response.tasks) {
          nextByAuthIndex.set(task.authIndex, { authIndex: task.authIndex, taskId: task.taskId, source })
        }
        return Array.from(nextByAuthIndex.values())
      })
      setQuotaStateByAuthIndex((current) => {
        const next = { ...current }
        for (const task of response.tasks) {
          next[task.authIndex] = {
            ...next[task.authIndex],
            refreshTaskId: task.taskId,
            refreshStatus: 'queued',
            error: undefined,
          }
        }
        for (const rejected of response.rejected ?? []) {
          next[rejected.authIndex] = {
            ...next[rejected.authIndex],
            refreshStatus: 'failed',
            error: quotaRefreshDisplayError(rejected.error),
          }
        }
        return next
      })
    } catch (nextError) {
      if (nextError instanceof ApiError && nextError.status === 401) {
        onAuthRequired?.()
        return
      }
      setQuotaRefreshError(quotaErrorMessage(nextError))
    } finally {
      if (source === 'batch') {
        setBatchRefreshSubmitting(false)
      }
    }
  }, [onAuthRequired])

  const refreshQuotaForCurrentAuthFilePage = useCallback(async () => {
    const refreshableAuthIndexes = currentAuthIndexes.filter((authIndex) => !isQuotaRefreshWorking(quotaStateByAuthIndex[authIndex]))
    await startQuotaRefresh(refreshableAuthIndexes.slice(0, CREDENTIALS_PAGE_SIZE), 'batch')
  }, [currentAuthIndexes, quotaStateByAuthIndex, startQuotaRefresh])

  const refreshQuotaForAuthIndex = useCallback(async (authIndex: string) => {
    if (isQuotaRefreshWorking(quotaStateByAuthIndex[authIndex])) {
      return
    }
    await startQuotaRefresh([authIndex], 'row')
  }, [quotaStateByAuthIndex, startQuotaRefresh])

  return {
    quotaStateByAuthIndex,
    quotaRefreshing,
    quotaRefreshError,
    refreshQuotaForCurrentAuthFilePage,
    refreshQuotaForAuthIndex,
  }
}

function isQuotaRefreshWorking(state: QuotaState | undefined): boolean {
  return state?.refreshStatus === 'queued' || state?.refreshStatus === 'running'
}

function mergeQuotaStates(current: Record<string, QuotaState>, updates: Record<string, QuotaState>): Record<string, QuotaState> {
  let changed = false
  const next = { ...current }
  for (const [authIndex, update] of Object.entries(updates)) {
    const previous = current[authIndex] ?? {}
    const merged = { ...previous, ...update }
    if (
      previous.loading !== merged.loading ||
      previous.error !== merged.error ||
      previous.refreshTaskId !== merged.refreshTaskId ||
      previous.refreshStatus !== merged.refreshStatus
    ) {
      next[authIndex] = merged
      changed = true
    }
  }
  return changed ? next : current
}

export function quotaRefreshDisplayError(error?: string): string {
  switch (error) {
    case 'duplicate':
      return 'Quota refresh is already running for this credential.'
    case 'not_auth_file':
      return 'Quota refresh only supports local auth files.'
    case 'unsupported':
      return 'Quota refresh is not supported for this credential type.'
    case 'not_found':
      return 'This credential is no longer available.'
    case 'invalid':
      return 'This credential cannot be refreshed.'
  }
  return error || 'Quota refresh failed. Please try again later.'
}

function quotaErrorMessage(error: unknown): string {
  if (error instanceof Error) {
    return error.message
  }
  return 'Quota request failed'
}
