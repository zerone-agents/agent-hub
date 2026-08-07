import { useEffect, useMemo, useState } from "react";
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
  ArrowLeft,
  ArrowsClockwise,
  ClipboardText,
  FileText,
  ImageSquare,
  PencilSimple,
  Plus,
  Trash,
} from "@phosphor-icons/react";
import { createStyles } from "antd-style";
import { useNavigate, useParams } from "react-router-dom";
import { parseApiError } from "@/api/client";
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
  const html = sanitizeAllowedInlineHtml(content || "(空)");
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
  if (Array.isArray(first)) return `位置 ${first.slice(0, 3).join(", ")}`;
  return `位置 ${positions.slice(0, 3).map(String).join(", ")}`;
}

function tagFeasText(chunk: KnowledgeChunk | null): string {
  if (!chunk || Object.keys(chunk.tag_feas).length === 0) return "";
  return JSON.stringify(chunk.tag_feas, null, 2);
}

function fileToBase64(file: File): Promise<ImageFileReadResult> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => {
      const dataUrl = String(reader.result ?? "");
      const marker = "base64,";
      const markerIndex = dataUrl.indexOf(marker);
      const base64 =
        markerIndex >= 0 ? dataUrl.slice(markerIndex + marker.length) : dataUrl;
      resolve({ dataUrl, base64 });
    };
    reader.onerror = () => { reject(reader.error); };
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

    setStatus("loading");
    setErrorMessage("");
    setImageSrc("");

    knowledgeApi.images
      .fetch(datasetId, imageId, controller.signal)
      .then((blob) => {
        if (!active) return;
        if (blob.type && !blob.type.startsWith("image/")) {
          throw new Error("图片代理返回了非图片内容");
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
    try {
      await navigator.clipboard.writeText(imageId);
      message.success("已复制 image id");
    } catch {
      message.error("复制失败");
    }
  };

  return (
    <div className={cx(styles.imageFrame, className)} style={{ width, height }}>
      {status === "failed" ? (
        <div className={styles.imageFallback}>
          <ImageSquare size={18} />
          <span>图片加载失败</span>
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
              重试
            </Button>
            <Button size="small" type="link" onClick={() => void copyImageId()}>
              复制 ID
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
          message.error("结构化标签必须是 JSON 对象");
          return;
        }
        tagFeas = parsed as Record<string, unknown>;
      } catch {
        message.error("结构化标签不是有效 JSON");
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
      title={editing ? "编辑切片" : "新增切片"}
      open={open}
      onClose={onClose}
      width={720}
      destroyOnHidden
      extra={
        <Space>
          <Button onClick={onClose}>取消</Button>
          <Button
            type="primary"
            loading={submitting}
            aria-label={editing ? "保存切片" : "创建切片"}
            onClick={submit}
          >
            {editing ? "保存" : "创建"}
          </Button>
        </Space>
      }
    >
      <Form form={form} layout="vertical" requiredMark={false}>
        <Space orientation="vertical" size={14} style={{ width: "100%" }}>
          <Segmented
            value={previewMode}
            onChange={(value) => { setPreviewMode(value as "edit" | "preview"); }}
            options={[
              { label: "编辑", value: "edit" },
              { label: "预览", value: "preview" },
            ]}
          />

          {previewMode === "edit" ? (
            <Form.Item
              label="内容"
              name="content"
              rules={[{ required: true, message: "请输入切片内容" }]}
            >
              <Input.TextArea rows={10} placeholder="切片文本内容" />
            </Form.Item>
          ) : (
            <div className={styles.preview}>
              <SafeContent content={contentValue} clipped={false} />
            </div>
          )}

          <Form.Item label="关键词" name="important_keywords">
            <Select
              mode="tags"
              placeholder="输入后回车添加关键词"
              tokenSeparators={[","]}
            />
          </Form.Item>

          <Form.Item label="关联问题" name="questions">
            <Select
              mode="tags"
              placeholder="输入后回车添加问题"
              tokenSeparators={[","]}
            />
          </Form.Item>

          <Form.Item label="标签" name="tag_kwd">
            <Select
              mode="tags"
              placeholder="输入后回车添加标签"
              tokenSeparators={[","]}
            />
          </Form.Item>

          <Form.Item label="结构化标签 JSON" name="tag_feas_text">
            <Input.TextArea rows={4} placeholder='例如 {"source":"manual"}' />
          </Form.Item>

          {editing?.image_id ? (
            <Space orientation="vertical">
              <Typography.Text strong>图片</Typography.Text>
              <ChunkImage
                datasetId={datasetId}
                imageId={editing.image_id}
                width={180}
                height={120}
              />
              <Typography.Text type="secondary">
                当前 RESTful update 未确认支持图片替换，本轮只展示。
              </Typography.Text>
            </Space>
          ) : null}

          {!editing ? (
            <Space orientation="vertical" size={8} style={{ width: "100%" }}>
              <Typography.Text strong>图片切片</Typography.Text>
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
                <Button icon={<ImageSquare size={16} />}>选择图片</Button>
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

  const chunks = query.data?.chunks ?? [];
  const total = query.data?.total ?? 0;
  const document = query.data?.document;
  const currentIds = useMemo(() => chunks.map((chunk) => chunk.id), [chunks]);
  const allCurrentSelected =
    currentIds.length > 0 &&
    currentIds.every((chunkId) => selectedIds.includes(chunkId));

  useEffect(() => {
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
    message.success(availableValue ? "已批量启用切片" : "已批量停用切片");
    setSelectedIds([]);
  };

  const bulkDelete = () => {
    deleteChunks.mutate(selectedIds);
    setSelectedIds([]);
  };

  const copyChunkId = async (chunkId: string) => {
    try {
      await navigator.clipboard.writeText(chunkId);
      message.success("已复制 chunk id");
    } catch {
      message.error("复制失败");
    }
  };

  return (
    <div>
      <button
        type="button"
        className={styles.back}
        onClick={() => { navigate(`/knowledge/${id}/documents`); }}
      >
        <ArrowLeft size={14} />
        返回文档列表
      </button>

      <div className={styles.shell}>
        <div className={styles.main}>
          <div className={styles.toolbar}>
            <div className={styles.filters}>
              <Input.Search
                placeholder="搜索切片内容"
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
                  { label: "省略", value: "ellipsis" },
                  { label: "全文", value: "full" },
                ]}
              />
              <Select
                value={availableFilter}
                style={{ width: 130 }}
                options={[
                  { label: "全部状态", value: "all" },
                  { label: "仅启用", value: "enabled" },
                  { label: "仅停用", value: "disabled" },
                ]}
                onChange={(value) => {
                  setAvailableFilter(value);
                  setPage(1);
                }}
              />
              <Checkbox
                checked={allCurrentSelected}
                indeterminate={selectedIds.length > 0 && !allCurrentSelected}
                onChange={(event) => { toggleAll(event.target.checked); }}
              >
                选择本页
              </Checkbox>
            </div>
            <div className={styles.actions}>
              <Button
                icon={<ArrowsClockwise size={16} />}
                loading={query.isFetching}
                onClick={() => query.refetch()}
              >
                刷新
              </Button>
              <Button
                type="primary"
                icon={<Plus size={16} weight="bold" />}
                onClick={openCreate}
              >
                新增切片
              </Button>
            </div>
          </div>

          {selectedIds.length > 0 ? (
            <div className={styles.bulkBar}>
              <Typography.Text strong>
                已选择 {selectedIds.length} 个切片
              </Typography.Text>
              <Space wrap>
                <Button
                  size="small"
                  aria-label="批量启用切片"
                  onClick={() => { bulkSwitch(true); }}
                >
                  启用
                </Button>
                <Button
                  size="small"
                  aria-label="批量停用切片"
                  onClick={() => { bulkSwitch(false); }}
                >
                  停用
                </Button>
                <Popconfirm
                  title="确认删除选中切片？"
                  description={`将删除 ${selectedIds.length} 个切片。`}
                  okText="删除"
                  okButtonProps={{ danger: true }}
                  cancelText="取消"
                  onConfirm={bulkDelete}
                >
                  <Button size="small" danger icon={<Trash size={14} />}>
                    删除
                  </Button>
                </Popconfirm>
                <Button
                  size="small"
                  type="text"
                  onClick={() => { setSelectedIds([]); }}
                >
                  清除选择
                </Button>
              </Space>
            </div>
          ) : null}

          <div className={styles.list}>
            {chunks.length === 0 && !query.isLoading ? (
              <Empty
                description={keywords ? "未找到匹配的切片" : "还没有切片"}
              />
            ) : null}
            {chunks.map((chunk) => (
              <div className={styles.card} key={chunk.id}>
                <Checkbox
                  checked={selectedIds.includes(chunk.id)}
                  onChange={(event) =>
                    { toggleOne(chunk.id, event.target.checked); }
                  }
                />
                <div className={styles.cardBody}>
                  <div className={styles.cardMeta}>
                    <Tag color={chunk.image_id ? "purple" : "blue"}>
                      {chunk.doc_type ?? (chunk.image_id ? "image" : "text")}
                    </Tag>
                    <Tag>{formatPositions(chunk)}</Tag>
                    <Badge
                      status={chunk.available ? "success" : "default"}
                      text={chunk.available ? "启用" : "停用"}
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
                    icon={<ClipboardText size={16} />}
                    onClick={() => void copyChunkId(chunk.id)}
                  />
                  <Button
                    type="text"
                    size="small"
                    icon={<PencilSimple size={16} />}
                    onClick={() => { openEdit(chunk); }}
                  >
                    编辑
                  </Button>
                  <Popconfirm
                    title="确认删除该切片？"
                    okText="删除"
                    okButtonProps={{ danger: true }}
                    cancelText="取消"
                    onConfirm={() => { deleteChunks.mutate([chunk.id]); }}
                  >
                    <Button
                      type="text"
                      size="small"
                      danger
                      icon={<Trash size={16} />}
                    >
                      删除
                    </Button>
                  </Popconfirm>
                </div>
              </div>
            ))}
          </div>

          <div className={styles.pager}>
            <Pagination
              current={page}
              pageSize={PAGE_SIZE}
              total={total}
              showTotal={(count) => `共 ${count} 条`}
              onChange={(next) => { setPage(next); }}
            />
          </div>
        </div>

        <aside className={styles.sidePanel}>
          <div className={styles.panelTitle}>
            <FileText size={18} weight="duotone" />
            文档信息
          </div>
          <Descriptions column={1} size="small">
            <Descriptions.Item label="名称">
              {document?.name ?? "-"}
            </Descriptions.Item>
            <Descriptions.Item label="切片数">{total}</Descriptions.Item>
            <Descriptions.Item label="解析方法">
              {document?.parser_id ?? "-"}
            </Descriptions.Item>
            <Descriptions.Item label="来源">
              {document?.source_type ?? "-"}
            </Descriptions.Item>
            <Descriptions.Item label="Metadata">
              {document?.meta_fields.length
                ? `${document.meta_fields.length} 项`
                : "-"}
            </Descriptions.Item>
          </Descriptions>
          <Divider />
          <Typography.Text type="secondary">
            图片切片通过 control-panel gateway 加载；完整 Office/PDF
            预览排在后续增强。
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
