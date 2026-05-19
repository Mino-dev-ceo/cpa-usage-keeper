import { useState, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { Card } from '@/components/ui/Card';
import { formatCompactNumber, formatUsd, type ApiStats } from '@/utils/usage';
import styles from '@/pages/UsagePage.module.scss';

function ApiDetailsTitle({ title, subtitle, eyebrow }: { title: string; subtitle: string; eyebrow: string }) {
  return (
    <div className={styles.sectionTitleBlock}>
      <span className={styles.sectionEyebrow}>{eyebrow}</span>
      <h3 className={styles.sectionTitle}>{title}</h3>
      <p className={styles.sectionSubtitle}>{subtitle}</p>
    </div>
  );
}

export interface ApiDetailsCardProps {
  apiStats: ApiStats[];
  loading: boolean;
  hasPrices: boolean;
  onSaveNote?: (apiKey: string, note: string) => Promise<void>;
  onClearUsage?: (apiKey: string) => Promise<void>;
  onClearAllUsage?: () => Promise<void>;
}

type ApiSortKey = 'endpoint' | 'requests' | 'tokens' | 'cost';
type SortDir = 'asc' | 'desc';

export function ApiDetailsCard({ apiStats, loading, hasPrices, onSaveNote, onClearUsage, onClearAllUsage }: ApiDetailsCardProps) {
  const { t } = useTranslation();
  const [expandedApis, setExpandedApis] = useState<Set<string>>(new Set());
  const [sortKey, setSortKey] = useState<ApiSortKey>('requests');
  const [sortDir, setSortDir] = useState<SortDir>('desc');
  const [editingEndpoint, setEditingEndpoint] = useState<string | null>(null);
  const [draftNote, setDraftNote] = useState('');
  const [savingEndpoint, setSavingEndpoint] = useState<string | null>(null);
  const [clearingEndpoint, setClearingEndpoint] = useState<string | null>(null);
  const [noteError, setNoteError] = useState<string | null>(null);
  const [clearError, setClearError] = useState<string | null>(null);

  const toggleExpand = (endpoint: string) => {
    setExpandedApis((prev) => {
      const newSet = new Set(prev);
      if (newSet.has(endpoint)) {
        newSet.delete(endpoint);
      } else {
        newSet.add(endpoint);
      }
      return newSet;
    });
  };

  const handleSort = (key: ApiSortKey) => {
    if (sortKey === key) {
      setSortDir((d) => (d === 'asc' ? 'desc' : 'asc'));
    } else {
      setSortKey(key);
      setSortDir(key === 'endpoint' ? 'asc' : 'desc');
    }
  };

  const beginEditNote = (api: ApiStats) => {
    setEditingEndpoint(api.endpoint);
    setDraftNote(api.note ?? '');
    setNoteError(null);
  };

  const cancelEditNote = () => {
    setEditingEndpoint(null);
    setDraftNote('');
    setNoteError(null);
  };

  const saveNote = async (endpoint: string) => {
    if (!onSaveNote) return;
    setSavingEndpoint(endpoint);
    setNoteError(null);
    try {
      await onSaveNote(endpoint, draftNote);
      setEditingEndpoint(null);
      setDraftNote('');
    } catch (error) {
      setNoteError(error instanceof Error ? error.message : t('usage_stats.api_note_save_failed'));
    } finally {
      setSavingEndpoint(null);
    }
  };

  const clearUsageForApi = async (api: ApiStats) => {
    if (!onClearUsage) return;
    if (!window.confirm(t('usage_stats.api_usage_clear_confirm', { name: api.displayName }))) return;
    setClearingEndpoint(api.endpoint);
    setClearError(null);
    try {
      await onClearUsage(api.endpoint);
    } catch (error) {
      setClearError(error instanceof Error ? error.message : t('usage_stats.api_usage_clear_failed'));
    } finally {
      setClearingEndpoint(null);
    }
  };

  const clearAllUsage = async () => {
    if (!onClearAllUsage) return;
    if (!window.confirm(t('usage_stats.api_usage_clear_all_confirm'))) return;
    setClearingEndpoint('__all__');
    setClearError(null);
    try {
      await onClearAllUsage();
    } catch (error) {
      setClearError(error instanceof Error ? error.message : t('usage_stats.api_usage_clear_failed'));
    } finally {
      setClearingEndpoint(null);
    }
  };

  const sorted = useMemo(() => {
    const list = [...apiStats];
    const dir = sortDir === 'asc' ? 1 : -1;
    list.sort((a, b) => {
      switch (sortKey) {
        case 'endpoint': return dir * a.displayName.localeCompare(b.displayName);
        case 'requests': return dir * (a.totalRequests - b.totalRequests);
        case 'tokens': return dir * (a.totalTokens - b.totalTokens);
        case 'cost': return dir * (a.totalCost - b.totalCost);
        default: return 0;
      }
    });
    return list;
  }, [apiStats, sortKey, sortDir]);

  const arrow = (key: ApiSortKey) =>
    sortKey === key ? (sortDir === 'asc' ? ' ▲' : ' ▼') : '';

  return (
    <Card
      title={
        <ApiDetailsTitle
          eyebrow={t('usage_stats.api_details_eyebrow')}
          title={t('usage_stats.api_details_title')}
          subtitle={t('usage_stats.api_details_subtitle')}
        />
      }
      className={styles.detailsFixedCard}
    >
      {loading ? (
        <div className={styles.hint}>{t('common.loading')}</div>
      ) : sorted.length > 0 ? (
        <>
          <div className={styles.apiSortBar}>
            {([
              ['endpoint', 'usage_stats.api_endpoint'],
              ['requests', 'usage_stats.requests_count'],
              ['tokens', 'usage_stats.tokens_count'],
              ...(hasPrices ? [['cost', 'usage_stats.total_cost']] : []),
            ] as [ApiSortKey, string][]).map(([key, labelKey]) => (
              <button
                key={key}
                type="button"
                aria-pressed={sortKey === key}
                className={`${styles.apiSortBtn} ${sortKey === key ? styles.apiSortBtnActive : ''}`}
                onClick={() => handleSort(key)}
              >
                {t(labelKey)}{arrow(key)}
              </button>
            ))}
            {onClearAllUsage && (
              <button
                type="button"
                className={`${styles.apiSortBtn} ${styles.apiClearAllButton}`}
                onClick={() => { void clearAllUsage(); }}
                disabled={clearingEndpoint !== null}
              >
                {clearingEndpoint === '__all__' ? t('common.loading') : t('usage_stats.api_usage_clear_all')}
              </button>
            )}
          </div>
          {clearError && <div className={styles.apiNoteError}>{clearError}</div>}
          <div className={styles.detailsScroll}>
            <div className={styles.apiList}>
              {sorted.map((api, index) => {
                const isExpanded = expandedApis.has(api.endpoint);
                const isEditing = editingEndpoint === api.endpoint;
                const isSaving = savingEndpoint === api.endpoint;
                const isClearing = clearingEndpoint === api.endpoint;
                const panelId = `api-models-${index}`;

                return (
                  <div key={api.endpoint} className={styles.apiItem}>
                    <div className={styles.apiHeader}>
                      <button
                        type="button"
                        className={styles.apiHeaderMain}
                        onClick={() => toggleExpand(api.endpoint)}
                        aria-expanded={isExpanded}
                        aria-controls={panelId}
                      >
                        <div className={styles.apiInfo}>
                          <span className={styles.apiEndpoint}>{api.displayName}</span>
                          {api.note && <span className={styles.apiKeyAlias}>{api.keyDisplayName}</span>}
                          <div className={styles.apiStats}>
                            <span className={styles.apiBadge}>
                              <span className={styles.requestCountCell}>
                                <span>
                                  {t('usage_stats.requests_count')}: {api.totalRequests.toLocaleString()}
                                </span>
                                <span className={styles.requestBreakdown}>
                                  (<span className={styles.statSuccess}>{api.successCount.toLocaleString()}</span>{' '}
                                  <span className={styles.statFailure}>{api.failureCount.toLocaleString()}</span>)
                                </span>
                              </span>
                            </span>
                            <span className={styles.apiBadge}>
                              {t('usage_stats.tokens_count')}: {formatCompactNumber(api.totalTokens)}
                            </span>
                            {hasPrices && api.totalCost > 0 && (
                              <span className={styles.apiBadge}>
                                {t('usage_stats.total_cost')}: {formatUsd(api.totalCost)}
                              </span>
                            )}
                          </div>
                        </div>
                        <span className={styles.expandIcon}>
                          {isExpanded ? '▼' : '▶'}
                        </span>
                      </button>
                      {onSaveNote && (
                        <button
                          type="button"
                          className={styles.apiNoteButton}
                          onClick={() => beginEditNote(api)}
                          disabled={isSaving}
                        >
                          {api.note ? t('usage_stats.api_note_edit') : t('usage_stats.api_note_add')}
                        </button>
                      )}
                      {onClearUsage && (
                        <button
                          type="button"
                          className={`${styles.apiNoteButton} ${styles.apiClearButton}`}
                          onClick={() => { void clearUsageForApi(api); }}
                          disabled={isClearing || clearingEndpoint !== null}
                        >
                          {isClearing ? t('common.loading') : t('usage_stats.api_usage_clear')}
                        </button>
                      )}
                    </div>
                    {isEditing && (
                      <form
                        className={styles.apiNoteEditor}
                        onSubmit={(event) => {
                          event.preventDefault();
                          void saveNote(api.endpoint);
                        }}
                      >
                        <label className={styles.apiNoteLabel}>
                          <span>{t('usage_stats.api_note_label')}</span>
                          <input
                            value={draftNote}
                            maxLength={80}
                            onChange={(event) => setDraftNote(event.target.value)}
                            placeholder={t('usage_stats.api_note_placeholder')}
                            disabled={isSaving}
                          />
                        </label>
                        <div className={styles.apiNoteActions}>
                          <button type="submit" className={styles.apiNoteSaveButton} disabled={isSaving}>
                            {isSaving ? t('common.loading') : t('usage_stats.api_note_save')}
                          </button>
                          <button type="button" className={styles.apiNoteGhostButton} onClick={cancelEditNote} disabled={isSaving}>
                            {t('usage_stats.api_note_cancel')}
                          </button>
                          {api.note && (
                            <button
                              type="button"
                              className={styles.apiNoteGhostButton}
                              onClick={() => {
                                setDraftNote('');
                                void saveNote(api.endpoint);
                              }}
                              disabled={isSaving}
                            >
                              {t('usage_stats.api_note_clear')}
                            </button>
                          )}
                        </div>
                        {noteError && <div className={styles.apiNoteError}>{noteError}</div>}
                      </form>
                    )}
                    {isExpanded && (
                      <div id={panelId} className={styles.apiModels}>
                        {Object.entries(api.models).map(([model, stats]) => (
                          <div key={model} className={styles.modelRow}>
                            <span className={styles.modelName}>{model}</span>
                            <span className={styles.modelStat}>
                              <span className={styles.requestCountCell}>
                                <span>{stats.requests.toLocaleString()}</span>
                                <span className={styles.requestBreakdown}>
                                  (<span className={styles.statSuccess}>{stats.successCount.toLocaleString()}</span>{' '}
                                  <span className={styles.statFailure}>{stats.failureCount.toLocaleString()}</span>)
                                </span>
                              </span>
                            </span>
                            <span className={styles.modelStat}>{formatCompactNumber(stats.tokens)}</span>
                          </div>
                        ))}
                      </div>
                    )}
                  </div>
                );
              })}
            </div>
          </div>
        </>
      ) : (
        <div className={styles.hint}>{t('usage_stats.no_data')}</div>
      )}
    </Card>
  );
}
