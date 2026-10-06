import axios from "axios";
import { useRef, useState } from "react";
import {
  Alert,
  Button,
  Modal,
  Space,
  Switch,
  Tag,
  Typography,
  Upload,
} from "antd";
import { UploadSimpleIcon, TrashIcon } from "@phosphor-icons/react";
import { useTranslation } from "react-i18next";
import type { KnowledgeDocument } from "@/api/knowledge";
import { parseApiError } from "@/api/client";
import { usePrimaryButtonStyle } from "@/components/PrimaryButton";
import { useDocumentWorkflowText } from "./documentWorkflowText";

type UploadState =
  | "queued"
  | "uploading"
  | "uploaded"
  | "submitting"
  | "accepted"
  | "upload_unknown"
  | "upload_failed"
  | "parse_failed";
interface Entry {
  key: string;
  file: File;
  state: UploadState;
  document?: KnowledgeDocument;
  error?: string;
}
const keyFor = (file: File) =>
  `${file.webkitRelativePath || file.name}:${file.size}:${file.lastModified}`;
export default function KnowledgeDocumentUpload({
  onClose,
  onUpload,
  onParse,
}: {
  onClose: () => void;
  onUpload: (file: File) => Promise<KnowledgeDocument>;
  onParse: (document: KnowledgeDocument) => Promise<unknown>;
}) {
  const { t } = useTranslation();
  const text = useDocumentWorkflowText();
  const style = usePrimaryButtonStyle();
  const [entries, setEntries] = useState<Entry[]>([]);
  const [autoParse, setAutoParse] = useState(true);
  const [busy, setBusy] = useState(false);
  const lock = useRef(false);
  const change = (key: string, patch: Partial<Entry>) => {
    setEntries((current) =>
      current.map((entry) =>
        entry.key === key ? { ...entry, ...patch } : entry,
      ),
    );
  };
  const add = (files: File[]) => {
    if (lock.current) return;
    setEntries((current) => {
      const merged = new Map(current.map((entry) => [entry.key, entry]));
      for (const file of files)
        if (!merged.has(keyFor(file)))
          merged.set(keyFor(file), {
            key: keyFor(file),
            file,
            state: "queued",
          });
      return Array.from(merged.values());
    });
  };
  const pending = entries.filter(
    (entry) =>
      entry.state !== "accepted" &&
      entry.state !== "upload_unknown" &&
      (!entry.document || autoParse),
  );
  const run = async (items: Entry[]) => {
    if (
      lock.current ||
      !items.length ||
      items.some((entry) => entry.state === "upload_unknown")
    )
      return;
    lock.current = true;
    setBusy(true);
    try {
      for (const entry of items) {
        let doc = entry.document;
        if (!doc) {
          change(entry.key, { state: "uploading", error: undefined });
          try {
            doc = await onUpload(entry.file);
            if (!doc.id) throw new Error(text("missing"));
            change(entry.key, { state: "uploaded", document: doc });
          } catch (err) {
            change(entry.key, {
              state:
                !axios.isAxiosError(err) ||
                !err.response ||
                err.response.status >= 500
                  ? "upload_unknown"
                  : "upload_failed",
              error: parseApiError(err),
            });
            continue;
          }
        }
        if (autoParse) {
          change(entry.key, { state: "submitting", error: undefined });
          try {
            await onParse(doc);
            change(entry.key, { state: "accepted" });
          } catch (err) {
            change(entry.key, {
              state: "parse_failed",
              error: parseApiError(err),
            });
          }
        }
      }
    } finally {
      lock.current = false;
      setBusy(false);
    }
  };
  return (
    <Modal
      open
      title={t("knowledge.docs.uploadTitle")}
      onCancel={() => {
        if (!lock.current) onClose();
      }}
      onOk={() => void run(pending)}
      width={720}
      closable={!busy}
      mask={{ closable: !busy }}
      keyboard={!busy}
      confirmLoading={busy}
      cancelButtonProps={{ disabled: busy }}
      cancelText={t("knowledge.docs.close")}
      okButtonProps={{ className: style.root, disabled: pending.length === 0 }}
      okText={
        entries.some((e) => e.error)
          ? text("retry")
          : t("knowledge.docs.startUpload")
      }
    >
      <Space orientation="vertical" size="middle" style={{ width: "100%" }}>
        {entries.length > 0 && (
          <Typography.Text aria-live="polite">
            {text("uploadSummary", {
              total: entries.length,
              uploaded: entries.filter((e) => e.document).length,
              accepted: entries.filter((e) => e.state === "accepted").length,
              failed: entries.filter((e) => e.error).length,
            })}
          </Typography.Text>
        )}
        <Alert type="info" showIcon title={text("hint")} />
        <Upload.Dragger
          multiple
          disabled={busy}
          showUploadList={false}
          beforeUpload={(_file, files) => {
            add(files);
            return Upload.LIST_IGNORE;
          }}
        >
          <UploadSimpleIcon size={26} />
          <p>{t("knowledge.docs.dragText")}</p>
        </Upload.Dragger>
        <Space wrap>
          <Upload
            directory
            multiple
            disabled={busy}
            showUploadList={false}
            beforeUpload={(_file, files) => {
              add(files);
              return Upload.LIST_IGNORE;
            }}
          >
            <Button disabled={busy}>{text("folder")}</Button>
          </Upload>
          <Switch
            checked={autoParse}
            disabled={busy}
            onChange={setAutoParse}
            aria-label={t("knowledge.docs.parseAfterUpload")}
            checkedChildren={t("knowledge.docs.parseAfterUpload")}
            unCheckedChildren={t("knowledge.docs.uploadOnly")}
          />
        </Space>
        <div
          aria-live="polite"
          style={{ maxHeight: "45vh", overflowY: "auto" }}
        >
          {!entries.length && (
            <Typography.Text type="secondary">
              {t("knowledge.docs.queueEmpty")}
            </Typography.Text>
          )}
          {entries.map((entry) => (
            <div
              key={entry.key}
              style={{
                padding: "12px 0",
                borderBottom: "1px solid var(--ant-color-border-secondary)",
                display: "flex",
                gap: 8,
                alignItems: "flex-start",
                flexWrap: "wrap",
              }}
            >
              <div style={{ flex: "1 1 200px", minWidth: 0 }}>
                <Typography.Text strong style={{ overflowWrap: "anywhere" }}>
                  {entry.file.webkitRelativePath || entry.file.name}
                </Typography.Text>
                <div>
                  <Tag
                    color={
                      entry.error
                        ? "error"
                        : entry.state === "accepted"
                          ? "success"
                          : "default"
                    }
                  >
                    {text(entry.state)}
                  </Tag>
                  <Typography.Text type="secondary">
                    {Math.ceil(entry.file.size / 1024)} KB
                  </Typography.Text>
                </div>
                {entry.state === "upload_unknown" && (
                  <Typography.Paragraph type="warning">
                    {text("verifyUpload")}
                  </Typography.Paragraph>
                )}
                {entry.error && (
                  <Typography.Text type="danger">{entry.error}</Typography.Text>
                )}
              </div>
              {entry.error && entry.state !== "upload_unknown" && (
                <Button
                  size="small"
                  disabled={busy}
                  onClick={() => void run([entry])}
                >
                  {text("retry")}
                </Button>
              )}
              {entry.state === "upload_unknown" && (
                <Button
                  size="small"
                  disabled={busy}
                  onClick={() => {
                    change(entry.key, { state: "queued", error: undefined });
                  }}
                >
                  {text("uploadAgain")}
                </Button>
              )}
              {!entry.document && (
                <Button
                  type="text"
                  danger
                  disabled={busy}
                  aria-label={`${text("remove")}: ${entry.file.name}`}
                  icon={<TrashIcon size={16} />}
                  onClick={() => {
                    setEntries((current) =>
                      current.filter((item) => item.key !== entry.key),
                    );
                  }}
                />
              )}
            </div>
          ))}
        </div>
      </Space>
    </Modal>
  );
}
