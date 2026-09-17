import { useEffect, useState } from "react";
import { useTranslation } from 'react-i18next'
// 组件外纯函数：直调 i18next
import i18next from '@/i18n'
import type { Key } from "react";
import {
  Tag,
  Progress,
  Space,
  Button,
  Input,
  Upload,
  Switch,
  Popconfirm,
  Tooltip,
  Modal,
  Empty,
  Select,
  Typography,
  Descriptions,
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
  ListBulletsIcon,
  PencilSimpleIcon,
  PlayIcon,
  StopIcon,
  TrashIcon,
  UploadSimpleIcon,
} from "@phosphor-icons/react";
import { createStyles } from "antd-style";
import { useNavigate, useParams } from "react-router";
import { parseApiError } from "@/api/client";
import { knowledgeApi, type KnowledgeDocument } from "@/api/knowledge";
import {
  useDocuments,
  useUploadDocuments,
  useParseDocuments,
  useStopParsingDocuments,
  useUpdateDocument,
  useDeleteDocuments,
} from "@/queries/useKnowledge";
import BorderedTable from "@/components/BorderedTable";
import PrimaryButton from "@/components/PrimaryButton";
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
      border-top: 1px solid color-mix(in srgb, var(--foreground) 8%, transparent);
    }
  `,
  detailText: css`
    color: ${t.textTertiary};
    font-size: ${t.textSm};
  `,
}));

const PAGE_SIZE = 10;

const STATUS_OPTIONS = [
  { label: i18next.t('knowledge.docs.runParsing'), value: "1" },
  { label: i18next.t('knowledge.docs.runCancelled'), value: "2" },
  { label: i18next.t('knowledge.docs.runDone'), value: "3" },
  { label: i18next.t('knowledge.docs.runFailed'), value: "4" },
  { label: i18next.t('knowledge.docs.runUnparsed'), value: "0" },
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
  const percent = Math.round((doc.progress) * 100);
  if (doc.run === "1") return { label: i18next.t('knowledge.docs.runParsing'), color: "processing", percent };
  if (doc.run === "3" || percent >= 100)
    return { label: i18next.t('knowledge.docs.runDone'), color: "success", percent: 100 };
  if (doc.run === "4") return { label: i18next.t('knowledge.docs.runFailed'), color: "error", percent };
  if (doc.run === "2") return { label: i18next.t('knowledge.docs.runCancelled'), color: "warning", percent };
  return { label: i18next.t('knowledge.docs.runUnparsed'), color: "default", percent };
}

function metadataSummary(doc: KnowledgeDocument): string {
  const fields = doc.meta_fields;
  if (fields.length === 0) return "-";
  const pickStr = (v: unknown): string =>
    typeof v === "string" ? v : typeof v === "number" || typeof v === "boolean" || typeof v === "bigint" ? String(v) : "";
  const names = fields
    .map((item) => pickStr(item.name ?? item.key ?? item.field).trim())
    .filter(Boolean);
  if (names.length === 0) return i18next.t('knowledge.docs.metaCount', { n: fields.length });
  return (
    names.slice(0, 2).join("、") +
    (names.length > 2 ? i18next.t('knowledge.docs.metaMore', { n: names.length }) : "")
  );
}

function queueKey(file: File): string {
  return `${file.name}:${file.size}:${file.lastModified}`;
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

interface UploadModalProps {
  open: boolean;
  uploading: boolean;
  onClose: () => void;
  onUpload: (files: File[], autoParse: boolean) => Promise<void>;
}

function UploadModal({ open, uploading, onClose, onUpload }: UploadModalProps) {
  const { t } = useTranslation()
  const { styles } = useStyles();
  const [files, setFiles] = useState<File[]>([]);
  const [autoParse, setAutoParse] = useState(true);
  const [error, setError] = useState("");

  useEffect(() => {
    if (!open) {
      // eslint-disable-next-line react-hooks/set-state-in-effect -- reset upload-modal state on close so the next open starts clean
      setFiles([]);
       
      setError("");
       
      setAutoParse(true);
    }
  }, [open]);

  const addFiles = (incoming: File[]) => {
    setError("");
    setFiles((current) => {
      const merged = new Map(current.map((file) => [queueKey(file), file]));
      for (const file of incoming) merged.set(queueKey(file), file);
      return Array.from(merged.values());
    });
  };

  const submit = async () => {
    if (files.length === 0) {
      setError(i18next.t('knowledge.docs.chooseFirst'));
      return;
    }
    try {
      await onUpload(files, autoParse);
      onClose();
    } catch (err) {
      setError(parseApiError(err));
    }
  };

  return (
    <Modal
      title={i18next.t('knowledge.docs.uploadTitle')}
      open={open}
      onOk={submit}
      onCancel={onClose}
      confirmLoading={uploading}
      okText={error ? i18next.t('knowledge.docs.retryUpload') : i18next.t('knowledge.docs.startUpload')}
      cancelText={t('common.cancel')}
      width={680}
      destroyOnHidden
    >
      <Space
        orientation="vertical"
        size={12}
        style={{ width: "100%", marginTop: 8 }}
      >
        <Upload.Dragger
          multiple
          showUploadList={false}
          beforeUpload={(_file, fileList) => {
            addFiles(fileList);
            return Upload.LIST_IGNORE;
          }}
        >
          <p className="ant-upload-drag-icon">
            <UploadSimpleIcon size={26} />
          </p>
          <p className="ant-upload-text">{i18next.t('knowledge.docs.dragText')}</p>
          <p className="ant-upload-hint">
            i18next.t('knowledge.docs.queueHint')
          </p>
        </Upload.Dragger>

        <Switch
          checked={autoParse}
          onChange={setAutoParse}
          checkedChildren={i18next.t('knowledge.docs.parseAfterUpload')}
          unCheckedChildren={i18next.t('knowledge.docs.uploadOnly')}
        />

        {error ? (
          <Typography.Text type="danger">{error}</Typography.Text>
        ) : null}

        <div className={styles.queueList}>
          {files.length === 0 ? (
            <div className={styles.queueEmpty}>{i18next.t('knowledge.docs.queueEmpty')}</div>
          ) : (
            files.map((file) => (
              <div className={styles.queueItem} key={queueKey(file)}>
                <div>
                  <Typography.Text strong>{file.name}</Typography.Text>
                  <div className={styles.detailText}>
                    {formatBytes(file.size)}
                  </div>
                </div>
                <Button
                  size="small"
                  type="text"
                  danger
                  icon={<TrashIcon size={15} />}
                  onClick={() =>
                    { setFiles((current) =>
                      current.filter(
                        (item) => queueKey(item) !== queueKey(file),
                      ),
                    ); }
                  }
                />
              </div>
            ))
          )}
        </div>
      </Space>
    </Modal>
  );
}

export default function KnowledgeDocumentsPage() {
  const { t } = useTranslation()
  const { styles } = useStyles();
  const navigate = useNavigate();
  const { id = "" } = useParams();

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

  const query = useDocuments(id, {
    page,
    page_size: PAGE_SIZE,
    keywords,
    suffix: suffixFilter,
    run: runFilter,
    orderby: "create_time",
    desc: true,
  });
  const uploadDocuments = useUploadDocuments(id);
  const parseDocuments = useParseDocuments(id);
  const stopParsing = useStopParsingDocuments(id);
  const updateDocument = useUpdateDocument(id);
  const deleteDocuments = useDeleteDocuments(id);

  const documents = query.data?.documents ?? [];
  const total = query.data?.total ?? 0;
  const selectedIds = selectedRowKeys.map(String);
  const hasRunning = documents.some((doc) => doc.run === "1");

  useEffect(() => {
    if (!hasRunning) return;
    const timer = window.setInterval(() => {
      void query.refetch();
    }, 3500);
    return () => { window.clearInterval(timer); };
  }, [hasRunning, query]);

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

  const handleUpload = async (files: File[], autoParse: boolean) => {
    const docs = await uploadDocuments.mutateAsync(files);
    const ids = docs.map((doc) => doc.id).filter(Boolean);
    if (autoParse && ids.length > 0) {
      parseDocuments.mutate(ids);
    }
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
      window.setTimeout(() => { URL.revokeObjectURL(objectUrl); }, 0);
    } catch (err) {
      message.error(parseApiError(err));
    } finally {
      setDownloadingId(null);
    }
  };

  const bulkSwitch = async (enabled: boolean) => {
    await Promise.all(
      selectedIds.map((documentId) =>
        updateDocument.mutateAsync({ documentId, patch: { enabled } }),
      ),
    );
    setSelectedRowKeys([]);
    message.success(enabled ? t('knowledge.docs.bulkEnabled') : t('knowledge.docs.bulkDisabled'));
  };

  const bulkParse = () => {
    parseDocuments.mutate(selectedIds);
    setSelectedRowKeys([]);
  };

  const bulkStop = () => {
    stopParsing.mutate(selectedIds);
    setSelectedRowKeys([]);
  };

  const bulkDelete = () => {
    deleteDocuments.mutate(selectedIds);
    setSelectedRowKeys([]);
  };

  const rowSelection: TableRowSelection<KnowledgeDocument> = {
    selectedRowKeys,
    onChange: setSelectedRowKeys,
    preserveSelectedRowKeys: true,
  };

  const columns: ColumnsType<KnowledgeDocument> = [
    {
      title: t('knowledge.docs.docCol'),
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
              <span className={styles.primaryText}>{value || t('knowledge.docs.unnamed')}</span>
            </Tooltip>
            <span className={styles.secondaryText}>
              {/* eslint-disable-next-line @typescript-eslint/no-unnecessary-condition -- runtime defense: API may omit suffix */}
              {(record.suffix ?? record.type ?? "file")
                .toUpperCase()}{" "}
              · {formatBytes(record.size)}
            </span>
          </span>
        </div>
      ),
    },
    {
      title: t('knowledge.docs.parserCol'),
      key: "parser_id",
      width: 120,
      render: (_, record) => (
        <Tag color="blue">{record.parser_id || "naive"}</Tag>
      ),
    },
    {
      title: "Metadata",
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
      title: t('knowledge.docs.chunkCol'),
      dataIndex: "chunk_num",
      key: "chunk_num",
      width: 80,
      align: "right",
    },
    {
      title: t('knowledge.docs.statusCol'),
      key: "status",
      width: 170,
      render: (_, record) => {
        const meta = statusMeta(record);
        return (
          <button
            type="button"
            className={styles.statusButton}
            aria-label={t('knowledge.docs.viewStatusAria', { label: meta.label })}
            onClick={() => { setStatusDoc(record); }}
          >
            <Space orientation="vertical" size={3} style={{ width: "100%" }}>
              <Badge status={meta.color} text={meta.label} />
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
      title: t('knowledge.docs.enabledCol'),
      key: "enabled",
      width: 76,
      render: (_, record) =>
        canWrite ? (
          <Switch
            size="small"
            checked={record.enabled}
            onChange={(checked) =>
              { updateDocument.mutate({
                documentId: record.id,
                patch: { enabled: checked },
              }); }
            }
          />
        ) : (
          <Tag color={record.enabled ? "success" : "default"}>
            {record.enabled ? t('knowledge.docs.enabled') : t('knowledge.docs.disabled')}
          </Tag>
        ),
    },
    {
      title: t('knowledge.docs.createdAt'),
      key: "create_time",
      width: 136,
      render: (_, record) =>
        formatTime(record.create_time ?? record.create_date),
    },
    {
      title: t('knowledge.docs.actions'),
      key: "action",
      width: 270,
      fixed: "right",
      render: (_, record) => (
        <Space size={4} wrap>
          {canWrite &&
            (record.run === "1" ? (
              <Button
                type="link"
                size="small"
                aria-label={t('knowledge.docs.stopParseAria', { name: record.name })}
                icon={<StopIcon size={14} />}
                onClick={() => { stopParsing.mutate([record.id]); }}
              >
                {t('knowledge.docs.stop')}
              </Button>
            ) : (
              <Button
                type="link"
                size="small"
                aria-label={t('knowledge.docs.parseDocAria', { name: record.name })}
                icon={<PlayIcon size={14} />}
                onClick={() => { parseDocuments.mutate([record.id]); }}
              >
                {t('knowledge.docs.parse')}
              </Button>
            ))}
          <Button
            type="link"
            size="small"
            icon={<ListBulletsIcon size={14} />}
            onClick={async () =>
              { await navigate(`/knowledge/${id}/documents/${record.id}/chunks`); }
            }
          >
            {t('knowledge.docs.chunkNav')}
          </Button>
          <Button
            type="link"
            size="small"
            aria-label={t('knowledge.docs.downloadAria', { name: record.name })}
            icon={<DownloadSimpleIcon size={14} />}
            loading={downloadingId === record.id}
            onClick={() => void handleDownload(record)}
          >
            {t('common.download')}
          </Button>
          {canWrite && (
            <>
              <Button
                type="link"
                size="small"
                icon={<PencilSimpleIcon size={14} />}
                onClick={() => {
                  setRenaming(record);
                  setRenameValue(record.name);
                }}
              >
                {t('knowledge.docs.rename')}
              </Button>
              <Popconfirm
                title={t('scenes.deleteConfirmTitle')}
                description={t('knowledge.docs.deleteDesc', { name: record.name })}
                okText={t('common.delete')}
                okButtonProps={{ danger: true }}
                cancelText={t('common.cancel')}
                onConfirm={() => { deleteDocuments.mutate([record.id]); }}
              >
                <Button type="link" size="small" danger>
                  {t('common.delete')}
                </Button>
              </Popconfirm>
            </>
          )}
        </Space>
      ),
    },
  ];

  return (
    <div className={styles.shell}>
      <div className={styles.toolbar}>
        <div className={styles.filters}>
          <Input.Search
            placeholder={t('knowledge.docs.searchPh')}
            allowClear
            style={{ width: 260 }}
            onSearch={(value) => {
              setKeywords(value.trim());
              setPage(1);
            }}
          />
          <Select
            mode="multiple"
            allowClear
            maxTagCount="responsive"
            placeholder={t('knowledge.docs.statusPh')}
            style={{ minWidth: 160 }}
            options={STATUS_OPTIONS}
            value={runFilter}
            onChange={(value) => {
              setRunFilter(value);
              setPage(1);
            }}
          />
          <Select
            mode="multiple"
            allowClear
            maxTagCount="responsive"
            placeholder={t('knowledge.docs.fileTypePh')}
            style={{ minWidth: 160 }}
            options={SUFFIX_OPTIONS.map((value) => ({
              label: value.toUpperCase(),
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
              setPage(1);
            }}
          >
            {t('knowledge.docs.reset')}
          </Button>
        </div>
        <div className={styles.actions}>
          <Button
            icon={<ArrowsClockwiseIcon size={16} />}
            loading={query.isFetching}
            onClick={() => query.refetch()}
          >
            {t('knowledge.docs.refresh')}
          </Button>
          {canWrite && (
            <PrimaryButton
              icon={<UploadSimpleIcon size={16} />}
              onClick={() => { setUploadOpen(true); }}
            >
              {t('knowledge.docs.uploadBtn')}
            </PrimaryButton>
          )}
        </div>
      </div>

      {canWrite && selectedIds.length > 0 ? (
        <div className={styles.bulkBar}>
          <Typography.Text strong>
            {t('knowledge.docs.selectedN', { n: selectedIds.length })}
          </Typography.Text>
          <Space wrap>
            <Button
              size="small"
              aria-label={t('knowledge.docs.bulkEnableAria')}
              onClick={() => void bulkSwitch(true)}
            >
              {t('knowledge.docs.enabled')}
            </Button>
            <Button
              size="small"
              aria-label={t('knowledge.docs.bulkDisableAria')}
              onClick={() => void bulkSwitch(false)}
            >
              {t('knowledge.docs.disabled')}
            </Button>
            <Button
              size="small"
              aria-label={t('knowledge.docs.bulkParseAria')}
              icon={<PlayIcon size={14} />}
              onClick={bulkParse}
            >
              {t('knowledge.docs.parse')}
            </Button>
            <Button
              size="small"
              aria-label={t('knowledge.docs.bulkStopParseAria')}
              icon={<StopIcon size={14} />}
              onClick={bulkStop}
            >
              {t('knowledge.docs.stop')}
            </Button>
            <Popconfirm
              title={t('knowledge.docs.deleteSelectedTitle')}
              description={t('knowledge.docs.deleteSelectedDesc', { n: selectedIds.length })}
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
              onClick={() => { setSelectedRowKeys([]); }}
            >
              {t('knowledge.docs.clearSelection')}
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
                keywords
                  ? t('knowledge.docs.emptyNoMatch')
                  : canWrite
                    ? t('knowledge.docs.emptyNoUpload')
                    : t('knowledge.docs.emptyNone')
              }
            />
          ),
        }}
        pagination={{
          current: page,
          pageSize: PAGE_SIZE,
          total,
          showTotal: (count) => t('common.totalItems', { total: count }),
          onChange: (next) => { setPage(next); },
        }}
      />

      <UploadModal
        open={uploadOpen}
        uploading={uploadDocuments.isPending}
        onClose={() => { setUploadOpen(false); }}
        onUpload={handleUpload}
      />

      <Modal
        title={t('knowledge.docs.statusCol')}
        open={!!statusDoc}
        onCancel={() => { setStatusDoc(null); }}
        footer={<Button onClick={() => { setStatusDoc(null); }}>{t('knowledge.docs.close')}</Button>}
        width={640}
        destroyOnHidden
      >
        {statusDoc ? (
          <Space
            orientation="vertical"
            size={16}
            style={{ width: "100%", marginTop: 8 }}
          >
            <Progress
              percent={statusMeta(statusDoc).percent}
              status={statusDoc.run === "4" ? "exception" : undefined}
            />
            <Descriptions column={1} size="small" bordered>
              <Descriptions.Item label={t('knowledge.docs.statusDocCol')}>
                {statusDoc.name}
              </Descriptions.Item>
              <Descriptions.Item label={t('knowledge.docs.statusColState')}>
                {statusMeta(statusDoc).label}
              </Descriptions.Item>
              <Descriptions.Item label={t('knowledge.docs.progressMsg')}>
                {statusDoc.progress_msg || "-"}
              </Descriptions.Item>
              <Descriptions.Item label={t('knowledge.docs.statusChunkCount')}>
                {statusDoc.chunk_num}
              </Descriptions.Item>
              <Descriptions.Item label={t('knowledge.docs.elapsed')}>
                {statusDoc.process_duration
                  ? `${statusDoc.process_duration}s`
                  : "-"}
              </Descriptions.Item>
              <Descriptions.Item label={t('knowledge.docs.startTime')}>
                {formatTime(
                  statusDoc.process_begin_at ??
                    statusDoc.create_time ??
                    statusDoc.create_date,
                )}
              </Descriptions.Item>
              <Descriptions.Item label={t('knowledge.docs.errorSummary')}>
                {statusDoc.run === "4"
                  ? statusDoc.progress_msg || t('knowledge.docs.parseFail')
                  : "-"}
              </Descriptions.Item>
            </Descriptions>
          </Space>
        ) : null}
      </Modal>

      <Modal
        title={t('knowledge.docs.renameTitle')}
        open={!!renaming}
        onOk={submitRename}
        onCancel={() => { setRenaming(null); }}
        confirmLoading={updateDocument.isPending}
        okText={t('knowledge.docs.saveBtn')}
        cancelText={t('common.cancel')}
        destroyOnHidden
      >
        <Input
          value={renameValue}
          onChange={(event) => { setRenameValue(event.target.value); }}
          onPressEnter={submitRename}
          placeholder={t('knowledge.docs.renamePh')}
          style={{ marginTop: 8 }}
        />
      </Modal>
    </div>
  );
}
