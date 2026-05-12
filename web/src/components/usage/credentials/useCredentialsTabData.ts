import { useCallback, useMemo, useState } from 'react'
import { ApiError, cleanupBannedAuthFiles } from '@/lib/api'
import type { AccountGuardCleanupResponse } from '@/lib/types'
import {
  buildAiProviderCredentialRows,
  buildAuthFileCredentialRows,
  selectQuotaEligibleAuthIndexes,
  type AiProviderCredentialRow,
  type AuthFileCredentialRow,
} from './credentialViewModels'
import { useCredentialPages } from './useCredentialPages'
import { useQuotaCache } from './useQuotaCache'
import { quotaRefreshDisplayError, useQuotaRefreshTasks } from './useQuotaRefreshTasks'

interface UseCredentialsTabDataOptions {
  enabled: boolean
  onAuthRequired?: () => void
}

export interface CredentialsTabData {
  authFileRows: AuthFileCredentialRow[]
  aiProviderRows: AiProviderCredentialRow[]
  authFileTotal: number
  aiProviderTotal: number
  authFilePageSize: number
  aiProviderPageSize: number
  authFilePage: number
  aiProviderPage: number
  authFileTotalPages: number
  aiProviderTotalPages: number
  setAuthFilePage: (page: number) => void
  setAiProviderPage: (page: number) => void
  setAuthFilePageSize: (pageSize: number) => void
  setAiProviderPageSize: (pageSize: number) => void
  loading: boolean
  error: string
  quotaRefreshing: boolean
  quotaRefreshError: string
  cleanupLoading: boolean
  cleanupError: string
  cleanupResult?: AccountGuardCleanupResponse
  refresh: () => Promise<void>
  refreshQuotaForCurrentAuthFilePage: () => Promise<void>
  refreshQuotaForAuthIndex: (authIndex: string) => Promise<void>
  cleanupBannedAuthFiles: (dryRun?: boolean) => Promise<AccountGuardCleanupResponse | undefined>
  clearCleanupResult: () => void
}

export function useCredentialsTabData({ enabled, onAuthRequired }: UseCredentialsTabDataOptions): CredentialsTabData {
  // 页面 hook 只编排分页、缓存和刷新任务三层数据，不直接发散 API 调用。
  const credentialPages = useCredentialPages({ enabled, onAuthRequired })
  const [cleanupLoading, setCleanupLoading] = useState(false)
  const [cleanupError, setCleanupError] = useState('')
  const [cleanupResult, setCleanupResult] = useState<AccountGuardCleanupResponse | undefined>()
  const currentAuthIndexes = useMemo(
    // quota 只对当前 Auth Files 页生效，AI Provider 不参与缓存读取和刷新。
    () => selectQuotaEligibleAuthIndexes(credentialPages.authFileIdentities),
    [credentialPages.authFileIdentities],
  )
  const { quotaByAuthIndex, setQuotaByAuthIndex } = useQuotaCache({
    enabled,
    authIndexes: currentAuthIndexes,
    onAuthRequired,
  })
  const quotaRefreshTasks = useQuotaRefreshTasks({
    enabled,
    currentAuthIndexes,
    setQuotaByAuthIndex,
    onAuthRequired,
  })

  // 把对象状态转成 Map 后交给纯 view model，组件层只消费已组合好的行数据。
  const quotaRowsByAuthIndex = useMemo(() => new Map(Object.entries(quotaByAuthIndex)), [quotaByAuthIndex])
  const quotaStates = useMemo(() => new Map(Object.entries(quotaRefreshTasks.quotaStateByAuthIndex).map(([authIndex, state]) => [authIndex, {
    quotaLoading: state.loading ?? false,
    quotaError: state.error,
    refreshTaskId: state.refreshTaskId,
    refreshStatus: state.refreshStatus,
  }])), [quotaRefreshTasks.quotaStateByAuthIndex])

  const authFileRows = useMemo(
    () => buildAuthFileCredentialRows(credentialPages.authFileIdentities, quotaRowsByAuthIndex, quotaStates),
    [credentialPages.authFileIdentities, quotaRowsByAuthIndex, quotaStates],
  )
  const aiProviderRows = useMemo(
    () => buildAiProviderCredentialRows(credentialPages.aiProviderIdentities),
    [credentialPages.aiProviderIdentities],
  )

  const runCleanupBannedAuthFiles = useCallback(async (dryRun = true) => {
    setCleanupLoading(true)
    setCleanupError('')
    try {
      const result = await cleanupBannedAuthFiles(dryRun)
      setCleanupResult(result)
      if (!dryRun) {
        await credentialPages.refresh()
      }
      return result
    } catch (error) {
      if (error instanceof ApiError && error.status === 401) {
        onAuthRequired?.()
        return undefined
      }
      const message = error instanceof Error ? error.message : 'Failed to cleanup banned auth files'
      setCleanupError(message)
      return undefined
    } finally {
      setCleanupLoading(false)
    }
  }, [credentialPages.refresh, onAuthRequired])

  const clearCleanupResult = useCallback(() => {
    setCleanupResult(undefined)
    setCleanupError('')
  }, [])

  return {
    authFileRows,
    aiProviderRows,
    authFileTotal: credentialPages.authFileTotal,
    aiProviderTotal: credentialPages.aiProviderTotal,
    authFilePageSize: credentialPages.authFilePageSize,
    aiProviderPageSize: credentialPages.aiProviderPageSize,
    authFilePage: credentialPages.authFilePage,
    aiProviderPage: credentialPages.aiProviderPage,
    authFileTotalPages: credentialPages.authFileTotalPages,
    aiProviderTotalPages: credentialPages.aiProviderTotalPages,
    setAuthFilePage: credentialPages.setAuthFilePage,
    setAiProviderPage: credentialPages.setAiProviderPage,
    setAuthFilePageSize: credentialPages.setAuthFilePageSize,
    setAiProviderPageSize: credentialPages.setAiProviderPageSize,
    loading: credentialPages.loading,
    error: credentialPages.error,
    quotaRefreshing: quotaRefreshTasks.quotaRefreshing,
    quotaRefreshError: quotaRefreshTasks.quotaRefreshError,
    cleanupLoading,
    cleanupError,
    cleanupResult,
    refresh: credentialPages.refresh,
    refreshQuotaForCurrentAuthFilePage: quotaRefreshTasks.refreshQuotaForCurrentAuthFilePage,
    refreshQuotaForAuthIndex: quotaRefreshTasks.refreshQuotaForAuthIndex,
    cleanupBannedAuthFiles: runCleanupBannedAuthFiles,
    clearCleanupResult,
  }
}

export { quotaRefreshDisplayError }
