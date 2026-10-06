import { useState } from "react";
import {
  Alert,
  Collapse,
  Button,
  Form,
  Space,
  Popconfirm,
  Spin,
  Switch,
  Table,
  Typography,
} from "antd";
import { useTranslation } from "react-i18next";
import {
  useKnowledgeMetadataInventory,
  useKnowledgeResource,
} from "@/queries/useKnowledgeManagement";
import {
  containsKnowledgeFields,
  knowledgeRequest,
} from "@/api/knowledgeManagement";
import MetadataTemplateEditor from "./MetadataTemplateEditor";
import {
  metadataFields,
  metadataReadbackMatches,
  parseMetadataDefinition,
  type MetadataDefinition,
  type MetadataField,
} from "./metadata";
import { parseApiError } from "@/api/client";
import PrimaryButton from "@/components/PrimaryButton";
import { useMetadataTranslation } from "./useMetadataTranslation";
import MetadataBatchEditor from "./MetadataBatchEditor";
import { useCanWrite } from "@/hooks/useCanWrite";

interface MetadataConfig {
  enabled?: boolean;
  fields?: MetadataField[];
  metadata?: MetadataDefinition;
  built_in_metadata?: MetadataField[];
}
function MetadataConfigurationForm({
  id,
  config,
  onSaved,
  readError,
}: {
  id: string;
  config: MetadataConfig;
  onSaved: () => void;
  readError?: unknown;
}) {
  const { t } = useTranslation();
  const canWrite = useCanWrite();
  const { t: mt } = useMetadataTranslation();
  const [form] = Form.useForm<{
    enabled: boolean;
    metadata: string;
    built_in_metadata: string;
  }>();
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [saved, setSaved] = useState(false);
  const [snapshot, setSnapshot] = useState(config);
  const initialValues = (snapshot: MetadataConfig) => ({
    enabled: snapshot.enabled ?? true,
    metadata: JSON.stringify(
      snapshot.metadata ?? snapshot.fields ?? [],
      null,
      2,
    ),
    built_in_metadata: JSON.stringify(
      snapshot.built_in_metadata ?? [],
      null,
      2,
    ),
  });
  const definitionChanged = (
    name: "metadata" | "built_in_metadata",
    text: string,
  ): boolean => {
    if (!form.isFieldTouched(name)) return false;
    const original =
      name === "metadata"
        ? (snapshot.metadata ?? snapshot.fields)
        : snapshot.built_in_metadata;
    try {
      const draft: unknown = JSON.parse(text);
      return !metadataReadbackMatches(draft, original ?? []);
    } catch {
      return true;
    }
  };
  const validate =
    (name: "metadata" | "built_in_metadata", arrayOnly = false) =>
    (_: unknown, value: string) => {
      if (!definitionChanged(name, value)) return Promise.resolve();
      try {
        parseMetadataDefinition(value, arrayOnly);
        return Promise.resolve();
      } catch {
        return Promise.reject(
          new Error(t("knowledge.manage.metadataDefinitionInvalid")),
        );
      }
    };
  return (
    <Form
      form={form}
      layout="vertical"
      disabled={!canWrite || saving || Boolean(readError)}
      initialValues={initialValues(config)}
      onValuesChange={() => {
        setSaved(false);
      }}
      onFinish={async (values) => {
        if (!canWrite || readError) return;
        setSaving(true);
        setError("");
        setSaved(false);
        try {
          const originalMetadata = snapshot.metadata ?? snapshot.fields;
          const metadataChanged = definitionChanged(
            "metadata",
            values.metadata,
          );
          const builtInChanged = definitionChanged(
            "built_in_metadata",
            values.built_in_metadata,
          );
          const metadata = metadataChanged
            ? parseMetadataDefinition(values.metadata)
            : undefined;
          const builtIn = builtInChanged
            ? (parseMetadataDefinition(
                values.built_in_metadata,
                true,
              ) as MetadataField[])
            : undefined;
          const enabledChanged =
            form.isFieldTouched("enabled") &&
            values.enabled !== (snapshot.enabled ?? true);
          if (enabledChanged && snapshot.enabled === undefined)
            throw new Error(mt("configIncomplete"));
          const body = {
            ...(enabledChanged ? { enabled: values.enabled } : {}),
            ...(metadataChanged ? { metadata } : {}),
            ...(builtInChanged ? { built_in_metadata: builtIn } : {}),
          };
          if (!Object.keys(body).length) return;
          const latest = await knowledgeRequest<MetadataConfig>(
            "get",
            id,
            "metadata/config",
          );
          if (
            (metadataChanged &&
              !metadataReadbackMatches(
                latest.metadata ?? latest.fields,
                originalMetadata ?? [],
              )) ||
            (builtInChanged &&
              !metadataReadbackMatches(
                latest.built_in_metadata,
                snapshot.built_in_metadata ?? [],
              ))
          )
            throw new Error(mt("configConflict"));
          await knowledgeRequest("put", id, "metadata/config", body);
          const persisted = await knowledgeRequest<MetadataConfig>(
            "get",
            id,
            "metadata/config",
          );
          const expectedEnabled = body.enabled ?? latest.enabled;
          const expectedMetadata =
            body.metadata ?? latest.metadata ?? latest.fields;
          const expectedBuiltIn =
            body.built_in_metadata ?? latest.built_in_metadata;
          if (
            (expectedEnabled !== undefined &&
              !containsKnowledgeFields(persisted.enabled, expectedEnabled)) ||
            (expectedMetadata !== undefined &&
              !metadataReadbackMatches(
                persisted.metadata ?? persisted.fields,
                expectedMetadata,
              )) ||
            (expectedBuiltIn !== undefined &&
              !metadataReadbackMatches(
                persisted.built_in_metadata,
                expectedBuiltIn,
              ))
          )
            throw new Error(t("knowledge.manage.readbackFailed"));
          setSnapshot(persisted);
          form.resetFields();
          form.setFieldsValue(initialValues(persisted));
          setSaved(true);
          onSaved();
        } catch (err) {
          setError(parseApiError(err));
        } finally {
          setSaving(false);
        }
      }}
    >
      {error && (
        <Alert
          type="error"
          showIcon
          title={error}
          style={{ marginBottom: 16 }}
        />
      )}
      <Form.Item
        name="enabled"
        valuePropName="checked"
        label={t("knowledge.manage.autoMetadata")}
      >
        <Switch />
      </Form.Item>
      <Form.Item
        name="metadata"
        label={t("knowledge.manage.metadataTemplate")}
        extra={t("knowledge.manage.datasetTemplateHint")}
        rules={[{ validator: validate("metadata") }]}
      >
        <MetadataTemplateEditor
          disabled={!canWrite || saving || Boolean(readError)}
        />
      </Form.Item>
      <Typography.Paragraph type="secondary">
        {mt("clearTemplate")}
      </Typography.Paragraph>
      <Collapse
        style={{ marginBottom: 16 }}
        items={[
          {
            key: "builtin",
            label: t("knowledge.manage.builtInMetadata"),
            forceRender: true,
            children: (
              <Form.Item
                name="built_in_metadata"
                rules={[{ validator: validate("built_in_metadata", true) }]}
                extra={t("knowledge.manage.builtInMetadataHint")}
              >
                <MetadataTemplateEditor
                  arrayOnly
                  disabled={!canWrite || saving || Boolean(readError)}
                />
              </Form.Item>
            ),
          },
        ]}
      />
      <Space wrap>
        {canWrite && (
          <PrimaryButton
            htmlType="submit"
            loading={saving}
            disabled={Boolean(readError)}
          >
            {t("knowledge.docs.saveBtn")}
          </PrimaryButton>
        )}
        <Popconfirm
          title={mt("reloadWarning")}
          disabled={saving}
          onConfirm={async () => {
            setSaving(true);
            try {
              const persisted = await knowledgeRequest<MetadataConfig>(
                "get",
                id,
                "metadata/config",
              );
              setSnapshot(persisted);
              form.resetFields();
              form.setFieldsValue(initialValues(persisted));
              setError("");
              setSaved(false);
              onSaved();
            } catch (err) {
              setError(parseApiError(err));
            } finally {
              setSaving(false);
            }
          }}
        >
          <Button disabled={saving}>
            {t("knowledge.manage.reloadMetadataConfig")}
          </Button>
        </Popconfirm>
      </Space>
      {saved && (
        <Alert
          style={{ marginTop: 12 }}
          type="success"
          showIcon
          title={t("knowledge.manage.metadataConfigSaved")}
        />
      )}
    </Form>
  );
}

function MetadataInventory({ id }: { id: string }) {
  const { t } = useTranslation();
  const query = useKnowledgeMetadataInventory(id);
  return (
    <Space orientation="vertical" style={{ width: "100%" }}>
      <Typography.Text type="secondary">
        {t("knowledge.manage.metadataInventoryHint")}
      </Typography.Text>
      {query.error && (
        <Alert type="error" showIcon title={parseApiError(query.error)} />
      )}
      <Table
        size="small"
        rowKey="key"
        loading={query.isFetching}
        scroll={{ x: 480 }}
        dataSource={(query.data?.keys ?? []).flatMap((key) => {
          const values = query.data?.flattened[key];
          return values && typeof values === "object" && !Array.isArray(values)
            ? Object.entries(values).map(([value, ids]) => ({
                key: `${key}-${value}`,
                field: key,
                value,
                count: Array.isArray(ids) ? ids.length : 0,
              }))
            : [];
        })}
        columns={[
          { title: t("knowledge.manage.field"), dataIndex: "field" },
          {
            title: t("knowledge.manage.value"),
            dataIndex: "value",
            render: (value: string) => (
              <span style={{ overflowWrap: "anywhere" }}>{value}</span>
            ),
          },
          { title: t("knowledge.manage.count"), dataIndex: "count" },
        ]}
      />
      <Button onClick={() => void query.refetch()}>
        {t("knowledge.docs.refresh")}
      </Button>
    </Space>
  );
}

export default function KnowledgeMetadataPanel({ id }: { id: string }) {
  const { t } = useTranslation();
  const canWrite = useCanWrite();
  const config = useKnowledgeResource<MetadataConfig>(id, "metadata/config");
  const summary = useKnowledgeResource<{
    summary: Record<
      string,
      [unknown, number][] | { values: [unknown, number][]; type?: string }
    >;
  }>(id, "metadata/summary");
  const rows = Object.entries(summary.data?.summary ?? {}).flatMap(
    ([key, values]) =>
      (Array.isArray(values) ? values : values.values).map(
        ([value, count], index) => ({
          id: `${key}-${index}`,
          key,
          value: JSON.stringify(value),
          count,
        }),
      ),
  );
  return (
    <Space orientation="vertical" style={{ width: "100%" }} size="middle">
      <Button
        onClick={() => {
          void config.refetch();
          void summary.refetch();
        }}
      >
        {t("knowledge.docs.refresh")}
      </Button>
      {(config.error ?? summary.error) && (
        <Alert
          type="error"
          showIcon
          title={parseApiError(config.error ?? summary.error)}
        />
      )}
      {config.isLoading ? (
        <Spin />
      ) : (
        config.data && (
          <MetadataConfigurationForm
            key={id}
            id={id}
            config={config.data}
            readError={config.error}
            onSaved={() => {
              void config.refetch();
              void summary.refetch();
            }}
          />
        )
      )}
      <Collapse
        items={[
          {
            key: "inventory",
            label: t("knowledge.manage.metadataInventory"),
            children: <MetadataInventory id={id} />,
          },
        ]}
      />
      <Typography.Title level={5}>
        {t("knowledge.manage.metadataSummary")}
      </Typography.Title>
      <Table
        size="small"
        rowKey="id"
        dataSource={rows}
        loading={summary.isFetching}
        scroll={{ x: 480 }}
        columns={[
          { title: t("knowledge.manage.field"), dataIndex: "key" },
          { title: t("knowledge.manage.value"), dataIndex: "value" },
          { title: t("knowledge.manage.count"), dataIndex: "count" },
        ]}
      />
      {canWrite && (
        <Collapse
          items={[
            {
              key: "batch",
              label: t("knowledge.manage.batchMetadata"),
              children: (
                <MetadataBatchEditor
                  key={id}
                  id={id}
                  fields={metadataFields(
                    config.data?.metadata ?? config.data?.fields,
                  )}
                  onSaved={() => {
                    void summary.refetch();
                  }}
                />
              ),
            },
          ]}
        />
      )}
    </Space>
  );
}
