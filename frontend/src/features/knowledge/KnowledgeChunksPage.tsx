import { useEffect, useMemo, useState } from "react";
import { useTranslation } from 'react-i18next'
// 组件外纯函数：直调 i18next
import i18next from '@/i18n'
import {
  Badge,
  Button,
  Checkbox,
  Descriptions,
  Divider,
  Drawer,
  Empty,
  Form,
  Image,
  Input,
  Pagination,
  Popconfirm,
  Segmented,
  Select,
  Space,
  Spin,
  Switch,
  Tag,
  Tooltip,
  Typography,
  Upload,
  message,
} from "antd";
import {
  ArrowLeftIcon,
  ArrowsClockwiseIcon,
  ClipboardTextIcon,
  FileTextIcon,
  ImageSquareIcon,
  PencilSimpleIcon,
  PlusIcon,
  TrashIcon,
} from "@phosphor-icons/react";
import { createStyles } from "antd-style";
import { useNavigate, useParams } from "react-router";
import { parseApiError } from "@/api/client";
import { copyOrManual } from "@/utils/clipboard";
import {
  knowledgeApi,
  type ChunkFormInput,
  type KnowledgeChunk,
} from "@/api/knowledge";
import {
  useChunks,
  useCreateChunk,
  useUpdateChunk,
  useDeleteChunks,
  useSwitchChunks,
} from "@/queries/useKnowledge";
import { useCanWrite } from "@/hooks/useCanWrite";
import PrimaryButton from "@/components/PrimaryButton";
import { tokens as t } from "@/styles/tokens";

const useStyles = createStyles(({ css }) => ({
  shell: css`
    display: grid;
    grid-template-columns: minmax(0, 1fr) 300px;
    gap: 16px;

    @media (max-width: 1100px) {
      grid-template-columns: 1fr;
    }
  `,
  main: css`
    min-width: 0;
  `,
  back: css`
    display: inline-flex;
    align-items: center;
    gap: 6px;
    margin-bottom: 10px;
    padding: 0;
    border: none;
    background: none;
    color: ${t.textTertiary};
    cursor: pointer;
    font-size: ${t.textSm};

    &:hover {
      color: ${t.ink};
    }
  `,
  toolbar: css`
    display: flex;
    justify-content: space-between;
    align-items: flex-start;
    gap: 12px;
    margin-bottom: 12px;

    @media (max-width: 980px) {
      flex-direction: column;
      align-items: stretch;
    }
  `,
  filters: css`
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
    align-items: center;
  `,
  actions: css`
    display: flex;
    flex-wrap: wrap;
    justify-content: flex-end;
    gap: 8px;
  `,
  bulkBar: css`
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 10px;
    margin-bottom: 12px;
    padding: 10px 12px;
    border: 1px solid color-mix(in srgb, var(--foreground) 12%, transparent);
    border-radius: ${t.radius}px;
    background: linear-gradient(
      90deg,
      color-mix(in srgb, var(--foreground) 6%, transparent),
      rgba(5, 150, 105, 0.06)
    );

    @media (max-width: 760px) {
      flex-direction: column;
      align-items: stretch;
    }
  `,
  list: css`
    display: flex;
    flex-direction: column;
    gap: 10px;
  `,
  card: css`
    display: grid;
    grid-template-columns: auto minmax(0, 1fr) auto;
    gap: 12px;
    padding: 14px;
    border: 1px solid color-mix(in srgb, var(--foreground) 10%, transparent);
    border-radius: ${t.radius}px;
    background: ${t.surface};
    box-shadow: ${t.elevation1};

    @media (max-width: 760px) {
      grid-template-columns: auto minmax(0, 1fr);
    }
  `,
  cardBody: css`
    min-width: 0;
  `,
  cardMeta: css`
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
    align-items: center;
    margin-bottom: 8px;
  `,
  content: css`
    color: ${t.text};
    font-size: ${t.textBase};
    line-height: 1.65;
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  `,
  clipped: css`
    display: -webkit-box;
    overflow: hidden;
    -webkit-box-orient: vertical;
    -webkit-line-clamp: 4;
  `,
  tags: css`
    display: flex;
    flex-wrap: wrap;
    gap: 4px;
    margin-top: 10px;
  `,
  thumbnail: css`
    margin-bottom: 10px;
  `,
  imageFrame: css`
    position: relative;
    display: flex;
    align-items: center;
    justify-content: center;
    overflow: hidden;
    border: 1px solid color-mix(in srgb, var(--foreground) 12%, transparent);
    border-radius: ${t.radiusSm}px;
    background: ${t.inkSubtle};
  `,
  imageLoading: css`
    position: absolute;
    inset: 0;
    z-index: 1;
    display: flex;
    align-items: center;
    justify-content: center;
    color: ${t.textTertiary};
    background: ${t.inkSubtle};
  `,
  imageFallback: css`
    display: flex;
    width: 100%;
    height: 100%;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 4px;
    padding: 8px;
    color: ${t.textTertiary};
    text-align: center;
    font-size: ${t.textXs};
  `,
  imageFallbackActions: css`
    display: flex;
    gap: 4px;
  `,
  cardActions: css`
    display: flex;
    flex-direction: column;
    align-items: flex-end;
    gap: 8px;

    @media (max-width: 760px) {
      grid-column: 1 / -1;
      flex-direction: row;
      justify-content: flex-end;
    }
  `,
  sidePanel: css`
    position: sticky;
    top: 12px;
    align-self: start;
    padding: 14px;
    border: 1px solid color-mix(in srgb, var(--foreground) 10%, transparent);
    border-radius: ${t.radius}px;
    background: ${t.surface};
    box-shadow: ${t.elevation1};
  `,
  panelTitle: css`
    display: flex;
    align-items: center;
    gap: 8px;
    margin-bottom: 12px;
    color: ${t.text};
    font-weight: 700;
  `,
  pager: css`
    display: flex;
    justify-content: flex-end;
    margin-top: 14px;
  `,
  preview: css`
    min-height: 130px;
    padding: 12px;
    border: 1px solid color-mix(in srgb, var(--foreground) 10%, transparent);
    border-radius: ${t.radiusSm}px;
    background: ${t.inkSubtle};
    white-space: pre-wrap;
  `,
}));

const PAGE_SIZE = 10;
const ALLOWED_INLINE_TAGS = new Set(["EM", "STRONG", "B", "I", "BR"]);

interface ChunkFormValues {
  content: string;
  important_keywords: string[];
  questions: string[];
  tag_kwd: string[];
  tag_feas_text: string;
}

interface ImageFileReadResult {
  dataUrl: string;
  base64: string;
}

function hasHtmlTags(value: string): boolean {
  return /<\/?[a-z][\s\S]*>/i.test(value);
}

function escapeHtml(value: string): string {
  return value
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#39;");
}

function sanitizeAllowedInlineHtml(value: string): string {
  if (!hasHtmlTags(value) || typeof document === "undefined")
    return escapeHtml(value);
  const template = document.createElement("template");
  template.innerHTML = value;

  const serialize = (node: ChildNode): string => {
    if (node.nodeType === Node.TEXT_NODE)
      return escapeHtml(node.textContent ?? "");
    if (node.nodeType !== Node.ELEMENT_NODE) return "";
    const element = node as HTMLElement;
    if (!ALLOWED_INLINE_TAGS.has(element.tagName))
      return escapeHtml(element.textContent);
    if (element.tagName === "BR") return "<br>";
    const children = Array.from(element.childNodes).map(serialize).join("");
    const tag = element.tagName.toLowerCase();
    return `<${tag}>${children}</${tag}>`;
  };

  return Array.from(template.content.childNodes).map(serialize).join("");
}

function SafeContent({
  content,
  clipped,
}: {
  content: string;
  clipped: boolean;
}) {
  const { styles, cx } = useStyles();
  const html = sanitizeAllowedInlineHtml(content || i18next.t('knowledge.chunks.emptyContent'));
  return (
    <div
      className={cx(styles.content, clipped ? styles.clipped : undefined)}
      dangerouslySetInnerHTML={{ __html: html }}
    />
  );
}

function formatPositions(chunk: KnowledgeChunk): string {
  const positions = chunk.positions;
  if (positions.length === 0) return "-";
  const first = positions[0];
  if (Array.isArray(first)) return i18next.t('knowledge.chunks.position', { list: first.slice(0, 3).join(", ") });
  return i18next.t('knowledge.chunks.position', { list: positions.slice(0, 3).map(String).join(", ") });
}

function tagFeasText(chunk: KnowledgeChunk | null): string {
  if (!chunk || Object.keys(chunk.tag_feas).length === 0) return "";
  return JSON.stringify(chunk.tag_feas, null, 2);
}

function fileToBase64(file: File): Promise<ImageFileReadResult> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => {
      const raw = reader.result;
      const dataUrl = typeof raw === "string" ? raw : "";
      const marker = "base64,";
      const markerIndex = dataUrl.indexOf(marker);
      const base64 =
        markerIndex >= 0 ? dataUrl.slice(markerIndex + marker.length) : dataUrl;
      resolve({ dataUrl, base64 });
    };
    reader.onerror = () => { reject(reader.error instanceof Error ? reader.error : new Error(String(reader.error))); };
    reader.readAsDataURL(file);
  });
}

function ChunkImage({
  datasetId,
  imageId,
  width = 92,
  height = 70,
  className,
}: {
  datasetId: string;
  imageId: string;
  width?: number;
  height?: number;
  className?: string;
}) {
  const { styles, cx } = useStyles();
  const [retryKey, setRetryKey] = useState(0);
  const [status, setStatus] = useState<"loading" | "loaded" | "failed">(
    "loading",
  );
  const [imageSrc, setImageSrc] = useState("");
  const [errorMessage, setErrorMessage] = useState("");

  useEffect(() => {
    const controller = new AbortController();
    let active = true;
    let objectURL = "";

    // eslint-disable-next-line react-hooks/set-state-in-effect -- reset fetch-lifecycle status before kicking off a new image fetch; coupled to the request below
    setStatus("loading");
     
    setErrorMessage("");
     
    setImageSrc("");

    knowledgeApi.images
      .fetch(datasetId, imageId, controller.signal)
      .then((blob) => {
        if (!active) return;
        if (blob.type && !blob.type.startsWith("image/")) {
          throw new Error(i18next.t('knowledge.chunks.imgProxyError'));
        }
        objectURL = URL.createObjectURL(blob);
        setImageSrc(objectURL);
      })
      .catch((error: unknown) => {
        if (!active || controller.signal.aborted) return;
        setErrorMessage(parseApiError(error));
        setStatus("failed");
      });

    return () => {
      active = false;
      controller.abort();
      if (objectURL) URL.revokeObjectURL(objectURL);
    };
  }, [datasetId, imageId, retryKey]);

  const retry = () => {
    setStatus("loading");
    setRetryKey((current) => current + 1);
  };

  const copyImageId = async () => {
    const result = await copyOrManual(imageId);
    if (result === "copied") {
      message.success(i18next.t('knowledge.chunks.copiedImageId'));
    } else if (result === "failed") {
      message.error(i18next.t('knowledge.chunks.copyFail'));
    }
  };

  return (
    <div className={cx(styles.imageFrame, className)} style={{ width, height }}>
      {status === "failed" ? (
        <div className={styles.imageFallback}>
          <ImageSquareIcon size={18} />
          <span>{i18next.t('knowledge.chunks.imgLoadFail')}</span>
          <Typography.Text type="secondary" style={{ fontSize: 11 }}>
            ID {imageId.slice(0, 8)}
          </Typography.Text>
          {errorMessage ? (
            <Typography.Text type="secondary" style={{ fontSize: 11 }}>
              {errorMessage}
            </Typography.Text>
          ) : null}
          <div className={styles.imageFallbackActions}>
            <Button size="small" type="link" onClick={retry}>
              {i18next.t('knowledge.chunks.retry')}
            </Button>
            <Button size="small" type="link" onClick={() => void copyImageId()}>
              {i18next.t('knowledge.chunks.copyId')}
            </Button>
          </div>
        </div>
      ) : (
        <>
          {status === "loading" ? (
            <div className={styles.imageLoading}>
              <Spin size="small" />
            </div>
          ) : null}
          {imageSrc ? (
            <Image
              key={imageSrc}
              src={imageSrc}
              alt="chunk image"
              preview={{ src: imageSrc }}
              styles={{ root: { width: "100%", height: "100%" } }}
              style={{
                width: "100%",
                height: "100%",
                objectFit: "cover",
                opacity: status === "loaded" ? 1 : 0,
              }}
              onLoad={() => { setStatus("loaded"); }}
              onError={() => { setStatus("failed"); }}
            />
          ) : null}
        </>
      )}
    </div>
  );
}

interface ChunkEditorProps {
  open: boolean;
  editing: KnowledgeChunk | null;
  datasetId: string;
  documentId: string;
  onClose: () => void;
}

function ChunkEditor({
  open,
  editing,
  datasetId,
  documentId,
  onClose,
}: ChunkEditorProps) {
  const { t } = useTranslation()
  const { styles } = useStyles();
  const [form] = Form.useForm<ChunkFormValues>();
  const [imageBase64, setImageBase64] = useState("");
  const [imagePreviewUrl, setImagePreviewUrl] = useState("");
  const [previewMode, setPreviewMode] = useState<"edit" | "preview">("edit");
  const createChunk = useCreateChunk(datasetId, documentId);
  const updateChunk = useUpdateChunk(datasetId, documentId);
  const submitting = createChunk.isPending || updateChunk.isPending;

  useEffect(() => {
    if (!open) return;
    // eslint-disable-next-line react-hooks/set-state-in-effect -- reset form-local state on modal open; values are coupled to the antd form.setFieldsValue call below
    setImageBase64("");
     
    setImagePreviewUrl("");
     
    setPreviewMode("edit");
    form.setFieldsValue({
      content: editing?.content ?? "",
      important_keywords: editing?.important_keywords ?? [],
      questions: editing?.questions ?? [],
      tag_kwd: editing?.tag_kwd ?? [],
      tag_feas_text: tagFeasText(editing),
    });
  }, [open, editing, form]);

  const submit = async () => {
    let values: ChunkFormValues;
    try {
      values = await form.validateFields();
    } catch {
      return;
    }

    let tagFeas: Record<string, unknown> | undefined;
    const tagFeasTextValue = values.tag_feas_text.trim();
    if (tagFeasTextValue) {
      try {
        const parsed = JSON.parse(tagFeasTextValue) as unknown;
        if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
          message.error(i18next.t('knowledge.chunks.tagNotObject'));
          return;
        }
        tagFeas = parsed as Record<string, unknown>;
      } catch {
        message.error(i18next.t('knowledge.chunks.tagInvalid'));
        return;
      }
    }

    const input: ChunkFormInput = {
      content: values.content,
      important_keywords: values.important_keywords,
      questions: values.questions,
      tag_kwd: values.tag_kwd,
      tag_feas: tagFeas,
    };
    if (!editing && imageBase64) input.image_base64 = imageBase64;

    if (editing) {
      await updateChunk.mutateAsync({ chunkId: editing.id, input });
    } else {
      await createChunk.mutateAsync(input);
    }
    onClose();
  };

  const contentValue = Form.useWatch("content", form);

  return (
    <Drawer
      title={editing ? i18next.t('knowledge.chunks.editTitle') : i18next.t('knowledge.chunks.createTitle')}
      open={open}
      onClose={onClose}
      size={720}
      destroyOnHidden
      extra={
        <Space>
          <Button onClick={onClose}>{t('common.cancel')}</Button>
          <PrimaryButton
            loading={submitting}
            aria-label={editing ? i18next.t('knowledge.chunks.saveAria') : i18next.t('knowledge.chunks.createAria')}
            onClick={submit}
          >
            {editing ? i18next.t('knowledge.chunks.save') : i18next.t('knowledge.chunks.create')}
          </PrimaryButton>
        </Space>
      }
    >
      <Form form={form} layout="vertical" requiredMark={false}>
        <Space orientation="vertical" size={14} style={{ width: "100%" }}>
          <Segmented
            value={previewMode}
            onChange={(value) => { setPreviewMode(value as "edit" | "preview"); }}
            options={[
              { label: i18next.t('knowledge.chunks.modeEdit'), value: "edit" },
              { label: i18next.t('knowledge.chunks.modePreview'), value: "preview" },
            ]}
          />

          {previewMode === "edit" ? (
            <Form.Item
              label={i18next.t('knowledge.chunks.content')}
              name="content"
              rules={[{ required: true, message: i18next.t('knowledge.chunks.contentRequired') }]}
            >
              <Input.TextArea rows={10} placeholder={i18next.t('knowledge.chunks.contentPh')} />
            </Form.Item>
          ) : (
            <div className={styles.preview}>
              <SafeContent content={contentValue} clipped={false} />
            </div>
          )}

          <Form.Item label={i18next.t('knowledge.chunks.keywords')} name="important_keywords">
            <Select
              mode="tags"
              placeholder={i18next.t('knowledge.chunks.keywordsPh')}
              tokenSeparators={[","]}
            />
          </Form.Item>

          <Form.Item label={i18next.t('knowledge.chunks.questions')} name="questions">
            <Select
              mode="tags"
              placeholder={i18next.t('knowledge.chunks.questionsPh')}
              tokenSeparators={[","]}
            />
          </Form.Item>

          <Form.Item label={i18next.t('knowledge.chunks.tag')} name="tag_kwd">
            <Select
              mode="tags"
              placeholder={i18next.t('knowledge.chunks.tagPh')}
              tokenSeparators={[","]}
            />
          </Form.Item>

          <Form.Item label={i18next.t('knowledge.chunks.tagJson')} name="tag_feas_text">
            <Input.TextArea rows={4} placeholder={i18next.t('knowledge.chunks.tagJsonPh')} />
          </Form.Item>

          {editing?.image_id ? (
            <Space orientation="vertical">
              <Typography.Text strong>{i18next.t('knowledge.chunks.image')}</Typography.Text>
              <ChunkImage
                datasetId={datasetId}
                imageId={editing.image_id}
                width={180}
                height={120}
              />
              <Typography.Text type="secondary">
                i18next.t('knowledge.chunks.imgReadOnly')
              </Typography.Text>
            </Space>
          ) : null}

          {!editing ? (
            <Space orientation="vertical" size={8} style={{ width: "100%" }}>
              <Typography.Text strong>{i18next.t('knowledge.chunks.imgChunk')}</Typography.Text>
              <Upload
                accept="image/*"
                maxCount={1}
                showUploadList={false}
                beforeUpload={async (file) => {
                  const image = await fileToBase64(file);
                  setImageBase64(image.base64);
                  setImagePreviewUrl(image.dataUrl);
                  return Upload.LIST_IGNORE;
                }}
              >
                <Button icon={<ImageSquareIcon size={16} />}>{i18next.t('knowledge.chunks.selectImage')}</Button>
              </Upload>
              {imagePreviewUrl ? (
                <Image width={180} src={imagePreviewUrl} alt="preview image" />
              ) : null}
            </Space>
          ) : null}
        </Space>
      </Form>
    </Drawer>
  );
}

export default function KnowledgeChunksPage() {
  const { t } = useTranslation()
  const { styles } = useStyles();
  const navigate = useNavigate();
  const { id = "", documentId = "" } = useParams();

  const [page, setPage] = useState(1);
  const [keywords, setKeywords] = useState("");
  const [availableFilter, setAvailableFilter] = useState<
    "all" | "enabled" | "disabled"
  >("all");
  const [displayMode, setDisplayMode] = useState<"ellipsis" | "full">(
    "ellipsis",
  );
  const [selectedIds, setSelectedIds] = useState<string[]>([]);
  const [editorOpen, setEditorOpen] = useState(false);
  const [editing, setEditing] = useState<KnowledgeChunk | null>(null);

  const available =
    availableFilter === "all" ? undefined : availableFilter === "enabled";

  const query = useChunks(id, documentId, {
    page,
    page_size: PAGE_SIZE,
    keywords,
    available,
  });
  const deleteChunks = useDeleteChunks(id, documentId);
  const switchChunks = useSwitchChunks(id, documentId);
  const canWrite = useCanWrite();

  const chunks = useMemo(() => query.data?.chunks ?? [], [query.data?.chunks]);
  const total = query.data?.total ?? 0;
  const document = query.data?.document;
  const currentIds = useMemo(() => chunks.map((chunk) => chunk.id), [chunks]);
  const allCurrentSelected =
    currentIds.length > 0 &&
    currentIds.every((chunkId) => selectedIds.includes(chunkId));

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect -- drop stale selections whenever the page result changes; this is the canonical "filter derived state against new source" sync pattern
    setSelectedIds((current) =>
      current.filter((chunkId) => currentIds.includes(chunkId)),
    );
  }, [currentIds]);

  const toggleOne = (chunkId: string, checked: boolean) => {
    setSelectedIds((current) =>
      checked
        ? Array.from(new Set([...current, chunkId]))
        : current.filter((idValue) => idValue !== chunkId),
    );
  };

  const toggleAll = (checked: boolean) => {
    setSelectedIds(checked ? currentIds : []);
  };

  const openCreate = () => {
    setEditing(null);
    setEditorOpen(true);
  };

  const openEdit = (chunk: KnowledgeChunk) => {
    setEditing(chunk);
    setEditorOpen(true);
  };

  const bulkSwitch = (availableValue: boolean) => {
    switchChunks.mutate({ chunkIds: selectedIds, available: availableValue });
    message.success(availableValue ? t('knowledge.chunks.bulkEnabled') : t('knowledge.chunks.bulkDisabled'));
    setSelectedIds([]);
  };

  const bulkDelete = () => {
    deleteChunks.mutate(selectedIds);
    setSelectedIds([]);
  };

  const copyChunkId = async (chunkId: string) => {
    const result = await copyOrManual(chunkId);
    if (result === "copied") {
      message.success(t('knowledge.chunks.copiedChunkId'));
    } else if (result === "failed") {
      message.error(t('knowledge.chunks.copyFail'));
    }
  };

  return (
    <div>
      <button
        type="button"
        className={styles.back}
        onClick={async () => { await navigate(`/knowledge/${id}/documents`); }}
      >
        <ArrowLeftIcon size={14} />
        {t('knowledge.chunks.backToDocs')}
      </button>

      <div className={styles.shell}>
        <div className={styles.main}>
          <div className={styles.toolbar}>
            <div className={styles.filters}>
              <Input.Search
                placeholder={t('knowledge.chunks.searchPh')}
                allowClear
                style={{ width: 260 }}
                onSearch={(value) => {
                  setKeywords(value.trim());
                  setPage(1);
                }}
              />
              <Segmented
                value={displayMode}
                onChange={(value) =>
                  { setDisplayMode(value as "ellipsis" | "full"); }
                }
                options={[
                  { label: t('knowledge.chunks.ellipsis'), value: "ellipsis" },
                  { label: t('knowledge.chunks.full'), value: "full" },
                ]}
              />
              <Select
                value={availableFilter}
                style={{ width: 130 }}
                options={[
                  { label: t('knowledge.chunks.statusAll'), value: "all" },
                  { label: t('knowledge.chunks.statusEnabled'), value: "enabled" },
                  { label: t('knowledge.chunks.statusDisabled'), value: "disabled" },
                ]}
                onChange={(value) => {
                  setAvailableFilter(value);
                  setPage(1);
                }}
              />
              {canWrite && (
                <Checkbox
                  checked={allCurrentSelected}
                  indeterminate={selectedIds.length > 0 && !allCurrentSelected}
                  onChange={(event) => { toggleAll(event.target.checked); }}
                >
                  {t('knowledge.chunks.selectPage')}
                </Checkbox>
              )}
            </div>
            <div className={styles.actions}>
              <Button
                icon={<ArrowsClockwiseIcon size={16} />}
                loading={query.isFetching}
                onClick={() => query.refetch()}
              >
                {t('knowledge.chunks.refresh')}
              </Button>
              {canWrite && (
                <PrimaryButton
                  icon={<PlusIcon size={16} weight="bold" />}
                  onClick={openCreate}
                >
                  {t('knowledge.chunks.createChunk')}
                </PrimaryButton>
              )}
            </div>
          </div>

          {canWrite && selectedIds.length > 0 ? (
            <div className={styles.bulkBar}>
              <Typography.Text strong>
                {t('knowledge.chunks.selectedN', { n: selectedIds.length })}
              </Typography.Text>
              <Space wrap>
                <Button
                  size="small"
                  aria-label={t('knowledge.chunks.bulkEnableAria')}
                  onClick={() => { bulkSwitch(true); }}
                >
                  {t('knowledge.chunks.enable')}
                </Button>
                <Button
                  size="small"
                  aria-label={t('knowledge.chunks.bulkDisableAria')}
                  onClick={() => { bulkSwitch(false); }}
                >
                  {t('knowledge.chunks.disable')}
                </Button>
                <Popconfirm
                  title={t('knowledge.chunks.deleteSelectedTitle')}
                  description={t('knowledge.chunks.deleteSelectedDesc', { n: selectedIds.length })}
                  okText={t('common.delete')}
                  okButtonProps={{ danger: true }}
                  cancelText={t('common.cancel')}
                  onConfirm={bulkDelete}
                >
                  <Button size="small" danger icon={<TrashIcon size={14} />}>
                    {t('common.delete')}
                  </Button>
                </Popconfirm>
                <Button
                  size="small"
                  type="text"
                  onClick={() => { setSelectedIds([]); }}
                >
                  {t('knowledge.chunks.clearSelection')}
                </Button>
              </Space>
            </div>
          ) : null}

          <div className={styles.list}>
            {chunks.length === 0 && !query.isLoading ? (
              <Empty
                description={keywords ? t('knowledge.chunks.emptyNoMatch') : t('knowledge.chunks.emptyNone')}
              />
            ) : null}
            {chunks.map((chunk) => (
              <div className={styles.card} key={chunk.id}>
                {canWrite && (
                  <Checkbox
                    checked={selectedIds.includes(chunk.id)}
                    onChange={(event) =>
                      { toggleOne(chunk.id, event.target.checked); }
                    }
                  />
                )}
                <div className={styles.cardBody}>
                  <div className={styles.cardMeta}>
                    <Tag color={chunk.image_id ? "purple" : "blue"}>
                      {chunk.doc_type ?? (chunk.image_id ? "image" : "text")}
                    </Tag>
                    <Tag>{formatPositions(chunk)}</Tag>
                    <Badge
                      status={chunk.available ? "success" : "default"}
                      text={chunk.available ? t('knowledge.chunks.enable') : t('knowledge.chunks.disable')}
                    />
                    <Typography.Text type="secondary">
                      ID {chunk.id.slice(0, 8)}
                    </Typography.Text>
                  </div>

                  {chunk.image_id ? (
                    <ChunkImage
                      datasetId={id}
                      imageId={chunk.image_id}
                      className={styles.thumbnail}
                    />
                  ) : null}

                  <SafeContent
                    content={chunk.content}
                    clipped={displayMode === "ellipsis"}
                  />

                  <div className={styles.tags}>
                    {chunk.important_keywords.slice(0, 8).map((keyword) => (
                      <Tag key={`kw-${keyword}`}>{keyword}</Tag>
                    ))}
                    {chunk.questions.slice(0, 3).map((question) => (
                      <Tooltip key={`q-${question}`} title={question}>
                        <Tag color="green">Q</Tag>
                      </Tooltip>
                    ))}
                    {chunk.tag_kwd.slice(0, 5).map((tag) => (
                      <Tag key={`tag-${tag}`} color="gold">
                        {tag}
                      </Tag>
                    ))}
                  </div>
                </div>
                <div className={styles.cardActions}>
                  {canWrite && (
                    <>
                      <Switch
                        size="small"
                        checked={chunk.available}
                        onChange={(checked) =>
                          { switchChunks.mutate({
                            chunkIds: [chunk.id],
                            available: checked,
                          }); }
                        }
                      />
                      <Button
                        type="text"
                        size="small"
                        icon={<PencilSimpleIcon size={16} />}
                        onClick={() => { openEdit(chunk); }}
                      >
                        {t('common.edit')}
                      </Button>
                      <Popconfirm
                        title={t('knowledge.chunks.deleteTitle')}
                        okText={t('common.delete')}
                        okButtonProps={{ danger: true }}
                        cancelText={t('common.cancel')}
                        onConfirm={() => { deleteChunks.mutate([chunk.id]); }}
                      >
                        <Button
                          type="text"
                          size="small"
                          danger
                          icon={<TrashIcon size={16} />}
                        >
                          {t('common.delete')}
                        </Button>
                      </Popconfirm>
                    </>
                  )}
                  <Button
                    type="text"
                    size="small"
                    icon={<ClipboardTextIcon size={16} />}
                    onClick={() => void copyChunkId(chunk.id)}
                  />
                </div>
              </div>
            ))}
          </div>

          <div className={styles.pager}>
            <Pagination
              current={page}
              pageSize={PAGE_SIZE}
              total={total}
              showTotal={(count) => t('common.totalItems', { total: count })}
              onChange={(next) => { setPage(next); }}
            />
          </div>
        </div>

        <aside className={styles.sidePanel}>
          <div className={styles.panelTitle}>
            <FileTextIcon size={18} weight="duotone" />
            {t('knowledge.chunks.docInfo')}
          </div>
          <Descriptions column={1} size="small">
            <Descriptions.Item label={t('knowledge.chunks.name')}>
              {document?.name ?? "-"}
            </Descriptions.Item>
            <Descriptions.Item label={t('knowledge.chunks.chunkCount')}>{total}</Descriptions.Item>
            <Descriptions.Item label={t('knowledge.chunks.parser')}>
              {document?.parser_id ?? "-"}
            </Descriptions.Item>
            <Descriptions.Item label={t('knowledge.chunks.source')}>
              {document?.source_type ?? "-"}
            </Descriptions.Item>
            <Descriptions.Item label="Metadata">
              {document?.meta_fields.length
                ? t('knowledge.chunks.metaCount', { n: document.meta_fields.length })
                : "-"}
            </Descriptions.Item>
          </Descriptions>
          <Divider />
          <Typography.Text type="secondary">
            {t('knowledge.chunks.imgNote')}
          </Typography.Text>
        </aside>
      </div>

      <ChunkEditor
        open={editorOpen}
        editing={editing}
        datasetId={id}
        documentId={documentId}
        onClose={() => { setEditorOpen(false); }}
      />
    </div>
  );
}
