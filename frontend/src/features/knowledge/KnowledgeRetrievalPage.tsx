import {
  Alert,
  Button,
  Collapse,
  Pagination,
  Select,
  Form,
  Input,
  InputNumber,
  Switch,
  Tag,
  Empty,
  Spin,
  Space,
  Typography,
  Tooltip,
} from "antd";
import { useEffect, useRef, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { isAxiosError } from "axios";
import { Link } from "react-router";
import { parseApiError } from "@/api/client";
import type { MetadataFilter, RetrievalInput } from "@/api/knowledge";
import { ChunkImage } from "./KnowledgeChunksPage";
import { useTranslation } from "react-i18next";
import { MagnifyingGlassIcon } from "@phosphor-icons/react";
import { createStyles } from "antd-style";
import { useParams } from "react-router";
import PrimaryButton from "@/components/PrimaryButton";
import { useCanWrite } from "@/hooks/useCanWrite";
import { useRetrievalTest } from "@/queries/useKnowledge";
import { useMultiragModels } from "@/queries/useMultirag";
import RetrievalDocumentSelect from "./RetrievalDocumentSelect";
import RetrievalMetadataBuilder from "./RetrievalMetadataBuilder";
import {
  metadataFilterFromJSON,
  retrievalMetadataKey,
  loadRetrievalMetadataKeys,
} from "./retrievalFilter";
export { metadataFilterFromJSON } from "./retrievalFilter";
import "@/i18n/locales/retrievalWorkbench";
import { tokens as tk } from "@/styles/tokens";

const useStyles = createStyles(({ css }) => ({
  workbench: css`
    container: retrieval / inline-size;
    min-width: 0;
  `,
  form: css`
    background: ${tk.surface};
    border-radius: ${tk.radius}px;
    border: 1px solid var(--border);
    padding: 20px 24px;
    margin: 8px 0 20px;
    @container retrieval (max-width: 640px) {
      padding: 16px;
    }
  `,
  scope: css`
    padding: 12px 0;
    border-block: 1px solid var(--border);
    margin-bottom: 16px;
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
    align-items: center;
    color: ${tk.textSecondary};
  `,
  filters: css`
    display: grid;
    grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
    gap: 20px;
    @container retrieval (max-width: 840px) {
      grid-template-columns: minmax(0, 1fr);
      gap: 0;
    }
  `,
  metadata: css`
    margin-top: 12px;
    padding-top: 8px;
    border-top: 1px solid var(--border);
    display: flex;
    gap: 8px 16px;
    flex-wrap: wrap;
    font-size: ${tk.textSm};
    color: ${tk.textSecondary};
  `,
  params: css`
    display: flex;
    gap: 16px;
    flex-wrap: wrap;
    align-items: flex-end;
  `,
  resultHead: css`
    font-size: ${tk.textSm};
    color: ${tk.textTertiary};
    margin-bottom: 12px;
  `,
  card: css`
    background: ${tk.surface};
    border-radius: ${tk.radius}px;
    padding: 16px 18px;
    max-width: 100%;
    margin-bottom: 12px;
    border: 1px solid var(--border);
    overflow-wrap: anywhere;
  `,
  cardMeta: css`
    display: flex;
    align-items: center;
    gap: 10px;
    margin-bottom: 8px;
    flex-wrap: wrap;
  `,
  docName: css`
    font-size: ${tk.textSm};
    font-weight: 600;
    color: ${tk.text};
  `,
  content: css`
    font-size: ${tk.textSm};
    color: ${tk.textSecondary};
    line-height: 1.6;
    white-space: pre-wrap;
  `,
  sourceLink: css`
    display: inline-block;
    margin-top: 12px;
    color: ${tk.ink};
    &:hover {
      text-decoration: underline;
    }
    &:focus-visible {
      outline: 2px solid var(--ring);
      outline-offset: 3px;
    }
  `,
  loadingWrap: css`
    display: flex;
    justify-content: center;
    padding: 60px 0;
  `,
}));

interface RetrievalFormValues {
  reference_metadata: boolean;
  document_ids: string[];
  metadata_condition: string;
  search_type: "default" | "sparse" | "dense" | "hybrid" | "fusion";
  fusion_sparse_weight: number;
  fusion_dense_weight: number;
  rerank_id?: string;
  keyword: boolean;
  use_kg: boolean;
  cross_languages: string[];
  question: string;
  top_k: number;
  similarity_threshold: number;
  vector_similarity_weight: number;
  highlight: boolean;
}

const DEFAULTS: RetrievalFormValues = {
  reference_metadata: true,
  document_ids: [],
  metadata_condition: "",
  search_type: "default",
  fusion_sparse_weight: 0.05,
  fusion_dense_weight: 0.95,
  rerank_id: undefined,
  keyword: false,
  use_kg: false,
  cross_languages: [],
  question: "",
  top_k: 1024,
  similarity_threshold: 0.2,
  vector_similarity_weight: 0.3,
  highlight: false,
};

export default function KnowledgeRetrievalPage() {
  const { id = "" } = useParams();
  return <RetrievalWorkbench key={id} id={id} />;
}

function RetrievalWorkbench({ id }: { id: string }) {
  const { t } = useTranslation();
  const { t: rw } = useTranslation("retrievalWorkbench");
  const { styles } = useStyles();
  const [form] = Form.useForm<RetrievalFormValues>();
  const retrieval = useRetrievalTest();
  const queryClient = useQueryClient();
  const [validating, setValidating] = useState(false);
  const active = useRef(true);
  useEffect(() => {
    active.current = true;
    return () => {
      active.current = false;
    };
  }, []);
  const canWrite = useCanWrite();
  const models = useMultiragModels("rerank");
  const [dirty, setDirty] = useState(false);
  const [lastRequest, setLastRequest] = useState<RetrievalInput | null>(null);
  const [page, setPage] = useState(1);
  const [formError, setFormError] = useState("");
  const selectedDocuments = Form.useWatch("document_ids", form) as
    | string[]
    | undefined;
  const conditionDraft = Form.useWatch("metadata_condition", form) as
    | string
    | undefined;
  const graphEnabled = Form.useWatch("use_kg", form) as boolean | undefined;
  const searchType = Form.useWatch("search_type", form) as string | undefined;
  let draftFilter: MetadataFilter | undefined;
  let draftInvalid = false;
  try {
    draftFilter = metadataFilterFromJSON(
      conditionDraft ?? "",
      queryClient.getQueryData<string[]>(retrievalMetadataKey(id)),
    );
  } catch {
    draftInvalid = true;
  }
  const hasDraftFilter = draftFilter !== undefined;

  const handleFinish = async (submitted: RetrievalFormValues) => {
    if (retrieval.isPending || validating) return;
    const values = { ...DEFAULTS, ...submitted };
    try {
      let metadata = metadataFilterFromJSON(values.metadata_condition);
      if (values.use_kg && (values.document_ids.length > 0 || metadata)) {
        setFormError(t("knowledge.manage.graphScopeConflict"));
        return;
      }
      if (metadata?.method === "semi_auto") {
        setValidating(true);
        let keys: string[];
        try {
          keys = await queryClient.fetchQuery({
            queryKey: retrievalMetadataKey(id),
            queryFn: () => loadRetrievalMetadataKeys(id),
            staleTime: 0,
            retry: false,
          });
        } catch {
          throw new Error("metadataDirectoryRequired");
        }
        if (!active.current) return;
        metadata = metadataFilterFromJSON(values.metadata_condition, keys);
      }
      if (
        values.rerank_id &&
        models.data &&
        !models.data.some(
          (model) =>
            model.fullId === values.rerank_id &&
            model.status === "1" &&
            model.type === "rerank",
        )
      ) {
        setFormError(rw("rerankUnavailable"));
        return;
      }
      const request: RetrievalInput = {
        question: values.question.trim(),
        dataset_ids: [id],
        ...(values.document_ids.length ? { doc_ids: values.document_ids } : {}),
        top_k: values.top_k,
        similarity_threshold: values.similarity_threshold,
        vector_similarity_weight: values.vector_similarity_weight,
        highlight: values.highlight,
        rerank_id:
          values.rerank_id?.trim() === ""
            ? undefined
            : values.rerank_id?.trim(),
        keyword: values.keyword,
        use_kg: values.use_kg,
        cross_languages: values.cross_languages,
        meta_data_filter: metadata,
        reference_metadata: { include: values.reference_metadata },
        page: 1,
        size: 30,
      };
      if (values.search_type !== "default") {
        request.search_mode = { type: values.search_type };
        if (values.search_type === "hybrid")
          Object.assign(request.search_mode, {
            weight_dense: values.vector_similarity_weight,
            weight_sparse: 1 - values.vector_similarity_weight,
          });
        if (values.search_type === "fusion") {
          const weights = [
            values.fusion_sparse_weight,
            values.fusion_dense_weight,
          ];
          if (
            weights.length !== 2 ||
            weights.some((v) => !Number.isFinite(v) || v < 0) ||
            weights.reduce((sum, v) => sum + v, 0) <= 0
          )
            throw new Error(rw("fusionInvalid"));
          request.search_mode.weights = weights.join(",");
        }
      }
      setFormError("");
      setDirty(false);
      setPage(1);
      setLastRequest(request);
      retrieval.mutate(request);
    } catch (error) {
      if (!active.current) return;
      setFormError(
        error instanceof Error &&
          [
            "unknownMetadataFields",
            "metadataDirectoryRequired",
            "invalidMetadataList",
            "unsupportedMetadataList",
            "unsupportedMetadataExclusion",
          ].includes(error.message)
          ? rw(error.message)
          : error instanceof Error && error.message === rw("fusionInvalid")
            ? error.message
            : rw("filterDraftInvalid"),
      );
    } finally {
      if (active.current) setValidating(false);
    }
  };

  const requestedDocumentIds = lastRequest?.doc_ids ?? [];
  const scopeMismatch = Boolean(
    lastRequest &&
    retrieval.data &&
    (retrieval.data.chunks.some(
      (chunk) =>
        (chunk.dataset_id !== undefined &&
          !lastRequest.dataset_ids.includes(chunk.dataset_id)) ||
        (requestedDocumentIds.length > 0 &&
          !requestedDocumentIds.includes(chunk.document_id)),
    ) ||
      (requestedDocumentIds.length > 0 &&
        retrieval.data.doc_aggs.some(
          (doc) => !requestedDocumentIds.includes(doc.doc_id),
        ))),
  );
  const result = retrieval.error || scopeMismatch ? undefined : retrieval.data;
  const errorCode = isAxiosError<{ code?: string }>(retrieval.error)
    ? retrieval.error.response?.data.code
    : undefined;
  const retrievalError =
    errorCode === "knowledge_scope_filter_conflict"
      ? t("knowledge.manage.scopeFilterConflict")
      : errorCode === "knowledge_graph_scope_conflict"
        ? t("knowledge.manage.graphScopeConflict")
        : parseApiError(retrieval.error);

  return (
    <div className={styles.workbench}>
      {formError && <Alert type="error" showIcon title={formError} />}
      {scopeMismatch && !retrieval.isPending && (
        <Alert type="error" showIcon title={rw("scopeMismatch")} />
      )}
      {retrieval.error && (
        <Alert
          type="error"
          showIcon
          title={retrievalError}
          description={t("knowledge.manage.retrievalRetryHint")}
          action={
            canWrite && lastRequest ? (
              <Button
                onClick={() => {
                  retrieval.mutate({ ...lastRequest, page });
                }}
              >
                {rw("retry")}
              </Button>
            ) : undefined
          }
        />
      )}
      <Typography.Paragraph type="secondary">
        {rw("intro")}
      </Typography.Paragraph>
      <Form
        form={form}
        disabled={validating}
        layout="vertical"
        className={styles.form}
        initialValues={DEFAULTS}
        onFinish={handleFinish}
        onValuesChange={() => {
          setDirty(Boolean(lastRequest));
          setFormError("");
        }}
      >
        <Form.Item
          label={t("knowledge.retrieval.question")}
          name="question"
          rules={[
            {
              required: true,
              whitespace: true,
              message: t("knowledge.retrieval.questionRequired"),
            },
          ]}
        >
          <Input.TextArea
            rows={2}
            placeholder={t("knowledge.retrieval.questionPh")}
            onKeyDown={(event) => {
              if (
                (event.metaKey || event.ctrlKey) &&
                event.key === "Enter" &&
                canWrite &&
                !retrieval.isPending &&
                !validating
              ) {
                event.preventDefault();
                form.submit();
              }
            }}
          />
        </Form.Item>
        {canWrite && (
          <Form.Item style={{ marginBottom: 12 }}>
            <PrimaryButton
              htmlType="submit"
              icon={<MagnifyingGlassIcon size={16} />}
              loading={retrieval.isPending || validating}
            >
              {t("knowledge.retrieval.testBtn")}
            </PrimaryButton>
          </Form.Item>
        )}
        <Typography.Title level={5}>{rw("scope")}</Typography.Title>
        <div className={styles.filters}>
          <Form.Item
            name="document_ids"
            label={t("knowledge.manage.documentScope")}
          >
            <RetrievalDocumentSelect datasetId={id} />
          </Form.Item>
          <Form.Item name="metadata_condition" label={rw("metadata")}>
            <RetrievalMetadataBuilder datasetId={id} />
          </Form.Item>
        </div>
        <div className={styles.scope} aria-live="polite">
          <Typography.Text strong>
            {t("knowledge.manage.currentScope")}
          </Typography.Text>
          <Tag>
            {selectedDocuments?.length
              ? t("knowledge.manage.selectedDocumentScope", {
                  n: selectedDocuments.length,
                })
              : t("knowledge.manage.allDocuments")}
          </Tag>
          <Tag>
            {draftInvalid
              ? t("knowledge.manage.invalidFilterDraft")
              : hasDraftFilter
                ? t("knowledge.manage.metadataFilterActive")
                : t("knowledge.manage.noMetadataFilter")}
          </Tag>
        </div>
        {selectedDocuments?.length && hasDraftFilter ? (
          <Alert
            style={{ marginBottom: 16 }}
            type="info"
            showIcon
            title={t("knowledge.manage.intersectionPending")}
          />
        ) : null}
        <Collapse
          style={{ marginBottom: 12 }}
          items={[
            {
              key: "advanced",
              label: rw("strategy"),
              children: (
                <>
                  <Form.Item
                    name="search_type"
                    label={t("knowledge.manage.searchMode")}
                    extra={rw(`strategyHelp.${searchType ?? "default"}`)}
                  >
                    <Select
                      options={[
                        "default",
                        "sparse",
                        "dense",
                        "hybrid",
                        "fusion",
                      ].map((value) => ({
                        value,
                        label: rw(`strategies.${value}`),
                      }))}
                    />
                  </Form.Item>
                  {searchType === "fusion" && (
                    <div className={styles.params}>
                      <Form.Item
                        name="fusion_sparse_weight"
                        label={rw("fusionKeywordWeight")}
                        rules={[{ required: true, type: "number", min: 0 }]}
                      >
                        <InputNumber min={0} step={0.05} />
                      </Form.Item>
                      <Form.Item
                        name="fusion_dense_weight"
                        label={rw("fusionSemanticWeight")}
                        rules={[{ required: true, type: "number", min: 0 }]}
                      >
                        <InputNumber min={0} step={0.05} />
                      </Form.Item>
                    </div>
                  )}
                  <Form.Item
                    name="rerank_id"
                    label={t("knowledge.manage.rerankModel")}
                    extra={models.error ? rw("rerankError") : rw("rerankHint")}
                  >
                    <Select
                      allowClear
                      showSearch={{ optionFilterProp: "label" }}
                      loading={models.isFetching}
                      placeholder={rw("rerankNone")}
                      notFoundContent={
                        models.isPending ? (
                          <Spin size="small" />
                        ) : (
                          rw("rerankEmpty")
                        )
                      }
                      options={(models.data ?? [])
                        .filter(
                          (model) =>
                            model.type === "rerank" && model.status === "1",
                        )
                        .map((model) => ({
                          value: model.fullId,
                          label: `${model.name} (${model.factory})`,
                        }))}
                    />
                  </Form.Item>
                  {models.error && (
                    <Button
                      onClick={() => {
                        void models.refetch();
                      }}
                    >
                      {rw("retry")}
                    </Button>
                  )}
                  <Form.Item
                    name="cross_languages"
                    label={t("knowledge.manage.crossLanguages")}
                  >
                    <Select
                      mode="tags"
                      options={[
                        "Chinese",
                        "English",
                        "Japanese",
                        "Korean",
                        "French",
                        "German",
                      ].map((value) => ({
                        value,
                        label: value,
                      }))}
                    />
                  </Form.Item>

                  <Form.Item
                    name="keyword"
                    label={t("knowledge.manage.keywordExpansion")}
                    valuePropName="checked"
                  >
                    <Switch />
                  </Form.Item>
                  <Form.Item
                    name="use_kg"
                    label={t("knowledge.manage.useGraph")}
                    extra={t("knowledge.manage.graphScopeHint")}
                    valuePropName="checked"
                  >
                    <Switch
                      disabled={
                        !graphEnabled &&
                        ((selectedDocuments?.length ?? 0) > 0 ||
                          hasDraftFilter ||
                          draftInvalid)
                      }
                    />
                  </Form.Item>
                </>
              ),
            },
          ]}
        />
        <Collapse
          style={{ marginBottom: 16 }}
          items={[
            {
              key: "presentation",
              label: rw("presentation"),
              children: (
                <div className={styles.params}>
                  <Form.Item
                    name="reference_metadata"
                    label={t("knowledge.manage.includeReferenceMetadata")}
                    valuePropName="checked"
                  >
                    <Switch />
                  </Form.Item>
                  <Form.Item
                    label={t("knowledge.retrieval.highlight")}
                    name="highlight"
                    valuePropName="checked"
                  >
                    <Switch />
                  </Form.Item>
                </div>
              ),
            },
          ]}
        />
        <Typography.Title level={5}>{rw("quality")}</Typography.Title>
        <div className={styles.params}>
          <Form.Item
            label={rw("candidates")}
            tooltip={rw("candidatesHint")}
            name="top_k"
            style={{ marginBottom: 0 }}
            rules={[{ required: true, type: "number", min: 1, max: 2048 }]}
          >
            <InputNumber
              min={1}
              max={2048}
              precision={0}
              style={{ width: 120 }}
            />
          </Form.Item>
          <Form.Item
            label={t("knowledge.retrieval.threshold")}
            name="similarity_threshold"
            extra={
              selectedDocuments?.length
                ? rw("selectedDocumentThresholdHint")
                : undefined
            }
            rules={[{ required: true, type: "number", min: 0, max: 1 }]}
            style={{ marginBottom: 0 }}
          >
            <InputNumber min={0} max={1} step={0.05} style={{ width: 120 }} />
          </Form.Item>
          <Form.Item
            label={t("knowledge.retrieval.vectorWeight")}
            name="vector_similarity_weight"
            tooltip={rw("weightHint")}
            rules={[{ required: true, type: "number", min: 0, max: 1 }]}
            style={{ marginBottom: 0 }}
          >
            <InputNumber min={0} max={1} step={0.05} style={{ width: 140 }} />
          </Form.Item>
        </div>
      </Form>

      <section aria-label={rw("evidence")}>
        <Typography.Title level={4}>{rw("evidence")}</Typography.Title>
        {lastRequest && (
          <div className={styles.resultHead}>
            <Typography.Text strong>
              {rw("testedQuestion")}: {lastRequest.question}
            </Typography.Text>
            <details className={styles.metadata}>
              <summary>{rw("applied")}</summary>
              <div>{rw("noExpansion")}</div>
              <pre style={{ whiteSpace: "pre-wrap", overflowWrap: "anywhere" }}>
                {JSON.stringify(
                  {
                    doc_ids: lastRequest.doc_ids ?? null,
                    meta_data_filter: lastRequest.meta_data_filter ?? null,
                    search_mode: lastRequest.search_mode ?? "default",
                    rerank_id: lastRequest.rerank_id ?? null,
                  },
                  null,
                  2,
                )}
              </pre>
            </details>
            {dirty && (
              <Alert
                style={{ marginTop: 8 }}
                type="info"
                showIcon
                title={rw("changed")}
              />
            )}
          </div>
        )}
        {retrieval.isPending ? (
          <div className={styles.loadingWrap}>
            <Space orientation="vertical" align="center">
              <Spin />
              <Typography.Text type="secondary">
                {rw("running")}
              </Typography.Text>
            </Space>
          </div>
        ) : result ? (
          result.chunks.length > 0 ? (
            <div>
              <div className={styles.resultHead}>
                {t("knowledge.retrieval.recalled", { n: result.total })}
                {lastRequest && (
                  <span>
                    {" "}
                    ·{" "}
                    {lastRequest.doc_ids?.length
                      ? t("knowledge.manage.selectedDocumentScope", {
                          n: lastRequest.doc_ids.length,
                        })
                      : t("knowledge.manage.allDocuments")}{" "}
                    ·{" "}
                    {t(
                      lastRequest.meta_data_filter
                        ? "knowledge.manage.metadataFilterActive"
                        : "knowledge.manage.noMetadataFilter",
                    )}
                  </span>
                )}
              </div>
              {result.reference_metadata && (
                <details className={styles.metadata}>
                  <summary>{t("knowledge.manage.referenceMetadata")}</summary>
                  <pre
                    style={{ whiteSpace: "pre-wrap", overflowWrap: "anywhere" }}
                  >
                    {JSON.stringify(result.reference_metadata, null, 2)}
                  </pre>
                </details>
              )}
              {result.doc_aggs.length > 0 && (
                <div>
                  {result.doc_aggs.map((doc) => (
                    <Tag key={doc.doc_id}>
                      {doc.doc_name}: {doc.count}
                    </Tag>
                  ))}
                </div>
              )}
              {result.chunks.map((chunk) => (
                <div key={chunk.id} className={styles.card}>
                  <div className={styles.cardMeta}>
                    <Tag color="blue">
                      {t("knowledge.retrieval.similarity", {
                        v: chunk.similarity.toFixed(3),
                      })}
                    </Tag>
                    <Tooltip title={t("knowledge.retrieval.simTooltip")}>
                      <Tag>
                        {t("knowledge.retrieval.vectorTerm", {
                          v: chunk.vector_similarity.toFixed(2),
                          t: chunk.term_similarity.toFixed(2),
                        })}
                      </Tag>
                    </Tooltip>
                    <span className={styles.docName}>
                      {chunk.document_name ||
                        t("knowledge.retrieval.unknownDoc")}
                    </span>
                  </div>
                  <div className={styles.content}>
                    {chunk.highlight ? (
                      <HighlightedContent content={chunk.highlight} />
                    ) : (
                      chunk.content
                    )}
                  </div>
                  {Object.keys(chunk.document_metadata ?? {}).length > 0 && (
                    <div
                      className={styles.metadata}
                      aria-label={t("knowledge.manage.documentMetadata")}
                    >
                      {Object.entries(chunk.document_metadata ?? {}).map(
                        ([key, value]) => (
                          <span key={key}>
                            <strong>{key}</strong>:{" "}
                            {typeof value === "string"
                              ? value
                              : JSON.stringify(value)}
                          </span>
                        ),
                      )}
                    </div>
                  )}
                  {Object.keys(chunk.reference_metadata ?? {}).length > 0 && (
                    <Typography.Paragraph type="secondary">
                      {t("knowledge.manage.referenceMetadata")}:{" "}
                      {JSON.stringify(chunk.reference_metadata)}
                    </Typography.Paragraph>
                  )}
                  {chunk.image_id && (
                    <ChunkImage
                      datasetId={chunk.dataset_id ?? id}
                      imageId={chunk.image_id}
                    />
                  )}
                  <details className={styles.metadata}>
                    <summary>{rw("citation")}</summary>
                    <div>
                      {rw("chunkId")}: {chunk.id}
                    </div>
                    {chunk.positions?.length ? (
                      <div>
                        {rw("positions")}: {JSON.stringify(chunk.positions)}
                      </div>
                    ) : null}
                    <div>
                      {t("knowledge.manage.documentScope")}: {chunk.document_id}
                    </div>
                  </details>
                  <Link
                    className={styles.sourceLink}
                    target="_blank"
                    rel="noopener noreferrer"
                    to={`/knowledge/${encodeURIComponent(chunk.dataset_id ?? id)}/documents/${encodeURIComponent(chunk.document_id)}/chunks?chunkId=${encodeURIComponent(chunk.id)}`}
                  >
                    {rw("sourceNewTab")}
                  </Link>
                </div>
              ))}
              {canWrite && lastRequest && (
                <Pagination
                  current={page}
                  pageSize={30}
                  total={result.total}
                  showSizeChanger={false}
                  disabled={retrieval.isPending}
                  onChange={(next) => {
                    setPage(next);
                    retrieval.mutate({ ...lastRequest, page: next });
                  }}
                />
              )}
            </div>
          ) : (
            <Empty
              description={
                <>
                  <div>{t("knowledge.retrieval.emptyNoResult")}</div>
                  <Typography.Text type="secondary">
                    {t("knowledge.manage.emptyScopeHint")}
                    <br />
                    {rw("emptyHint")}
                  </Typography.Text>
                </>
              }
            />
          )
        ) : retrieval.error || scopeMismatch ? null : (
          <Empty description={rw("notRun")} />
        )}
      </section>
    </div>
  );
}

// Treat upstream markup as text and retain only emphasis. No HTML attributes,
// images, links or scripts are passed into React's DOM.
export function HighlightedContent({ content }: { content: string }) {
  const template = document.createElement("template");
  template.innerHTML = content;
  const read = (node: ChildNode, key: string): React.ReactNode => {
    if (node.nodeType === Node.TEXT_NODE) return node.textContent;
    if (node.nodeType !== Node.ELEMENT_NODE) return null;
    const element = node as HTMLElement;
    if (["SCRIPT", "STYLE"].includes(element.tagName)) return null;
    const children = Array.from(element.childNodes).map((child, index) =>
      read(child, `${key}-${index}`),
    );
    if (["EM", "MARK", "B", "STRONG"].includes(element.tagName))
      return <mark key={key}>{children}</mark>;
    if (element.tagName === "BR") return <br key={key} />;
    return <span key={key}>{children}</span>;
  };
  return (
    <>
      {Array.from(template.content.childNodes).map((node, index) =>
        read(node, String(index)),
      )}
    </>
  );
}
