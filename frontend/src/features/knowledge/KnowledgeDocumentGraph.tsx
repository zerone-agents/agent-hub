import { Alert, Button, Modal, Space, Spin, Typography } from "antd";
import { useTranslation } from "react-i18next";
import { useKnowledgeResource } from "@/queries/useKnowledgeManagement";
import { parseApiError } from "@/api/client";
import type { KnowledgeObject } from "@/api/knowledgeManagement";
import KnowledgeGraphExplorer from "./KnowledgeGraphExplorer";

export default function KnowledgeDocumentGraph({
  datasetId,
  documentId,
  name,
  onClose,
}: {
  datasetId: string;
  documentId: string;
  name: string;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const query = useKnowledgeResource<KnowledgeObject>(datasetId, "graph", {
    doc_id: documentId,
  });
  return (
    <Modal
      open
      title={t("knowledge.manage.documentGraphTitle", { name })}
      onCancel={onClose}
      width={900}
      footer={<Button onClick={onClose}>{t("knowledge.docs.close")}</Button>}
    >
      <Space
        orientation="vertical"
        style={{ width: "100%", minWidth: 0 }}
        size="middle"
      >
        <Typography.Text type="secondary">
          {t("knowledge.manage.documentGraphHint")}
        </Typography.Text>
        <Button loading={query.isFetching} onClick={() => void query.refetch()}>
          {t("knowledge.docs.refresh")}
        </Button>
        {query.error ? (
          <Alert type="error" showIcon title={parseApiError(query.error)} />
        ) : query.isLoading ? (
          <Spin />
        ) : (
          <KnowledgeGraphExplorer
            key={`${datasetId}:${documentId}`}
            data={query.data}
            loading={query.isFetching}
            emptyText={t("knowledge.manage.noDocumentGraph")}
          />
        )}
      </Space>
    </Modal>
  );
}
