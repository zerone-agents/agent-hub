import { useEffect, useMemo } from "react";
import { useTranslation } from 'react-i18next'
import { Form, Button, Spin } from "antd";
import { createStyles } from "antd-style";
import { useParams } from "react-router";
import { useKnowledgeDetail, useUpdateKnowledge } from "@/queries/useKnowledge";
import { useMultiragModels } from "@/queries/useMultirag";
import { useProviders, useSyncProviderMultiRAG } from "@/queries/useProviders";
import { useCanWrite } from "@/hooks/useCanWrite";
import { parseApiError } from "@/api/client";
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
import { buildEmbeddingCandidates, decodeCandidateValue } from "./candidates";

const useStyles = createStyles(({ css }) => ({
  card: css`
    background: ${tk.surface};
    border-radius: ${tk.radius}px;
    box-shadow: ${tk.elevation1};
    padding: 24px 28px;
    max-width: 640px;
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

export default function KnowledgeSettingsPage() {
  const { t } = useTranslation()
  const { styles } = useStyles();
  const { id = "" } = useParams();
  const [form] = Form.useForm<DatasetFormValues>();
  const { data: dataset, isLoading, refetch } = useKnowledgeDetail(id);
  const updateKnowledge = useUpdateKnowledge();
  const syncProvider = useSyncProviderMultiRAG();
  const providers = useProviders();
  const multiragEmbedding = useMultiragModels("embedding");
  const canWrite = useCanWrite();
  const embeddingLocked = (dataset?.chunk_num ?? 0) > 0;

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
  }, [dataset?.embd_id, embdRawToValue, embeddingGroups]);
  const candidatesLoading = providers.isLoading || multiragEmbedding.isLoading;

  useEffect(() => {
    if (dataset) form.setFieldsValue(datasetToFormValues(dataset));
  }, [dataset, form]);

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
    let values: DatasetFormValues;
    try {
      values = await form.validateFields();
    } catch {
      return;
    }
    try {
      const selected = embeddingLocked
        ? null
        : decodeCandidateValue(values.embd_id);
      if (selected?.source === "local" && selected.providerId) {
        await syncProvider.mutateAsync({
          id: selected.providerId,
          verifyOnly: false,
          modelIds: [selected.rawValue],
        });
      }

      await updateKnowledge.mutateAsync({
        id,
        data: formValuesToInput(values, {
          includeEmbeddingModel: !embeddingLocked,
        }),
      });
    } catch (error) {
      const message = parseApiError(error);
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
    }
  };

  // eslint-disable-next-line @typescript-eslint/no-unnecessary-condition -- react-query data typed as T | undefined but lint infers dataset as always falsy
  if (isLoading && !dataset) {
    return (
      <div className={styles.loadingWrap}>
        <Spin />
      </div>
    );
  }

  return (
    <div className={styles.card}>
      <Form form={form} layout="vertical" requiredMark={false} disabled={!canWrite}>
        <DatasetFields
          embeddingOptions={embeddingOptions}
          embeddingLoading={candidatesLoading}
          embeddingLocked={embeddingLocked}
          embeddingChunkCount={dataset?.chunk_num ?? 0}
        />
        <div className={styles.foot}>
          {canWrite && (
            <Button
              type="primary"
              onClick={handleSave}
              loading={updateKnowledge.isPending}
            >
              {t('knowledge.settings.save')}
            </Button>
          )}
        </div>
      </Form>
    </div>
  );
}
