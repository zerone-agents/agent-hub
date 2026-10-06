import { useState } from "react";
import {
  Alert,
  Button,
  Checkbox,
  Form,
  Input,
  Modal,
  Select,
  Space,
  Table,
  Typography,
} from "antd";
import { useQuery } from "@tanstack/react-query";
import { useKnowledgeMetadataInventory } from "@/queries/useKnowledgeManagement";
import { useTranslation } from "react-i18next";
import { knowledgeApi, type KnowledgeDocument } from "@/api/knowledge";
import {
  knowledgeRequest,
  type KnowledgeObject,
} from "@/api/knowledgeManagement";
import { parseApiError } from "@/api/client";
import PrimaryButton, {
  usePrimaryButtonStyle,
} from "@/components/PrimaryButton";
import { useCanWrite } from "@/hooks/useCanWrite";
import {
  documentMetadata,
  metadataBatchOperationsBody,
  metadataDefaultValue,
  metadataValuesMatch,
  previewMetadataOperations,
  validateMetadataValue,
  type MetadataField,
  type MetadataOperation,
} from "./metadata";
import { MetadataValueInput } from "./MetadataValuesEditor";
import {
  BASELINE_METADATA_OPERATORS,
  BASELINE_LIST_EXCLUSIONS,
  baselineMetadataConditionAllowed,
  baselineMetadataFieldAllowed,
  baselineMetadataDocumentAllowed,
  baselineMetadataEquivalent,
} from "./metadataBaseline";
import { useMetadataTranslation } from "./useMetadataTranslation";

interface Preview {
  document: KnowledgeDocument;
  before: KnowledgeObject;
  after: KnowledgeObject;
}
export default function MetadataBatchEditor({
  id,
  fields,
  inventory,
  onSaved,
}: {
  id: string;
  fields: MetadataField[];
  inventory?: { keys: string[]; flattened: KnowledgeObject };
  onSaved: () => void;
}) {
  const { t } = useMetadataTranslation();
  const { t: common } = useTranslation();
  const canWrite = useCanWrite();
  const style = usePrimaryButtonStyle();
  const [page, setPage] = useState(1);
  const [search, setSearch] = useState("");
  const [keywords, setKeywords] = useState("");
  const [reload, setReload] = useState(0);
  const [selected, setSelected] = useState<Record<string, string>>({});
  const [operation, setOperation] = useState<MetadataOperation>({
    key: "",
    mode: "set",
    value: "",
  });
  const [type, setType] = useState("string");
  const [queued, setQueued] = useState<MetadataOperation[]>([]);
  const [frozenOperations, setFrozenOperations] = useState<MetadataOperation[]>(
    [],
  );
  const operations = [...queued, ...(operation.key.trim() ? [operation] : [])];
  const [filterEnabled, setFilterEnabled] = useState(false);
  const [filterKey, setFilterKey] = useState("");
  const [filterValue, setFilterValue] = useState<string>();
  const [filterOp, setFilterOp] = useState("=");
  const [filterValues, setFilterValues] = useState<string[]>([]);
  const [logic, setLogic] = useState("and");
  const [conditions, setConditions] = useState<
    { name: string; comparison_operator: string; value: unknown }[]
  >([]);
  const draftCondition =
    filterKey &&
    (["empty", "not empty"].includes(filterOp) ||
      filterValue !== undefined ||
      (["in", "not in"].includes(filterOp) && filterValues.length))
      ? {
          name: filterKey,
          comparison_operator: filterOp,
          value: ["in", "not in"].includes(filterOp)
            ? filterValues
            : ["empty", "not empty"].includes(filterOp)
              ? ""
              : filterValue,
        }
      : undefined;
  const [preview, setPreview] = useState<Preview[]>();
  const [frozenCondition, setFrozenCondition] = useState<KnowledgeObject>();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [result, setResult] = useState<{
    verified: string[];
    failed: string[];
    matched?: number;
    updated?: number;
  }>();
  const list = useQuery({
    queryKey: ["knowledge", "metadata-documents", id, page, keywords, reload],
    queryFn: () =>
      knowledgeApi.documents.list(id, { page, page_size: 50, keywords }),
  });
  const { data: inventoryData } = useKnowledgeMetadataInventory(id);
  inventory ??= inventoryData;
  const documents = list.data?.documents ?? [];
  const total = list.data?.total ?? 0;
  const loading = list.isFetching;
  const loadError = list.error ? parseApiError(list.error) : "";
  const keys = [
    ...new Set([
      ...fields.map((field) => field.key ?? field.name ?? "").filter(Boolean),
      ...(inventory?.keys ?? []),
    ]),
  ];
  const field = fields.find(
    (field) => (field.key ?? field.name) === operation.key,
  );
  const changed =
    preview?.filter((row) => !metadataValuesMatch(row.before, row.after)) ?? [];
  const verify = async (
    rows: Preview[],
    matched?: number,
    updated?: number,
  ) => {
    const verified: string[] = [];
    const failed: string[] = [];
    for (const row of rows) {
      try {
        const doc = await knowledgeApi.documents.get(id, row.document.id);
        if (!metadataValuesMatch(documentMetadata(doc.meta_fields), row.after))
          throw new Error();
        verified.push(doc.id);
      } catch {
        failed.push(row.document.id);
      }
    }
    setResult({ verified, failed, matched, updated });
    onSaved();
  };
  const review = async () => {
    if (!canWrite || !Object.keys(selected).length) return;
    setBusy(true);
    setError("");
    setResult(undefined);
    setLastPreview([]);
    try {
      metadataBatchOperationsBody(Object.keys(selected), operations);
      for (const item of operations)
        if (item.mode === "set" || item.mode === "replace")
          validateMetadataValue(
            item.value,
            fields.find((field) => (field.key ?? field.name) === item.key),
          );
      if (
        type === "number" &&
        (typeof operation.value !== "number" ||
          !Number.isFinite(operation.value)) &&
        (operation.mode === "set" || operation.mode === "replace")
      )
        throw new Error(t("invalid"));
      const condition = filterEnabled
        ? {
            logic,
            conditions: [
              ...conditions,
              ...(draftCondition ? [draftCondition] : []),
            ],
          }
        : undefined;
      if (
        filterEnabled &&
        (!condition?.conditions.length || (filterKey && !draftCondition))
      )
        throw new Error(t("invalid"));
      if (
        condition?.conditions.some(
          (item) =>
            !baselineMetadataConditionAllowed(
              item,
              fields,
              inventory?.flattened,
            ),
        )
      )
        throw new Error(t("baselineFilterLimit"));
      // Ask the server to intersect the explicitly selected set with the condition.
      const ids = Object.keys(selected);
      const matches: KnowledgeDocument[] = [];
      const seen = new Set<string>();
      let expectedTotal: number | undefined;
      for (let cursor = 1; ; cursor++) {
        const data = await knowledgeApi.documents.list(id, {
          ids,
          page: cursor,
          page_size: 100,
          ...(condition
            ? { metadata_condition: JSON.stringify(condition) }
            : {}),
        });
        expectedTotal ??= data.total;
        if (data.total !== expectedTotal)
          throw new Error(t("changedSincePreview"));
        for (const doc of data.documents) {
          if (!Object.hasOwn(selected, doc.id) || seen.has(doc.id))
            throw new Error(t("changedSincePreview"));
          seen.add(doc.id);
        }
        matches.push(...data.documents);
        if (matches.length === expectedTotal) break;
        if (!data.documents.length || matches.length > expectedTotal)
          throw new Error(t("changedSincePreview"));
      }
      // Missing selected IDs are not silently dropped when no narrowing condition exists.
      if (!condition && matches.length !== ids.length)
        throw new Error(t("changedSincePreview"));
      if (!matches.length) throw new Error(t("noMatches"));
      const rows: Preview[] = [];
      for (const match of matches) {
        const doc = await knowledgeApi.documents.get(id, match.id);
        const before = documentMetadata(doc.meta_fields);
        if (
          condition &&
          !baselineMetadataDocumentAllowed(
            before,
            condition.conditions,
            condition.logic,
          )
        )
          throw new Error(t("baselineFilterLimit"));
        const after = previewMetadataOperations(before, operations);
        if (
          !metadataValuesMatch(before, after) &&
          baselineMetadataEquivalent(before, after)
        )
          throw new Error(t("baselineTypeOnly"));
        rows.push({ document: doc, before, after });
      }
      if (rows.every((row) => metadataValuesMatch(row.before, row.after)))
        throw new Error(t("noChanges"));
      setFrozenOperations(operations);
      setFrozenCondition(condition);
      setLastPreview(rows);
      setPreview(rows);
    } catch (error) {
      setError(parseApiError(error));
    } finally {
      setBusy(false);
    }
  };
  const apply = async () => {
    if (!canWrite || !preview?.length) return;
    setBusy(true);
    setError("");
    let attempted = false;
    try {
      // Refuse to overwrite a value changed by another editor after review.
      for (const row of preview) {
        const doc = await knowledgeApi.documents.get(id, row.document.id);
        if (!metadataValuesMatch(documentMetadata(doc.meta_fields), row.before))
          throw new Error(t("changedSincePreview"));
      }
      attempted = true;
      const response = await knowledgeRequest<{
        matched_docs: number;
        updated: number;
      }>(
        "patch",
        id,
        "documents/metadatas",
        metadataBatchOperationsBody(
          preview.map((row) => row.document.id),
          frozenOperations,
          frozenCondition,
        ),
      );
      await verify(preview, response.matched_docs, response.updated);
      setPreview(undefined);
    } catch (error) {
      setError(parseApiError(error));
      // A network error can arrive after the write. Keep expected values for read-only recovery.
      if (attempted) {
        setResult({
          verified: [],
          failed: preview.map((row) => row.document.id),
        });
        setPreview(undefined);
        onSaved();
      }
    } finally {
      setBusy(false);
    }
  };
  const [lastPreview, setLastPreview] = useState<Preview[]>([]);
  return (
    <Space orientation="vertical" size="middle" style={{ width: "100%" }}>
      <Alert type="info" showIcon title={t("baselineFilterLimit")} />
      <Typography.Title level={5}>{t("scope")}</Typography.Title>
      <Typography.Text type="secondary">{t("scopeHint")}</Typography.Text>
      <Input.Search
        aria-label={t("search")}
        placeholder={t("search")}
        value={search}
        disabled={busy}
        onChange={(event) => {
          setSearch(event.target.value);
        }}
        onSearch={(value) => {
          setPage(1);
          setKeywords(value);
        }}
      />
      {loadError && (
        <Alert
          type="error"
          showIcon
          title={t("notLoaded")}
          description={loadError}
          action={
            <Button
              onClick={() => {
                setReload((value) => value + 1);
              }}
            >
              {t("retry")}
            </Button>
          }
        />
      )}
      <Table
        size="small"
        rowKey="id"
        loading={loading}
        scroll={{ x: 360 }}
        dataSource={documents}
        columns={[
          {
            title: common("knowledge.docs.docCol"),
            dataIndex: "name",
            render: (name: string) => (
              <span style={{ overflowWrap: "anywhere" }}>{name}</span>
            ),
          },
        ]}
        rowSelection={{
          selectedRowKeys: Object.keys(selected),
          preserveSelectedRowKeys: true,
          getCheckboxProps: () => ({
            disabled: busy || !canWrite || Boolean(loadError),
          }),
          onChange: (keys, rows) => {
            const names = { ...selected };
            rows.forEach((doc) => {
              names[doc.id] = doc.name;
            });
            setSelected(
              Object.fromEntries(
                keys.map((key) => [
                  String(key),
                  names[String(key)] ?? String(key),
                ]),
              ),
            );
          },
        }}
        pagination={{
          current: page,
          total,
          pageSize: 50,
          showSizeChanger: false,
          onChange: setPage,
          disabled: busy,
        }}
      />
      <Space wrap>
        <Typography.Text>
          {t("selected", { count: Object.keys(selected).length })}
        </Typography.Text>
        <Button
          disabled={busy}
          onClick={() => {
            setSelected({});
          }}
        >
          {t("clearSelection")}
        </Button>
      </Space>
      {!!queued.length && (
        <Table
          size="small"
          rowKey="key"
          pagination={false}
          scroll={{ x: 400 }}
          dataSource={queued}
          columns={[
            { title: t("field"), dataIndex: "key" },
            {
              title: t("operation"),
              render: (_, row) => (
                <span>
                  {t(row.mode)} {JSON.stringify(row.value ?? row.match)}
                </span>
              ),
            },
            {
              title: common("knowledge.docs.actions"),
              render: (_, row) => (
                <Button
                  disabled={busy}
                  type="link"
                  danger
                  onClick={() => {
                    setQueued(queued.filter((item) => item.key !== row.key));
                  }}
                >
                  {common("common.delete")}
                </Button>
              ),
            },
          ]}
        />
      )}
      <Form layout="vertical" disabled={busy || !canWrite}>
        <Form.Item label={t("operation")}>
          <Select
            aria-label={t("operation")}
            value={operation.mode}
            options={["set", "replace", "deleteField", "deleteValue"].map(
              (mode) => ({ value: mode, label: t(mode) }),
            )}
            onChange={(mode) => {
              setOperation({
                ...operation,
                mode,
                ...(mode === "deleteValue"
                  ? { match: operation.match ?? "" }
                  : {}),
              });
            }}
          />
        </Form.Item>
        <Form.Item label={t("field")}>
          <Select
            aria-label={t("field")}
            mode="tags"
            maxCount={1}
            value={operation.key ? [operation.key] : []}
            options={keys.map((key) => ({ value: key, label: key }))}
            onChange={(values) => {
              const key = values.at(-1) ?? "";
              const type =
                fields.find((field) => (field.key ?? field.name) === key)
                  ?.type ?? "string";
              setType(type);
              setOperation({
                ...operation,
                key,
                value: metadataDefaultValue(type),
              });
            }}
          />
        </Form.Item>
        {(operation.mode === "replace" || operation.mode === "deleteValue") && (
          <Form.Item label={t("match")}>
            <Input
              aria-label={t("match")}
              value={operation.match ?? ""}
              onChange={(event) => {
                setOperation({ ...operation, match: event.target.value });
              }}
            />
          </Form.Item>
        )}
        {(operation.mode === "set" || operation.mode === "replace") && (
          <>
            <Form.Item label={t("type")}>
              <Select
                aria-label={t("type")}
                value={type}
                options={[
                  "string",
                  "number",
                  "list",
                  "time",
                  "boolean",
                  "null",
                ].map((type) => ({ value: type, label: t(type) }))}
                onChange={(type) => {
                  setType(type);
                  setOperation({
                    ...operation,
                    value: metadataDefaultValue(type),
                  });
                }}
              />
            </Form.Item>
            <Form.Item label={t("value")} extra={t("operationHint")}>
              <MetadataValueInput
                label={t("value")}
                type={type}
                value={operation.value}
                disabled={busy || !canWrite}
                options={
                  field?.restrict_values === true && Array.isArray(field.enum)
                    ? field.enum
                    : undefined
                }
                onChange={(value) => {
                  setOperation({ ...operation, value });
                }}
              />
            </Form.Item>
          </>
        )}
        <Form.Item extra={t("filterHint")}>
          <Checkbox
            checked={filterEnabled}
            onChange={(event) => {
              setFilterEnabled(event.target.checked);
            }}
          >
            {t("filter")}
          </Checkbox>
        </Form.Item>
        {filterEnabled && (
          <Space orientation="vertical" style={{ width: "100%" }}>
            <Select
              aria-label={t("logic")}
              value={logic}
              options={[
                { value: "and", label: t("and") },
                { value: "or", label: t("or") },
              ]}
              onChange={setLogic}
              style={{ width: "100%" }}
            />
            {!!conditions.length && (
              <Table
                size="small"
                pagination={false}
                dataSource={conditions.map((condition, index) => ({
                  ...condition,
                  index,
                }))}
                rowKey="index"
                columns={[
                  { title: t("field"), dataIndex: "name" },
                  { title: t("operator"), dataIndex: "comparison_operator" },
                  {
                    title: t("filterValue"),
                    render: (_, row) => JSON.stringify(row.value),
                  },
                  {
                    title: common("knowledge.docs.actions"),
                    render: (_, row) => (
                      <Button
                        disabled={busy}
                        type="text"
                        danger
                        onClick={() => {
                          setConditions(
                            conditions.filter(
                              (_, index) => index !== row.index,
                            ),
                          );
                        }}
                      >
                        {common("common.delete")}
                      </Button>
                    ),
                  },
                ]}
              />
            )}
            <Select
              aria-label={t("filter")}
              placeholder={t("field")}
              value={filterKey || undefined}
              style={{ width: "100%" }}
              options={(inventory?.keys ?? []).map((key) => ({
                value: key,
                label: key,
                disabled: !baselineMetadataFieldAllowed(
                  fields.find((field) => (field.key ?? field.name) === key),
                  inventory?.flattened[key],
                  filterOp,
                ),
              }))}
              onChange={(key) => {
                setFilterKey(key);
                setFilterValue(undefined);
                setFilterValues([]);
              }}
            />
            <Select
              aria-label={t("operator")}
              value={filterOp}
              style={{ width: "100%" }}
              options={BASELINE_METADATA_OPERATORS.map((op) => ({
                value: op,
                label: t(`op_${op}`),
                disabled:
                  fields.find(
                    (field) => (field.key ?? field.name) === filterKey,
                  )?.type === "list" && BASELINE_LIST_EXCLUSIONS.has(op),
              }))}
              onChange={(op) => {
                setFilterOp(op);
                setFilterValue(undefined);
                setFilterValues([]);
              }}
            />
            {!["empty", "not empty"].includes(filterOp) && (
              <Select
                aria-label={t("filterValue")}
                placeholder={t("filterValue")}
                mode="tags"
                maxCount={["in", "not in"].includes(filterOp) ? undefined : 1}
                value={
                  ["in", "not in"].includes(filterOp)
                    ? filterValues
                    : filterValue !== undefined
                      ? [filterValue]
                      : []
                }
                style={{ width: "100%" }}
                options={Object.keys(
                  inventory?.flattened[filterKey] &&
                    typeof inventory.flattened[filterKey] === "object"
                    ? (inventory.flattened[filterKey] as KnowledgeObject)
                    : {},
                ).map((value) => ({ value, label: value || t("emptyString") }))}
                onChange={(values) => {
                  if (["in", "not in"].includes(filterOp))
                    setFilterValues(values);
                  else setFilterValue(values.at(-1));
                }}
              />
            )}
            <Button
              disabled={busy || !draftCondition}
              onClick={() => {
                if (draftCondition) {
                  setConditions([...conditions, draftCondition]);
                  setFilterKey("");
                  setFilterValue(undefined);
                  setFilterValues([]);
                }
              }}
            >
              {t("addCondition")}
            </Button>
          </Space>
        )}
      </Form>
      <Button
        disabled={busy || !canWrite || !operation.key.trim()}
        onClick={() => {
          try {
            metadataBatchOperationsBody(Object.keys(selected), operations);
            if (operation.mode === "set" || operation.mode === "replace")
              validateMetadataValue(operation.value, field);
            setQueued([...queued, operation]);
            setOperation({ key: "", mode: "set", value: "" });
            setType("string");
            setError("");
          } catch {
            setError(t("invalid"));
          }
        }}
      >
        {t("queueOperation")}
      </Button>
      {error && <Alert type="error" showIcon title={error} />}
      {result && (
        <>
          {result.matched !== undefined && (
            <Alert
              type="info"
              title={t("partial", {
                matched: result.matched,
                updated: result.updated,
              })}
            />
          )}
          {!!result.verified.length && (
            <Alert
              type="success"
              showIcon
              title={t("verified", { count: result.verified.length })}
            />
          )}
          {!!result.failed.length && (
            <Alert
              type="warning"
              showIcon
              title={t("unverified", { count: result.failed.length })}
              description={
                <Space orientation="vertical">
                  <span>
                    {result.failed.map((id) => selected[id] ?? id).join(", ")}
                  </span>
                  <Button
                    disabled={busy}
                    onClick={async () => {
                      setBusy(true);
                      try {
                        await verify(
                          lastPreview.length ? lastPreview : (preview ?? []),
                        );
                      } finally {
                        setBusy(false);
                      }
                    }}
                  >
                    {t("retryReadback")}
                  </Button>
                </Space>
              }
            />
          )}
        </>
      )}
      <PrimaryButton
        disabled={
          !canWrite ||
          !Object.keys(selected).length ||
          !operations.length ||
          Boolean(loadError)
        }
        loading={busy}
        onClick={review}
      >
        {t("preview")}
      </PrimaryButton>
      <Modal
        open={Boolean(preview)}
        width={800}
        title={t("previewTitle")}
        onCancel={() => {
          if (!busy) setPreview(undefined);
        }}
        onOk={apply}
        confirmLoading={busy}
        closable={!busy}
        mask={{ closable: !busy }}
        cancelButtonProps={{ disabled: busy }}
        okText={t("confirm", { count: preview?.length ?? 0 })}
        okButtonProps={{
          className: style.root,
          disabled: !canWrite || !changed.length,
        }}
      >
        <Alert
          type={
            frozenOperations.some((item) => item.mode.startsWith("delete"))
              ? "warning"
              : "info"
          }
          showIcon
          title={t("previewCount", {
            matched: preview?.length ?? 0,
            changed: changed.length,
            excluded: Object.keys(selected).length - (preview?.length ?? 0),
          })}
          description={
            frozenOperations.some((item) => item.mode.startsWith("delete"))
              ? t("deleteWarning")
              : t("operationHint")
          }
          style={{ marginBottom: 16 }}
        />
        {error && (
          <Alert type="error" title={error} style={{ marginBottom: 16 }} />
        )}
        <Typography.Paragraph>
          {frozenOperations
            .map((item) => `${item.key}: ${t(item.mode)}`)
            .join("; ")}
        </Typography.Paragraph>
        <Table
          rowKey={(row) => row.document.id}
          size="small"
          dataSource={preview}
          pagination={false}
          scroll={{ x: 520, y: 360 }}
          columns={[
            {
              title: common("knowledge.docs.docCol"),
              render: (_, row) => row.document.name,
            },
            {
              title: t("before"),
              render: (_, row) => (
                <span style={{ overflowWrap: "anywhere" }}>
                  {JSON.stringify(row.before)}
                </span>
              ),
            },
            {
              title: t("after"),
              render: (_, row) => (
                <span style={{ overflowWrap: "anywhere" }}>
                  {JSON.stringify(row.after)}
                </span>
              ),
            },
          ]}
        />
      </Modal>
    </Space>
  );
}
