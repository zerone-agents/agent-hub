import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
// 组件外纯函数：直调 i18next
import i18next from "@/i18n";
import type { Key } from "react";
import {
  Alert,
  Collapse,
  Tag,
  Progress,
  Space,
  Button,
  Input,
  Switch,
  Popconfirm,
  Tooltip,
  Modal,
  Empty,
  Select,
  Typography,
  Descriptions,
  Dropdown,
  Badge,
  message,
} from "antd";
import type { ColumnsType } from "antd/es/table";
import type { TableRowSelection } from "antd/es/table/interface";
import {
  ArrowsClockwiseIcon,
  DownloadSimpleIcon,
  FileTextIcon,
  FunnelSimpleIcon,
  GraphIcon,
  DotsThreeIcon,
  ListBulletsIcon,
  PencilSimpleIcon,
  PlayIcon,
  StopIcon,
  TrashIcon,
  UploadSimpleIcon,
} from "@phosphor-icons/react";
import { createStyles } from "antd-style";
import { useNavigate, useParams } from "react-router";
import {
  knowledgeManagement,
  parseObjectJSON,
} from "@/api/knowledgeManagement";
import KnowledgeDocumentGraph from "./KnowledgeDocumentGraph";
import KnowledgeIngestModal from "./KnowledgeIngestModal";
import {
  documentExactMetadataFilter,
  EmptyDocumentFilterValue,
  ReservedDocumentFilterKey,
  NonFiniteDocumentFilterValue,
} from "./documentSettings";
import MetadataValuesEditor from "./MetadataValuesEditor";
import KnowledgeDocumentUpload from "./KnowledgeDocumentUpload";
import { useDocumentWorkflowText } from "./documentWorkflowText";
import {
  KnowledgeDocumentEditor,
  KnowledgeDocumentCreate,
} from "./KnowledgeDocumentEditor";
import { parseApiError } from "@/api/client";
import { knowledgeApi, type KnowledgeDocument } from "@/api/knowledge";
import {
  useDocuments,
  useDocumentFilters,
  useUploadDocuments,
  useIngestDocuments,
  useStopParsingDocuments,
  useUpdateDocument,
  useDeleteDocuments,
} from "@/queries/useKnowledge";
import BorderedTable from "@/components/BorderedTable";
import PrimaryButton, {
  usePrimaryButtonStyle,
} from "@/components/PrimaryButton";
import { useCanWrite } from "@/hooks/useCanWrite";
import { tokens as t } from "@/styles/tokens";
import { formatTime } from "@/utils/time";

const useStyles = createStyles(({ css }) => ({
  shell: css`
    display: flex;
    flex-direction: column;
    gap: 12px;
  `,
  toolbar: css`
    display: flex;
    justify-content: space-between;
    align-items: flex-start;
    gap: 12px;
    margin: 8px 0 4px;

    @media (max-width: 920px) {
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
    gap: 12px;
    padding: 10px 12px;
    border: 1px solid color-mix(in srgb, var(--foreground) 12%, transparent);
    border-radius: ${t.radius}px;
    background: linear-gradient(
      90deg,
      color-mix(in srgb, var(--foreground) 6%, transparent),
      rgba(5, 150, 105, 0.06)
    );

    @media (max-width: 768px) {
      flex-direction: column;
      align-items: stretch;
    }
  `,
  documentName: css`
    display: flex;
    min-width: 0;
    align-items: center;
    gap: 8px;
  `,
  fileIcon: css`
    display: inline-flex;
    width: 30px;
    height: 30px;
    flex: 0 0 30px;
    align-items: center;
    justify-content: center;
    border-radius: ${t.radiusSm}px;
    background: ${t.inkLight};
    color: ${t.ink};
  `,
  nameText: css`
    min-width: 0;
  `,
  primaryText: css`
    display: block;
    overflow: hidden;
    color: ${t.text};
    font-weight: 600;
    text-overflow: ellipsis;
    white-space: nowrap;
  `,
  secondaryText: css`
    display: block;
    color: ${t.textTertiary};
    font-size: ${t.textXs};
  `,
  statusButton: css`
    width: 100%;
    padding: 0;
    border: 0;
    background: transparent;
    text-align: left;
    cursor: pointer;
  `,
  queueList: css`
    border: 1px solid color-mix(in srgb, var(--foreground) 12%, transparent);
    border-radius: ${t.radiusSm}px;
    background: ${t.surface};
  `,
  queueEmpty: css`
    padding: 16px;
    color: ${t.textTertiary};
    text-align: center;
  `,
  queueItem: css`
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    padding: 10px 12px;

    & + & {
      border-top: 1px solid
        color-mix(in srgb, var(--foreground) 8%, transparent);
    }
  `,
  detailText: css`
    color: ${t.textTertiary};
    font-size: ${t.textSm};
  `,
}));

const PAGE_SIZE = 10;

// 筛选选项 label 存 i18n key，消费处 map t()——模块级 i18next.t() 会在首次
// import 时烘焙语言，运行期切换不生效（PR #172 review 阻塞项 2）。
const STATUS_OPTIONS = [
  { label: "knowledge.docs.runParsing", value: "1" },
  { label: "knowledge.docs.runCancelled", value: "2" },
  { label: "knowledge.docs.runDone", value: "3" },
  { label: "knowledge.docs.runFailed", value: "4" },
  { label: "knowledge.docs.runUnparsed", value: "0" },
];

const SUFFIX_OPTIONS = [
  "pdf",
  "docx",
  "txt",
  "md",
  "html",
  "csv",
  "xlsx",
  "pptx",
  "png",
  "jpg",
];

function formatBytes(bytes: number): string {
  if (!bytes || bytes <= 0) return "-";
  const units = ["B", "KB", "MB", "GB"];
  let value = bytes;
  let unit = 0;
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024;
    unit += 1;
  }
  return `${value.toFixed(value < 10 && unit > 0 ? 1 : 0)} ${units[unit]}`;
}

function statusMeta(doc: KnowledgeDocument): {
  label: string;
  color: "processing" | "success" | "error" | "warning" | "default";
  percent: number;
} {
  const percent = Math.round(doc.progress * 100);
  if (doc.run === "1")
    return {
      label: i18next.t("knowledge.docs.runParsing"),
      color: "processing",
      percent,
    };
  if (doc.run === "4")
    return {
      label: i18next.t("knowledge.docs.runFailed"),
      color: "error",
      percent,
    };
  if (doc.run === "2")
    return {
      label: i18next.t("knowledge.docs.runCancelled"),
      color: "warning",
      percent,
    };
  if (doc.run === "3")
    return {
      label: i18next.t("knowledge.docs.runDone"),
      color: doc.chunk_num ? "success" : "warning",
      percent: 100,
    };
  return {
    label: i18next.t("knowledge.docs.runUnparsed"),
    color: "default",
    percent,
  };
}

function metadataSummary(doc: KnowledgeDocument): string {
  const fields = doc.meta_fields;
  if (fields.length === 0) return "-";
  const pickStr = (v: unknown): string =>
    typeof v === "string"
      ? v
      : typeof v === "number" || typeof v === "boolean" || typeof v === "bigint"
        ? String(v)
        : "";
  const names = fields
    .map((item) => pickStr(item.name ?? item.key ?? item.field).trim())
    .filter(Boolean);
  if (names.length === 0)
    return i18next.t("knowledge.docs.metaCount", { n: fields.length });
  return (
    names.slice(0, 2).join("、") +
    (names.length > 2
      ? i18next.t("knowledge.docs.metaMore", { n: names.length })
      : "")
  );
}

function extractDownloadFileName(
  contentDisposition: unknown,
): string | undefined {
  if (typeof contentDisposition !== "string") return undefined;

  const encodedMatch = /filename\*=([^;]+)/i.exec(contentDisposition);
  if (encodedMatch?.[1]) {
    const encodedValue = encodedMatch[1]
      .trim()
      .replace(/^"?UTF-8''/i, "")
      .replace(/^"|"$/g, "");
    try {
      return decodeURIComponent(encodedValue);
    } catch {
      return encodedValue;
    }
  }

  const filenameMatch = /filename="?([^";]+)"?/i.exec(contentDisposition);
  return filenameMatch?.[1]?.trim();
}

export default function KnowledgeDocumentsPage() {
  const { t } = useTranslation();
  const { styles } = useStyles();
  const text = useDocumentWorkflowText();
  const primaryStyle = usePrimaryButtonStyle();
  const navigate = useNavigate();
  const { id = "" } = useParams();

  const [graphDoc, setGraphDoc] = useState<KnowledgeDocument | null>(null);
  const [idsFilter, setIdsFilter] = useState<string[]>([]);
  const [typesFilter, setTypesFilter] = useState<string[]>([]);
  const [emptyMetadata, setEmptyMetadata] = useState(false);
  const [exactMetadataDraft, setExactMetadataDraft] = useState("{}");
  const [exactMetadata, setExactMetadata] = useState("");
  const [editingDoc, setEditingDoc] = useState<KnowledgeDocument | null>(null);
  const [ingestIds, setIngestIds] = useState<string[] | null>(null);
  const [acceptedAt, setAcceptedAt] = useState(0);
  const [createOpen, setCreateOpen] = useState(false);
  const [bulkBusy, setBulkBusy] = useState(false);
  const [bulkError, setBulkError] = useState("");
  const [metadataFilter, setMetadataFilter] = useState("");
  const [metadataCondition, setMetadataCondition] = useState("");
  const [page, setPage] = useState(1);
  const [keywords, setKeywords] = useState("");
  const [runFilter, setRunFilter] = useState<string[]>([]);
  const [suffixFilter, setSuffixFilter] = useState<string[]>([]);
  const [selectedRowKeys, setSelectedRowKeys] = useState<Key[]>([]);
  const [uploadOpen, setUploadOpen] = useState(false);
  const [statusDoc, setStatusDoc] = useState<KnowledgeDocument | null>(null);
  const [renaming, setRenaming] = useState<KnowledgeDocument | null>(null);
  const [renameValue, setRenameValue] = useState("");
  const [downloadingId, setDownloadingId] = useState<string | null>(null);

  const canWrite = useCanWrite();
  const [modal, modalContext] = Modal.useModal();

  const query = useDocuments(id, {
    page,
    page_size: PAGE_SIZE,
    keywords,
    suffix: suffixFilter,
    run: runFilter,
    ids: idsFilter,
    types: typesFilter,
    metadata: emptyMetadata ? undefined : exactMetadata,
    return_empty_metadata: emptyMetadata,
    metadata_condition: emptyMetadata ? undefined : metadataCondition,
    orderby: "create_time",
    desc: true,
  });
  const filtersQuery = useDocumentFilters(id, {
    keywords,
    run: runFilter,
    types: typesFilter,
    suffix: suffixFilter,
  });
  const candidates = filtersQuery.data?.filter;
  const uploadDocuments = useUploadDocuments(id);
  const ingestDocuments = useIngestDocuments(id);
  const stopParsing = useStopParsingDocuments(id);
  const updateDocument = useUpdateDocument(id);
  const deleteDocuments = useDeleteDocuments(id);

  const documents = query.data?.documents ?? [];
  const total = query.data?.total ?? 0;
  const displayedStatusDoc =
    statusDoc &&
    (documents.find((doc) => doc.id === statusDoc.id) ?? statusDoc);
  const selectedIds = selectedRowKeys.map(String);
  const hasRunning = documents.some((doc) => doc.run === "1");

  useEffect(() => {
    if (!hasRunning && !acceptedAt) return;
    const timer = window.setInterval(() => {
      void query.refetch();
      if (acceptedAt && Date.now() - acceptedAt >= 60000) setAcceptedAt(0);
    }, 3500);
    return () => {
      window.clearInterval(timer);
    };
  }, [hasRunning, acceptedAt, query]);

  const submitRename = async () => {
    if (!renaming) return;
    const name = renameValue.trim();
    if (name && name !== renaming.name) {
      await updateDocument.mutateAsync({
        documentId: renaming.id,
        patch: { name },
      });
    }
    setRenaming(null);
  };

  const handleUpload = async (file: File) => {
    const docs = await uploadDocuments.mutateAsync([file]);
    const doc = docs.find((item) => item.id);
    if (!doc) throw new Error(text("missing"));
    return doc;
  };
  const handleUploadParse = async (doc: KnowledgeDocument) => {
    await ingestDocuments.mutateAsync({
      doc_ids: [doc.id],
      run: 1,
      delete: false,
      apply_kb: false,
    });
    setAcceptedAt(Date.now());
  };

  const handleDownload = async (doc: KnowledgeDocument) => {
    setDownloadingId(doc.id);
    try {
      const response = await knowledgeApi.documents.download(id, doc.id);
      const blob = response.data;
      const objectUrl = URL.createObjectURL(blob);
      const link = document.createElement("a");
      link.href = objectUrl;
      link.download =
        extractDownloadFileName(response.headers["content-disposition"]) ??
        (doc.name || `document-${doc.id}`);
      document.body.appendChild(link);
      link.click();
      link.remove();
      window.setTimeout(() => {
        URL.revokeObjectURL(objectUrl);
      }, 0);
    } catch (err) {
      message.error(parseApiError(err));
    } finally {
      setDownloadingId(null);
    }
  };

  const bulkSwitch = async (enabled: boolean) => {
    setBulkBusy(true);
    setBulkError("");
    try {
      const result = await knowledgeManagement.documentStatus(
        id,
        selectedIds,
        enabled,
      );
      setSelectedRowKeys(result.failed);
      await query.refetch();
      if (result.failed.length)
        setBulkError(
          t("knowledge.manage.bulkPartial", {
            succeeded: result.succeeded.length,
            failed: result.failed.length,
          }),
        );
      else
        message.success(
          enabled
            ? t("knowledge.docs.bulkEnabled")
            : t("knowledge.docs.bulkDisabled"),
        );
    } catch (err) {
      setBulkError(parseApiError(err));
      await query.refetch();
    } finally {
      setBulkBusy(false);
    }
  };

  const bulkParse = () => {
    setIngestIds(selectedIds);
  };

  const bulkStop = () => {
    stopParsing.mutate(selectedIds, {
      onSuccess: () => {
        setSelectedRowKeys([]);
      },
    });
  };

  const bulkDelete = () => {
    deleteDocuments.mutate(selectedIds, {
      onSuccess: () => {
        setSelectedRowKeys([]);
      },
    });
  };

  const rowSelection: TableRowSelection<KnowledgeDocument> = {
    selectedRowKeys,
    onChange: setSelectedRowKeys,
    preserveSelectedRowKeys: true,
  };

  const columns: ColumnsType<KnowledgeDocument> = [
    {
      title: t("knowledge.docs.docCol"),
      dataIndex: "name",
      key: "name",
      width: 280,
      ellipsis: true,
      render: (value: string, record) => (
        <div className={styles.documentName}>
          <span className={styles.fileIcon}>
            <FileTextIcon size={18} weight="duotone" />
          </span>
          <span className={styles.nameText}>
            <Tooltip title={value} placement="topLeft">
              <span className={styles.primaryText}>
                {value || t("knowledge.docs.unnamed")}
              </span>
            </Tooltip>
            <span className={styles.secondaryText}>
              {/* eslint-disable-next-line @typescript-eslint/no-unnecessary-condition -- runtime defense: API may omit suffix */}
              {(record.suffix ?? record.type ?? "file").toUpperCase()} ·{" "}
              {formatBytes(record.size)}
            </span>
          </span>
        </div>
      ),
    },
    {
      title: t("knowledge.docs.parserCol"),
      key: "parser_id",
      width: 120,
      render: (_, record) => (
        <Button
          type="link"
          size="small"
          disabled={!canWrite}
          aria-label={`${t("knowledge.manage.parserSettings")}: ${record.name}`}
          onClick={() => {
            setEditingDoc(record);
          }}
        >
          {record.pipeline_id ? t("knowledge.docs.processingFlow") : record.parser_id || "naive"}
        </Button>
      ),
    },
    {
      title: t("knowledge.manage.metadataManagement"),
      key: "metadata",
      width: 160,
      ellipsis: true,
      render: (_, record) => (
        <Tooltip title={metadataSummary(record)}>
          <span>{metadataSummary(record)}</span>
        </Tooltip>
      ),
    },
    {
      title: t("knowledge.docs.chunkCol"),
      dataIndex: "chunk_num",
      key: "chunk_num",
      width: 80,
      align: "right",
    },
    {
      title: t("knowledge.docs.statusCol"),
      key: "status",
      width: 170,
      render: (_, record) => {
        const meta = statusMeta(record);
        return (
          <button
            type="button"
            className={styles.statusButton}
            aria-label={t("knowledge.docs.viewStatusAria", {
              label: meta.label,
            })}
            onClick={() => {
              setStatusDoc(record);
            }}
          >
            <Space orientation="vertical" size={3} style={{ width: "100%" }}>
              <Badge status={meta.color} text={meta.label} />
              {record.run === "3" && (
                <Typography.Text
                  type={record.chunk_num ? "secondary" : "warning"}
                >
                  {t(
                    record.chunk_num
                      ? "knowledge.manage.chunksProduced"
                      : "knowledge.manage.noChunksProduced",
                    { n: record.chunk_num },
                  )}
                </Typography.Text>
              )}
              {record.run === "1" ? (
                <Progress percent={meta.percent} size="small" status="active" />
              ) : record.progress_msg ? (
                <Typography.Text
                  type="secondary"
                  ellipsis
                  style={{ maxWidth: 140 }}
                >
                  {record.progress_msg}
                </Typography.Text>
              ) : null}
            </Space>
          </button>
        );
      },
    },
    {
      title: t("knowledge.docs.enabledCol"),
      key: "enabled",
      width: 76,
      render: (_, record) =>
        canWrite ? (
          <Switch
            size="small"
            checked={record.enabled}
            onChange={(checked) => {
              updateDocument.mutate({
                documentId: record.id,
                patch: { enabled: checked },
              });
            }}
          />
        ) : (
          <Tag color={record.enabled ? "success" : "default"}>
            {record.enabled
              ? t("knowledge.docs.enabled")
              : t("knowledge.docs.disabled")}
          </Tag>
        ),
    },
    {
      title: t("knowledge.docs.createdAt"),
      key: "create_time",
      width: 136,
      render: (_, record) =>
        formatTime(record.create_time ?? record.create_date),
    },
    {
      title: t("knowledge.docs.actions"),
      key: "action",
      width: 200,
      fixed: "right",
      render: (_, record) => (
        <Space size={4} wrap>
          {canWrite &&
            (record.run === "1" ? (
              <Button
                type="link"
                size="small"
                aria-label={t("knowledge.docs.stopParseAria", {
                  name: record.name,
                })}
                icon={<StopIcon size={14} />}
                onClick={() => {
                  stopParsing.mutate([record.id]);
                }}
              >
                {t("knowledge.docs.stop")}
              </Button>
            ) : (
              <Button
                type="link"
                size="small"
                aria-label={t("knowledge.docs.parseDocAria", {
                  name: record.name,
                })}
                icon={<PlayIcon size={14} />}
                onClick={() => {
                  setIngestIds([record.id]);
                }}
              >
                {t("knowledge.docs.parse")}
              </Button>
            ))}
          <Button
            type="link"
            size="small"
            icon={<ListBulletsIcon size={14} />}
            onClick={async () => {
              await navigate(`/knowledge/${id}/documents/${record.id}/chunks`);
            }}
          >
            {t("knowledge.docs.chunkNav")}
          </Button>
          <Dropdown
            trigger={["click"]}
            menu={{
              items: [
                {
                  key: "download",
                  label: t("common.download"),
                  icon: <DownloadSimpleIcon size={16} />,
                  disabled: downloadingId === record.id,
                  onClick: () => void handleDownload(record),
                },
                {
                  key: "graph",
                  label: t("knowledge.manage.documentGraph"),
                  icon: <GraphIcon size={16} />,
                  onClick: () => {
                    setGraphDoc(record);
                  },
                },
                ...(canWrite
                  ? [
                      {
                        key: "settings",
                        label: t("knowledge.manage.settings"),
                        onClick: () => {
                          setEditingDoc(record);
                        },
                      },
                      {
                        key: "rename",
                        label: t("knowledge.docs.rename"),
                        icon: <PencilSimpleIcon size={16} />,
                        onClick: () => {
                          setRenaming(record);
                          setRenameValue(record.name);
                        },
                      },
                      {
                        key: "delete",
                        label: t("common.delete"),
                        danger: true,
                        icon: <TrashIcon size={16} />,
                        onClick: () => {
                          modal.confirm({
                            title: t("scenes.deleteConfirmTitle"),
                            content: t("knowledge.docs.deleteDesc", {
                              name: record.name,
                            }),
                            okText: t("common.delete"),
                            cancelText: t("common.cancel"),
                            okButtonProps: {
                              danger: true,
                              className: primaryStyle.root,
                            },
                            onOk: () =>
                              deleteDocuments.mutateAsync([record.id]),
                          });
                        },
                      },
                    ]
                  : []),
              ],
            }}
          >
            <Button
              type="text"
              size="small"
              aria-label={`${text("moreActions")}: ${record.name}`}
              icon={<DotsThreeIcon size={18} />}
            />
          </Dropdown>
        </Space>
      ),
    },
  ];

  return (
    <div className={styles.shell}>
      {modalContext}
      {query.error && (
        <Alert
          type="error"
          showIcon
          title={parseApiError(query.error)}
          action={
            <Button onClick={() => void query.refetch()}>
              {text("retry")}
            </Button>
          }
        />
      )}
      {graphDoc && (
        <KnowledgeDocumentGraph
          datasetId={id}
          documentId={graphDoc.id}
          name={graphDoc.name}
          onClose={() => {
            setGraphDoc(null);
          }}
        />
      )}
      {bulkError && <Alert type="warning" showIcon title={bulkError} />}
      {editingDoc && (
        <KnowledgeDocumentEditor
          datasetId={id}
          document={editingDoc}
          onClose={() => {
            setEditingDoc(null);
          }}
          onSaved={() => void query.refetch()}
          onReparse={(doc) => {
            setIngestIds([doc.id]);
          }}
        />
      )}
      {createOpen && (
        <KnowledgeDocumentCreate
          datasetId={id}
          onClose={() => {
            setCreateOpen(false);
          }}
          onSaved={() => void query.refetch()}
        />
      )}
      <div className={styles.toolbar}>
        <div className={styles.filters}>
          <Input.Search
            placeholder={t("knowledge.docs.searchPh")}
            allowClear
            value={keywords}
            onChange={(event) => {
              setKeywords(event.target.value);
              setPage(1);
            }}
            style={{ width: "min(260px, 100%)" }}
            onSearch={(value) => {
              setKeywords(value.trim());
              setPage(1);
            }}
          />
          <Select
            aria-label={t("knowledge.docs.statusPh")}
            mode="multiple"
            allowClear
            maxTagCount="responsive"
            placeholder={t("knowledge.docs.statusPh")}
            style={{ minWidth: 160 }}
            options={STATUS_OPTIONS.map((o) => ({
              ...o,
              label: `${t(o.label)}${candidates?.run_status[o.value] !== undefined ? ` (${candidates.run_status[o.value]})` : ""}`,
            }))}
            value={runFilter}
            onChange={(value) => {
              setRunFilter(value);
              setPage(1);
            }}
          />
          <Select
            mode="tags"
            allowClear
            maxTagCount="responsive"
            aria-label={t("knowledge.docs.fileTypePh")}
            placeholder={t("knowledge.docs.fileTypePh")}
            style={{ minWidth: 160 }}
            options={Array.from(
              new Set([
                ...SUFFIX_OPTIONS,
                ...Object.keys(candidates?.suffix ?? {}),
                ...suffixFilter,
              ]),
            ).map((value) => ({
              label: `${value.toUpperCase()}${candidates?.suffix[value] !== undefined ? ` (${candidates.suffix[value]})` : ""}`,
              value,
            }))}
            value={suffixFilter}
            onChange={(value) => {
              setSuffixFilter(value);
              setPage(1);
            }}
          />
          <Button
            icon={<FunnelSimpleIcon size={16} />}
            onClick={() => {
              setKeywords("");
              setRunFilter([]);
              setSuffixFilter([]);
              setIdsFilter([]);
              setTypesFilter([]);
              setEmptyMetadata(false);
              setExactMetadata("");
              setExactMetadataDraft("{}");
              setMetadataFilter("");
              setMetadataCondition("");
              setPage(1);
            }}
          >
            {t("knowledge.docs.reset")}
          </Button>
        </div>
        <div className={styles.actions}>
          <Button
            icon={<ArrowsClockwiseIcon size={16} />}
            loading={query.isFetching}
            onClick={() => query.refetch()}
          >
            {t("knowledge.docs.refresh")}
          </Button>
          {canWrite && (
            <Dropdown
              trigger={["click"]}
              menu={{
                items: [
                  {
                    key: "upload",
                    label: t("knowledge.docs.uploadBtn"),
                    onClick: () => {
                      setUploadOpen(true);
                    },
                  },
                  {
                    key: "create",
                    label: t("knowledge.manage.createDocument"),
                    onClick: () => {
                      setCreateOpen(true);
                    },
                  },
                ],
              }}
            >
              <PrimaryButton icon={<UploadSimpleIcon size={16} />}>
                {text("addResources")}
              </PrimaryButton>
            </Dropdown>
          )}
        </div>
      </div>

      <Collapse
        items={[
          {
            key: "filters",
            label: t("knowledge.manage.preciseDocumentFilters"),
            children: (
              <Space
                orientation="vertical"
                size="middle"
                style={{ width: "100%" }}
              >
                <div className={styles.filters}>
                  <Select
                    aria-label={t("knowledge.manage.documentIDs")}
                    mode="tags"
                    allowClear
                    value={idsFilter}
                    placeholder={t("knowledge.manage.documentIDs")}
                    style={{ minWidth: 180, flex: "1 1 220px" }}
                    onChange={(values) => {
                      setIdsFilter(values);
                      setPage(1);
                    }}
                  />
                  <Select
                    aria-label={t("knowledge.manage.documentTypes")}
                    mode="multiple"
                    allowClear
                    value={typesFilter}
                    placeholder={t("knowledge.manage.documentTypes")}
                    style={{ minWidth: 160, flex: "1 1 180px" }}
                    options={[
                      "pdf",
                      "doc",
                      "visual",
                      "aural",
                      "virtual",
                      "folder",
                      "other",
                    ].map((value) => ({
                      value,
                      label: t(`knowledge.manage.documentType_${value}`),
                    }))}
                    onChange={(values) => {
                      setTypesFilter(values);
                      setPage(1);
                    }}
                  />
                </div>
                <Space wrap>
                  <Switch
                    aria-label={t("knowledge.manage.onlyEmptyMetadata")}
                    checked={emptyMetadata}
                    onChange={(value) => {
                      setEmptyMetadata(value);
                      setPage(1);
                    }}
                  />
                  <Typography.Text>
                    {t("knowledge.manage.onlyEmptyMetadata")}
                  </Typography.Text>
                </Space>
                {emptyMetadata && (
                  <Alert
                    type="info"
                    showIcon
                    title={t("knowledge.manage.emptyMetadataFilterHint")}
                  />
                )}
                {filtersQuery.error && (
                  <Alert
                    type="warning"
                    showIcon
                    title={parseApiError(filtersQuery.error)}
                    action={
                      <Button onClick={() => void filtersQuery.refetch()}>
                        {text("retry")}
                      </Button>
                    }
                  />
                )}
                <Typography.Text strong>
                  {t("knowledge.manage.exactMetadataFilter")}
                </Typography.Text>
                <MetadataValuesEditor
                  value={exactMetadataDraft}
                  onChange={setExactMetadataDraft}
                  disabled={emptyMetadata}
                  fields={Object.entries(candidates?.metadata ?? {})
                    .filter(([key]) => key !== "empty_metadata")
                    .map(([key, values]) => ({
                      key,
                      enum: Object.keys(values),
                      restrict_values: true,
                    }))}
                />
                <Button
                  disabled={emptyMetadata}
                  onClick={() => {
                    try {
                      setExactMetadata(
                        documentExactMetadataFilter(exactMetadataDraft),
                      );
                      setPage(1);
                    } catch (err) {
                      message.error(
                        err instanceof ReservedDocumentFilterKey
                          ? text("reservedFilterKey")
                          : err instanceof EmptyDocumentFilterValue
                            ? text("emptyFilterValue")
                            : err instanceof NonFiniteDocumentFilterValue
                              ? text("nonFiniteFilterValue")
                              : parseApiError(err),
                      );
                    }
                  }}
                >
                  {text("apply")}
                </Button>
                <Input.Search
                  disabled={emptyMetadata}
                  value={metadataFilter}
                  onChange={(e) => {
                    setMetadataFilter(e.target.value);
                  }}
                  placeholder={t("knowledge.manage.metadataFilter")}
                  style={{ width: 240 }}
                  onSearch={(value) => {
                    try {
                      if (value.trim()) parseObjectJSON(value);
                      setMetadataCondition(value.trim());
                      setPage(1);
                    } catch {
                      message.error(t("knowledge.manage.jsonInvalid"));
                    }
                  }}
                />
                <Typography.Text type="secondary">
                  {text("exactFilterValueHint")}
                  <br />
                  {t("knowledge.manage.exactMetadataFilterHint")}
                </Typography.Text>
              </Space>
            ),
          },
        ]}
      />

      {canWrite && selectedIds.length > 0 ? (
        <div className={styles.bulkBar}>
          <Typography.Text strong>
            {t("knowledge.docs.selectedN", { n: selectedIds.length })}
          </Typography.Text>
          <Space wrap>
            <Button
              size="small"
              loading={bulkBusy}
              aria-label={t("knowledge.docs.bulkEnableAria")}
              onClick={() => void bulkSwitch(true)}
            >
              {t("knowledge.docs.enabled")}
            </Button>
            <Button
              size="small"
              loading={bulkBusy}
              aria-label={t("knowledge.docs.bulkDisableAria")}
              onClick={() => void bulkSwitch(false)}
            >
              {t("knowledge.docs.disabled")}
            </Button>
            <Button
              size="small"
              aria-label={t("knowledge.docs.bulkParseAria")}
              icon={<PlayIcon size={14} />}
              onClick={bulkParse}
            >
              {t("knowledge.docs.parse")}
            </Button>
            <Button
              size="small"
              aria-label={t("knowledge.docs.bulkStopParseAria")}
              icon={<StopIcon size={14} />}
              onClick={bulkStop}
            >
              {t("knowledge.docs.stop")}
            </Button>
            <Popconfirm
              title={t("knowledge.docs.deleteSelectedTitle")}
              description={t("knowledge.docs.deleteSelectedDesc", {
                n: selectedIds.length,
              })}
              okText={t("common.delete")}
              okButtonProps={{ danger: true }}
              cancelText={t("common.cancel")}
              onConfirm={bulkDelete}
            >
              <Button size="small" danger icon={<TrashIcon size={14} />}>
                {t("common.delete")}
              </Button>
            </Popconfirm>
            <Button
              size="small"
              type="text"
              onClick={() => {
                setSelectedRowKeys([]);
              }}
            >
              {t("knowledge.docs.clearSelection")}
            </Button>
          </Space>
        </div>
      ) : null}

      <BorderedTable<KnowledgeDocument>
        columns={columns}
        dataSource={documents}
        rowKey="id"
        rowSelection={canWrite ? rowSelection : undefined}
        size="middle"
        loading={query.isLoading}
        scroll={{ x: 1180 }}
        locale={{
          emptyText: (
            <Empty
              description={
                keywords ||
                runFilter.length ||
                suffixFilter.length ||
                idsFilter.length ||
                typesFilter.length ||
                emptyMetadata ||
                exactMetadata ||
                metadataCondition
                  ? t("knowledge.docs.emptyNoMatch")
                  : canWrite
                    ? t("knowledge.docs.emptyNoUpload")
                    : t("knowledge.docs.emptyNone")
              }
            />
          ),
        }}
        pagination={{
          current: page,
          pageSize: PAGE_SIZE,
          total,
          showTotal: (count) => t("common.totalItems", { total: count }),
          onChange: (next) => {
            setPage(next);
          },
        }}
      />

      {ingestIds && (
        <KnowledgeIngestModal
          docIds={ingestIds}
          names={Object.fromEntries(documents.map((doc) => [doc.id, doc.name]))}
          onClose={() => {
            setIngestIds(null);
          }}
          onSubmit={async (input) => {
            await ingestDocuments.mutateAsync(input);
            setAcceptedAt(Date.now());
            setSelectedRowKeys((current) =>
              current.filter((key) => !input.doc_ids.includes(String(key))),
            );
          }}
        />
      )}

      {uploadOpen && (
        <KnowledgeDocumentUpload
          onClose={() => {
            setUploadOpen(false);
          }}
          onUpload={handleUpload}
          onParse={handleUploadParse}
        />
      )}

      <Modal
        title={t("knowledge.docs.statusCol")}
        open={!!statusDoc}
        onCancel={() => {
          setStatusDoc(null);
        }}
        footer={
          <Button
            onClick={() => {
              setStatusDoc(null);
            }}
          >
            {t("knowledge.docs.close")}
          </Button>
        }
        width={640}
        destroyOnHidden
      >
        {displayedStatusDoc ? (
          <Space
            orientation="vertical"
            size={16}
            style={{ width: "100%", marginTop: 8 }}
          >
            <Alert
              type="info"
              showIcon
              title={t("knowledge.manage.retrievalReadinessHint")}
            />
            <Progress
              percent={statusMeta(displayedStatusDoc).percent}
              status={displayedStatusDoc.run === "4" ? "exception" : undefined}
            />
            <Descriptions column={1} size="small" bordered>
              <Descriptions.Item label={t("knowledge.docs.statusDocCol")}>
                {displayedStatusDoc.name}
              </Descriptions.Item>
              <Descriptions.Item label={t("knowledge.docs.statusColState")}>
                {statusMeta(displayedStatusDoc).label}
              </Descriptions.Item>
              <Descriptions.Item label={t("knowledge.docs.progressMsg")}>
                {displayedStatusDoc.progress_msg || "-"}
              </Descriptions.Item>
              <Descriptions.Item label={t("knowledge.docs.statusChunkCount")}>
                {displayedStatusDoc.chunk_num}
              </Descriptions.Item>
              <Descriptions.Item label={t("knowledge.docs.elapsed")}>
                {displayedStatusDoc.process_duration
                  ? `${displayedStatusDoc.process_duration}s`
                  : "-"}
              </Descriptions.Item>
              <Descriptions.Item label={t("knowledge.docs.startTime")}>
                {formatTime(
                  displayedStatusDoc.process_begin_at ??
                    displayedStatusDoc.create_time ??
                    displayedStatusDoc.create_date,
                )}
              </Descriptions.Item>
              <Descriptions.Item label={t("knowledge.docs.errorSummary")}>
                {displayedStatusDoc.run === "4"
                  ? displayedStatusDoc.progress_msg ||
                    t("knowledge.docs.parseFail")
                  : "-"}
              </Descriptions.Item>
            </Descriptions>
          </Space>
        ) : null}
      </Modal>

      <Modal
        title={t("knowledge.docs.renameTitle")}
        open={!!renaming}
        onOk={submitRename}
        onCancel={() => {
          setRenaming(null);
        }}
        okButtonProps={{ className: primaryStyle.root }}
        confirmLoading={updateDocument.isPending}
        okText={t("knowledge.docs.saveBtn")}
        cancelText={t("common.cancel")}
        destroyOnHidden
      >
        <Input
          value={renameValue}
          onChange={(event) => {
            setRenameValue(event.target.value);
          }}
          onPressEnter={submitRename}
          placeholder={t("knowledge.docs.renamePh")}
          style={{ marginTop: 8 }}
        />
      </Modal>
    </div>
  );
}
