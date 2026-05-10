import { useMemo } from 'react'
import {
  CREDENTIALS_PAGE_SIZE,
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
  pageSize: number
  authFilePage: number
  aiProviderPage: number
  authFileTotalPages: number
  aiProviderTotalPages: number
  setAuthFilePage: (page: number) => void
  setAiProviderPage: (page: number) => void
  loading: boolean
  error: string
  quotaRefreshing: boolean
  quotaRefreshError: string
  refresh: () => Promise<void>
  refreshQuotaForCurrentAuthFilePage: () => Promise<void>
  refreshQuotaForAuthIndex: (authIndex: string) => Promise<void>
}

export function useCredentialsTabData({ enabled, onAuthRequired }: UseCredentialsTabDataOptions): CredentialsTabData {
  const credentialPages = useCredentialPages({ enabled, onAuthRequired })
  const currentAuthIndexes = useMemo(
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

  return {
    authFileRows,
    aiProviderRows,
    authFileTotal: credentialPages.authFileTotal,
    aiProviderTotal: credentialPages.aiProviderTotal,
    pageSize: CREDENTIALS_PAGE_SIZE,
    authFilePage: credentialPages.authFilePage,
    aiProviderPage: credentialPages.aiProviderPage,
    authFileTotalPages: credentialPages.authFileTotalPages,
    aiProviderTotalPages: credentialPages.aiProviderTotalPages,
    setAuthFilePage: credentialPages.setAuthFilePage,
    setAiProviderPage: credentialPages.setAiProviderPage,
    loading: credentialPages.loading,
    error: credentialPages.error,
    quotaRefreshing: quotaRefreshTasks.quotaRefreshing,
    quotaRefreshError: quotaRefreshTasks.quotaRefreshError,
    refresh: credentialPages.refresh,
    refreshQuotaForCurrentAuthFilePage: quotaRefreshTasks.refreshQuotaForCurrentAuthFilePage,
    refreshQuotaForAuthIndex: quotaRefreshTasks.refreshQuotaForAuthIndex,
  }
}

export { quotaRefreshDisplayError }
