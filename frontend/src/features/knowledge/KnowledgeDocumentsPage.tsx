import { useEffect, useState } from "react";
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
  ArrowsClockwise,
  DownloadSimple,
  FileText,
  FunnelSimple,
  ListBullets,
  PencilSimple,
  Play,
  Stop,
  Trash,
  UploadSimple,
} from "@phosphor-icons/react";
import { createStyles } from "antd-style";
import { useNavigate, useParams } from "react-router-dom";
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
  { label: "解析中", value: "1" },
  { label: "已取消", value: "2" },
  { label: "已完成", value: "3" },
  { label: "失败", value: "4" },
  { label: "未解析", value: "0" },
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
  const percent = Math.round((doc.progress ?? 0) * 100);
  if (doc.run === "1") return { label: "解析中", color: "processing", percent };
  if (doc.run === "3" || percent >= 100)
    return { label: "已完成", color: "success", percent: 100 };
  if (doc.run === "4") return { label: "失败", color: "error", percent };
  if (doc.run === "2") return { label: "已取消", color: "warning", percent };
  return { label: "未解析", color: "default", percent };
}

function metadataSummary(doc: KnowledgeDocument): string {
  const fields = doc.meta_fields ?? [];
  if (fields.length === 0) return "-";
  const names = fields
    .map((item) => String(item.name ?? item.key ?? item.field ?? "").trim())
    .filter(Boolean);
  if (names.length === 0) return `${fields.length} 项`;
  return (
    names.slice(0, 2).join("、") +
    (names.length > 2 ? ` 等 ${names.length} 项` : "")
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
  const { styles } = useStyles();
  const [files, setFiles] = useState<File[]>([]);
  const [autoParse, setAutoParse] = useState(true);
  const [error, setError] = useState("");

  useEffect(() => {
    if (!open) {
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
      setError("请先选择要上传的文件");
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
      title="上传文档"
      open={open}
      onOk={submit}
      onCancel={onClose}
      confirmLoading={uploading}
      okText={error ? "重试上传" : "开始上传"}
      cancelText="取消"
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
            <UploadSimple size={26} />
          </p>
          <p className="ant-upload-text">拖入文件，或点击选择</p>
          <p className="ant-upload-hint">
            队列会在确认后统一上传，避免误触即提交。
          </p>
        </Upload.Dragger>

        <Switch
          checked={autoParse}
          onChange={setAutoParse}
          checkedChildren="上传后解析"
          unCheckedChildren="仅上传"
        />

        {error ? (
          <Typography.Text type="danger">{error}</Typography.Text>
        ) : null}

        <div className={styles.queueList}>
          {files.length === 0 ? (
            <div className={styles.queueEmpty}>队列为空</div>
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
                  icon={<Trash size={15} />}
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
    message.success(enabled ? "已批量启用文档" : "已批量停用文档");
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
      title: "文档",
      dataIndex: "name",
      key: "name",
      width: 280,
      ellipsis: true,
      render: (value: string, record) => (
        <div className={styles.documentName}>
          <span className={styles.fileIcon}>
            <FileText size={18} weight="duotone" />
          </span>
          <span className={styles.nameText}>
            <Tooltip title={value} placement="topLeft">
              <span className={styles.primaryText}>{value || "未命名"}</span>
            </Tooltip>
            <span className={styles.secondaryText}>
              {(record.suffix ?? record.type ?? "file")
                .toString()
                .toUpperCase()}{" "}
              · {formatBytes(record.size)}
            </span>
          </span>
        </div>
      ),
    },
    {
      title: "解析方法",
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
      title: "分块",
      dataIndex: "chunk_num",
      key: "chunk_num",
      width: 80,
      align: "right",
    },
    {
      title: "解析状态",
      key: "status",
      width: 170,
      render: (_, record) => {
        const meta = statusMeta(record);
        return (
          <button
            type="button"
            className={styles.statusButton}
            aria-label={`查看解析状态：${meta.label}`}
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
      title: "启用",
      key: "enabled",
      width: 76,
      render: (_, record) => (
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
      ),
    },
    {
      title: "创建时间",
      key: "create_time",
      width: 136,
      render: (_, record) =>
        formatTime(record.create_time ?? record.create_date),
    },
    {
      title: "操作",
      key: "action",
      width: 270,
      fixed: "right",
      render: (_, record) => (
        <Space size={4} wrap>
          {record.run === "1" ? (
            <Button
              type="link"
              size="small"
              aria-label={`停止解析 ${record.name}`}
              icon={<Stop size={14} />}
              onClick={() => { stopParsing.mutate([record.id]); }}
            >
              停止
            </Button>
          ) : (
            <Button
              type="link"
              size="small"
              aria-label={`解析文档 ${record.name}`}
              icon={<Play size={14} />}
              onClick={() => { parseDocuments.mutate([record.id]); }}
            >
              解析
            </Button>
          )}
          <Button
            type="link"
            size="small"
            icon={<ListBullets size={14} />}
            onClick={() =>
              { navigate(`/knowledge/${id}/documents/${record.id}/chunks`); }
            }
          >
            切片
          </Button>
          <Button
            type="link"
            size="small"
            aria-label={`下载 ${record.name}`}
            icon={<DownloadSimple size={14} />}
            loading={downloadingId === record.id}
            onClick={() => void handleDownload(record)}
          >
            下载
          </Button>
          <Button
            type="link"
            size="small"
            icon={<PencilSimple size={14} />}
            onClick={() => {
              setRenaming(record);
              setRenameValue(record.name);
            }}
          >
            重命名
          </Button>
          <Popconfirm
            title="确认删除？"
            description={`删除文档 "${record.name}"？`}
            okText="删除"
            okButtonProps={{ danger: true }}
            cancelText="取消"
            onConfirm={() => { deleteDocuments.mutate([record.id]); }}
          >
            <Button type="link" size="small" danger>
              删除
            </Button>
          </Popconfirm>
        </Space>
      ),
    },
  ];

  return (
    <div className={styles.shell}>
      <div className={styles.toolbar}>
        <div className={styles.filters}>
          <Input.Search
            placeholder="搜索文档名称"
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
            placeholder="解析状态"
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
            placeholder="文件类型"
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
            icon={<FunnelSimple size={16} />}
            onClick={() => {
              setKeywords("");
              setRunFilter([]);
              setSuffixFilter([]);
              setPage(1);
            }}
          >
            重置
          </Button>
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
            icon={<UploadSimple size={16} />}
            onClick={() => { setUploadOpen(true); }}
          >
            上传文档
          </Button>
        </div>
      </div>

      {selectedIds.length > 0 ? (
        <div className={styles.bulkBar}>
          <Typography.Text strong>
            已选择 {selectedIds.length} 个文档
          </Typography.Text>
          <Space wrap>
            <Button
              size="small"
              aria-label="批量启用文档"
              onClick={() => void bulkSwitch(true)}
            >
              启用
            </Button>
            <Button
              size="small"
              aria-label="批量停用文档"
              onClick={() => void bulkSwitch(false)}
            >
              停用
            </Button>
            <Button
              size="small"
              aria-label="批量解析文档"
              icon={<Play size={14} />}
              onClick={bulkParse}
            >
              解析
            </Button>
            <Button
              size="small"
              aria-label="批量停止解析文档"
              icon={<Stop size={14} />}
              onClick={bulkStop}
            >
              停止
            </Button>
            <Popconfirm
              title="确认删除选中文档？"
              description={`将删除 ${selectedIds.length} 个文档。`}
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
              onClick={() => { setSelectedRowKeys([]); }}
            >
              清除选择
            </Button>
          </Space>
        </div>
      ) : null}

      <BorderedTable<KnowledgeDocument>
        columns={columns}
        dataSource={documents}
        rowKey="id"
        rowSelection={rowSelection}
        size="middle"
        loading={query.isLoading}
        scroll={{ x: 1180 }}
        locale={{
          emptyText: (
            <Empty
              description={
                keywords ? "未找到匹配的文档" : "还没有文档，点击上传"
              }
            />
          ),
        }}
        pagination={{
          current: page,
          pageSize: PAGE_SIZE,
          total,
          showTotal: (count) => `共 ${count} 条`,
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
        title="解析状态"
        open={!!statusDoc}
        onCancel={() => { setStatusDoc(null); }}
        footer={<Button onClick={() => { setStatusDoc(null); }}>关闭</Button>}
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
              <Descriptions.Item label="文档">
                {statusDoc.name}
              </Descriptions.Item>
              <Descriptions.Item label="状态">
                {statusMeta(statusDoc).label}
              </Descriptions.Item>
              <Descriptions.Item label="进度消息">
                {statusDoc.progress_msg || "-"}
              </Descriptions.Item>
              <Descriptions.Item label="分块数">
                {statusDoc.chunk_num}
              </Descriptions.Item>
              <Descriptions.Item label="耗时">
                {statusDoc.process_duration
                  ? `${statusDoc.process_duration}s`
                  : "-"}
              </Descriptions.Item>
              <Descriptions.Item label="开始时间">
                {formatTime(
                  statusDoc.process_begin_at ??
                    statusDoc.create_time ??
                    statusDoc.create_date,
                )}
              </Descriptions.Item>
              <Descriptions.Item label="错误摘要">
                {statusDoc.run === "4"
                  ? statusDoc.progress_msg || "解析失败"
                  : "-"}
              </Descriptions.Item>
            </Descriptions>
          </Space>
        ) : null}
      </Modal>

      <Modal
        title="重命名文档"
        open={!!renaming}
        onOk={submitRename}
        onCancel={() => { setRenaming(null); }}
        confirmLoading={updateDocument.isPending}
        okText="保存"
        cancelText="取消"
        destroyOnHidden
      >
        <Input
          value={renameValue}
          onChange={(event) => { setRenameValue(event.target.value); }}
          onPressEnter={submitRename}
          placeholder="输入新的文档名称"
          style={{ marginTop: 8 }}
        />
      </Modal>
    </div>
  );
}
