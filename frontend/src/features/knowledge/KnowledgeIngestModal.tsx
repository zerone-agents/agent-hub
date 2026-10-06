import { useRef, useState } from "react";
import { Alert, Checkbox, Modal, Space, Tag, Typography } from "antd";
import { useTranslation } from "react-i18next";
import { parseApiError } from "@/api/client";
import { usePrimaryButtonStyle } from "@/components/PrimaryButton";
import type { DocumentIngestInput } from "@/api/knowledge";
import { useDocumentWorkflowText } from "./documentWorkflowText";

export default function KnowledgeIngestModal({
  docIds,
  names = {},
  onClose,
  onSubmit,
}: {
  docIds: string[];
  names?: Record<string, string>;
  onClose: () => void;
  onSubmit: (input: DocumentIngestInput) => Promise<unknown>;
}) {
  const { t } = useTranslation();
  const style = usePrimaryButtonStyle();
  const [removeChunks, setRemoveChunks] = useState(false);
  const [applyKB, setApplyKB] = useState(false);
  const [busy, setBusy] = useState(false);
  const text = useDocumentWorkflowText();
  const [results, setResults] = useState<Record<string, string>>({});
  const lock = useRef(false);
  const accepted = Object.values(results).filter(
    (value) => value === "",
  ).length;
  const submit = async () => {
    if (lock.current) return;
    lock.current = true;
    setBusy(true);
    const next = { ...results };
    try {
      for (const id of docIds) {
        if (next[id] === "") continue;
        try {
          await onSubmit({
            doc_ids: [id],
            run: 1,
            delete: removeChunks,
            apply_kb: applyKB,
          });
          next[id] = "";
        } catch (err) {
          next[id] = parseApiError(err);
        }
        setResults({ ...next });
      }
      if (docIds.every((id) => next[id] === "")) onClose();
    } finally {
      lock.current = false;
      setBusy(false);
    }
  };
  return (
    <Modal
      open
      title={t("knowledge.manage.reparseTitle", { n: docIds.length })}
      onCancel={() => {
        if (!lock.current) onClose();
      }}
      onOk={submit}
      confirmLoading={busy}
      keyboard={!busy}
      closable={!busy}
      mask={{ closable: !busy }}
      cancelButtonProps={{ disabled: busy }}
      okButtonProps={{ className: style.root, disabled: docIds.length === 0 }}
      okText={accepted ? text("retry") : t("knowledge.manage.startReparse")}
    >
      <Space orientation="vertical" size="middle" style={{ width: "100%" }}>
        <Typography.Paragraph>
          {t("knowledge.manage.ingestAcceptanceHint")}
        </Typography.Paragraph>
        {Object.keys(results).length > 0 && (
          <div aria-live="polite">
            <Typography.Text strong>{text("outcomes")}</Typography.Text>
            {docIds.map((id) => (
              <div key={id} style={{ marginTop: 8, overflowWrap: "anywhere" }}>
                <Typography.Text>{names[id] || id}</Typography.Text>{" "}
                <Tag
                  color={
                    results[id] === ""
                      ? "success"
                      : results[id]
                        ? "error"
                        : "default"
                  }
                >
                  {results[id] === ""
                    ? text("accepted")
                    : results[id] || text("pending")}
                </Tag>
              </div>
            ))}
          </div>
        )}
        <Checkbox
          checked={removeChunks}
          disabled={busy || accepted > 0}
          onChange={(e) => {
            setRemoveChunks(e.target.checked);
          }}
        >
          {t("knowledge.manage.deleteOldChunks")}
        </Checkbox>
        {removeChunks && (
          <Alert
            type="warning"
            showIcon
            title={t("knowledge.manage.deleteOldChunksHint")}
          />
        )}
        <Checkbox
          checked={applyKB}
          disabled={busy || accepted > 0}
          onChange={(e) => {
            setApplyKB(e.target.checked);
          }}
        >
          {t("knowledge.manage.applyKBMetadata")}
        </Checkbox>
        <Typography.Text type="secondary">
          {t("knowledge.manage.applyKBMetadataHint")}
        </Typography.Text>
      </Space>
    </Modal>
  );
}
