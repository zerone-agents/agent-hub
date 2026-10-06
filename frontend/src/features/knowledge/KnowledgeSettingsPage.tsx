import { useEffect, useMemo, useState, useRef } from "react";
import { useTranslation } from 'react-i18next'
import KnowledgeConnectorsPanel from "./KnowledgeConnectorsPanel";
import KnowledgeTagsPanel from "./KnowledgeTagsPanel";
import KnowledgeMetadataPanel from "./KnowledgeMetadataPanel";
import { Alert, Button, Form, Grid, Spin, Tabs, Typography } from "antd";
import { createStyles } from "antd-style";
import { Link, useParams } from "react-router";
import { useKnowledgeDetail, useUpdateKnowledge } from "@/queries/useKnowledge";
import { useMultiragModels } from "@/queries/useMultirag";
import { useProviders, useSyncProviderMultiRAG } from "@/queries/useProviders";
import { useCanWrite } from "@/hooks/useCanWrite";
import PrimaryButton from "@/components/PrimaryButton";
import type { KnowledgeDataset, KnowledgeWriteOwner } from "@/api/knowledge";
import { getAccessToken, parseApiError } from "@/api/client";
import { useAuthStore } from '@/stores/auth';
import { queryClient } from '@/lib/query-client';
import type { KnowledgeListOrigin } from '@/queries/useKnowledge';
import { tokens as tk } from "@/styles/tokens";
import {
  buildRawToValueMap,
  DatasetFields,
  datasetToFormValues,
  formValuesToInput,
  groupsToAntdOptions,
  type SelectOptionGroup,
  type DatasetFormValues,
} from "./KnowledgeForm";
import { buildEmbeddingCandidates, buildLayoutCandidates, decodeCandidateValue, isAllowedLayoutSelection, retainLayoutCandidates } from "./candidates";

const useStyles = createStyles(({ css }) => ({
  card: css`
    background: ${tk.surface};
    border-radius: ${tk.radius}px;
    box-shadow: ${tk.elevation1};
    padding: 24px 28px;
    max-width: 880px;
    @media (max-width: 600px) { padding: 16px; }
    margin-top: 8px;
  `,
  loadingWrap: css`
    display: flex;
    justify-content: center;
    padding: 60px 0;
  `,
  foot: css`
    margin-top: 8px;
  `,
}));

function KnowledgeBasicSettings() {
  const { t } = useTranslation()
  const { styles } = useStyles();
  const { id = "" } = useParams();
  const accountId = useAuthStore((state) => state.user?.id);
  const [form] = Form.useForm<DatasetFormValues>();
  const watchedLayout: unknown = Form.useWatch('layout_recognize', form);
  const selectedLayout = typeof watchedLayout === 'string' ? watchedLayout : undefined;
  const { data: dataset, origin, isLoading, isError, refetch } = useKnowledgeDetail(id, { owned: true });
  const operation = useRef<{ controller: AbortController; guard: KnowledgeWriteOwner } | null>(null);
  const formOrigin = useRef<KnowledgeListOrigin | undefined>(undefined);
  const getOperationOwner = () => operation.current?.guard;
  const updateKnowledge = useUpdateKnowledge({ getOwner: getOperationOwner });
  const syncProvider = useSyncProviderMultiRAG({ getOwner: getOperationOwner });
  const providers = useProviders();
  const multiragEmbedding = useMultiragModels("embedding");
  const multiragLayout = useMultiragModels("ocr");
  const multiragVision = useMultiragModels("image2text");
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState("");
  const hydrated = useRef("");
  const baseline = useRef<KnowledgeDataset | null>(null);
  const [confirmedDataset, setConfirmedDataset] = useState<KnowledgeDataset | null>(null);
  const canWrite = useCanWrite();
  const embeddingLocked = (dataset?.chunk_num ?? 0) > 0;
  const readAuthorized = Boolean(origin && origin.id === accountId && origin.token === getAccessToken());
  const reloadIdentity = () => { void queryClient.invalidateQueries({ queryKey: ['userinfo'] }); void refetch(); };

  useEffect(() => {
    const unsubscribeUser = useAuthStore.subscribe((next, previous) => {
      if (next.user?.id !== previous.user?.id || next.user?.role !== previous.user?.role) {
        formOrigin.current = undefined;
        operation.current?.controller.abort();
      }
    });
    const unsubscribeCache = queryClient.getQueryCache().subscribe((event) => {
      const key = event.query.queryKey as readonly unknown[];
      if (event.type === 'removed' && key[0] === 'userinfo') { formOrigin.current = undefined; operation.current?.controller.abort(); }
    });
    return () => { unsubscribeUser(); unsubscribeCache(); operation.current?.controller.abort(); };
  }, []);

  const embeddingGroups = useMemo(
    () =>
      buildEmbeddingCandidates(
        multiragEmbedding.data ?? [],
        providers.data ?? [],
      ),
    [multiragEmbedding.data, providers.data],
  );
  const embdRawToValue = useMemo(
    () => buildRawToValueMap(embeddingGroups),
    [embeddingGroups],
  );
  const embeddingOptions = useMemo<SelectOptionGroup[]>(() => {
    const saved = dataset?.embd_id;
    const options = groupsToAntdOptions(embeddingGroups);
    if (saved && !embdRawToValue.has(saved)) {
      return [
        {
          label: t('knowledge.settings.currentValue'),
          options: [
            {
              label: t('knowledge.settings.modelUnavailable', { saved }),
              value: saved,
            },
          ],
        },
        ...options,
      ];
    }
    return options;
  }, [t, dataset?.embd_id, embdRawToValue, embeddingGroups]);
  const layoutGroups = useMemo(() => buildLayoutCandidates([...(multiragLayout.isError ? [] : multiragLayout.data ?? []), ...(multiragVision.isError ? [] : multiragVision.data ?? [])]), [multiragLayout.data, multiragLayout.isError, multiragVision.data, multiragVision.isError]);
  const layoutRawToValue = useMemo(() => buildRawToValueMap(layoutGroups), [layoutGroups]);
  const layoutOptions = useMemo<SelectOptionGroup[]>(() => {
    const saved = (confirmedDataset?.parser_config ?? dataset?.parser_config)?.layout_recognize;
    return groupsToAntdOptions(retainLayoutCandidates(layoutGroups, typeof saved === 'string' ? saved : undefined, selectedLayout), t);
  }, [dataset, confirmedDataset, layoutGroups, selectedLayout, t]);
  const embeddingLoading = providers.isLoading || multiragEmbedding.isLoading;
  const layoutLoading = multiragLayout.isLoading || multiragVision.isLoading;


  useEffect(() => {
    if (!dataset || !origin || origin.id !== useAuthStore.getState().user?.id || origin.token !== getAccessToken()) return;
    if (hydrated.current !== id) {
      form.resetFields();
      form.setFieldsValue(datasetToFormValues(dataset));
      hydrated.current = id;
      baseline.current = dataset;
      setConfirmedDataset(dataset);
    }
    formOrigin.current = origin;
  }, [dataset, origin, form, id]);
  useEffect(() => {
    const current = form.getFieldValue("layout_recognize") as unknown;
    if (typeof current === "string" && layoutRawToValue.has(current)) form.setFieldValue("layout_recognize", layoutRawToValue.get(current));
  }, [dataset, form, layoutRawToValue]);

  useEffect(() => {
    if (!dataset || embeddingLocked) return;
    const current = form.getFieldValue("embd_id") as string;
    if (current === dataset.embd_id) {
      const mapped = embdRawToValue.get(current);
      if (mapped !== undefined) {
        form.setFieldValue("embd_id", mapped);
      }
    }
  }, [dataset, embeddingLocked, embdRawToValue, form]);

  const handleSave = async () => {
    if (isError || isLoading || !canWrite || saving || operation.current || hydrated.current !== id) return;
    const initiating = formOrigin.current;
    if (!initiating || !readAuthorized || initiating.id !== accountId || initiating.token !== getAccessToken() || !['admin', 'maintainer'].includes(initiating.role ?? '')) { setSaveError(t('knowledge.form.ownerChanged')); return; }
    const controller = new AbortController();
    let token = initiating.token;
    const current = () => !controller.signal.aborted && useAuthStore.getState().user?.id === initiating.id && ['admin', 'maintainer'].includes(useAuthStore.getState().user?.role ?? '');
    const guard: KnowledgeWriteOwner = {
      signal: controller.signal,
      isCurrent: () => current() && token === getAccessToken(),
      assertCurrent: (refresh) => {
        if (refresh && current()) token = getAccessToken();
        if (!current() || token !== getAccessToken()) throw new Error(t('knowledge.form.ownerChanged'));
      },
    };
    const active = { controller, guard };
    operation.current = active;
    setSaveError("");
    setSaving(true);
    try {
      guard.assertCurrent();
      const validated = await form.validateFields();
      guard.assertCurrent();
      const values = { ...(form.getFieldsValue(true) as DatasetFormValues), ...validated };
      if (!isAllowedLayoutSelection(values.layout_recognize, layoutGroups, (baseline.current ?? dataset)?.parser_config.layout_recognize as string | undefined)) {
        form.setFields([{ name: 'layout_recognize', errors: [t('knowledge.form.parseLayoutUnavailable')] }]);
        return;
      }
      const selected = embeddingLocked ? null : decodeCandidateValue(values.embd_id);
      const targets = new Map<number, Set<string>>();
      for (const candidate of [selected]) {
        if (candidate?.source === "local" && candidate.providerId) {
          const models = targets.get(candidate.providerId) ?? new Set<string>();
          models.add(candidate.rawValue); targets.set(candidate.providerId, models);
        }
      }
      for (const [providerId, models] of targets) {
        guard.assertCurrent();
        await syncProvider.mutateAsync({ id: providerId, verifyOnly: false, modelIds: [...models] });
        guard.assertCurrent();
      }

      guard.assertCurrent();
      const saved = await updateKnowledge.mutateAsync({
        id,
        data: formValuesToInput(values, {
          includeEmbeddingModel: !embeddingLocked,
          original: baseline.current ?? undefined,
        }),
      });
      guard.assertCurrent();
      baseline.current = saved;
      setConfirmedDataset(saved);
      // Fields remain disabled throughout the operation. Rebase both visible
      // controls and advanced JSON onto the independently confirmed response,
      // including extensions the server changed during this save.
      form.resetFields();
      form.setFieldsValue(datasetToFormValues(saved));
    } catch (error) {
      if (!guard.isCurrent()) { if (useAuthStore.getState().user?.id === initiating.id) setSaveError(t('knowledge.form.ownerChanged')); return; }
      if (typeof error === 'object' && error !== null && 'errorFields' in error) return;
      const message = parseApiError(error);
      setSaveError(message);
      if (
        message.includes("chunk_num") &&
        message.includes("embedding_model")
      ) {
        form.setFields([
          {
            name: "embd_id",
            errors: [
              t('knowledge.settings.locked'),
            ],
          },
        ]);
        await refetch();
      }
    } finally { if (operation.current === active) { operation.current = null; setSaving(false); } }
  };

  if (!dataset) {
    if (isLoading) return <div className={styles.loadingWrap} role="status" aria-label={t('common.loading')}><Spin /></div>;
    return <Alert type="error" showIcon title={t('knowledge.states.settingsFailed')} description={t('knowledge.states.settingsFailedHint')} action={<Button onClick={() => { void refetch() }}>{t('knowledge.states.retry')}</Button>} />;
  }
  if (!readAuthorized) return <Alert type="warning" showIcon title={t('knowledge.form.ownerChanged')} action={<Button onClick={reloadIdentity}>{t('knowledge.states.retry')}</Button>} />;

  return (
    <div className={styles.card}>
      {isError ? <Alert type="warning" showIcon title={t('knowledge.states.settingsStale')} description={t('knowledge.states.settingsStaleHint')} action={<Button onClick={() => { void refetch() }}>{t('knowledge.states.retry')}</Button>} style={{ marginBottom: 16 }} /> : null}
      {saveError ? <Alert type="error" showIcon title={saveError} action={<Button onClick={() => { void refetch() }}>{t('knowledge.states.retry')}</Button>} style={{ marginBottom: 16 }} /> : null}
      {providers.isError || multiragEmbedding.isError || multiragLayout.isError || multiragVision.isError ? <Alert type="warning" showIcon title={t('knowledge.form.candidatesFailed')} description={t('knowledge.form.candidatesFailedHint')} action={<Button onClick={() => { void providers.refetch(); void multiragEmbedding.refetch(); void multiragLayout.refetch(); void multiragVision.refetch(); }}>{t('knowledge.states.retry')}</Button>} style={{ marginBottom: 16 }} /> : null}
      <Typography.Paragraph type="secondary">{t('knowledge.settings.scopeHint')}</Typography.Paragraph>
      <Form form={form} layout="vertical" requiredMark={false} disabled={!canWrite || saving || isError}>
        <DatasetFields
          savedParser={dataset.parser_id}
          preserveParserMode={typeof dataset.pipeline_id === "string" && Boolean(dataset.pipeline_id)}
          embeddingOptions={embeddingOptions}
          embeddingLoading={embeddingLoading}
          embeddingLocked={embeddingLocked}
          embeddingChunkCount={dataset.chunk_num}
          layoutOptions={layoutOptions}
          layoutLoading={layoutLoading}
          layoutGroups={layoutGroups}
        />
        <div className={styles.foot}>
          {canWrite && (
            <PrimaryButton
              onClick={handleSave}
              disabled={isError}
              loading={saving || updateKnowledge.isPending}
            >
              {t('knowledge.settings.save')}
            </PrimaryButton>
          )}
        </div>
      </Form>
    </div>
  );
}

export default function KnowledgeSettingsPage() {
  const { t } = useTranslation()
  const { id = "" } = useParams()
  const canWrite = useCanWrite()
  const accountId = useAuthStore((state) => state.user?.id)
  const screens = Grid.useBreakpoint()
  return <><Typography.Paragraph type="secondary"><Link to={`/knowledge/${id}/ingestions`}>{t('knowledge.tabs.ingestions')}</Link></Typography.Paragraph><Tabs tabPlacement={screens.md ? "start" : "top"} items={[
    { key: 'basic', label: t('knowledge.manage.basicSettings'), children: <KnowledgeBasicSettings key={`${id}:${accountId ?? ''}`} /> },
    { key: 'metadata', label: t('knowledge.manage.metadataManagement'), children: <KnowledgeMetadataPanel id={id} /> },
    { key: 'tags', label: t('knowledge.manage.tagManagement'), children: <KnowledgeTagsPanel id={id} /> },
    ...(canWrite ? [{ key: 'connectors', label: t('knowledge.manage.connectors'), children: <KnowledgeConnectorsPanel id={id} /> }] : []),
  ]} /></>
}
