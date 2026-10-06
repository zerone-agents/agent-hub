import { useState } from "react";
import {
  Alert,
  Button,
  Empty,
  Input,
  InputNumber,
  Select,
  Space,
  Switch,
  Tabs,
  Typography,
} from "antd";
import { TrashIcon } from "@phosphor-icons/react";
import { useTranslation } from "react-i18next";
import { parseObjectJSON } from "@/api/knowledgeManagement";
import {
  metadataDefaultValue as emptyValue,
  type MetadataField,
} from "./metadata";
import { useMetadataTranslation } from "./useMetadataTranslation";

export function MetadataValueInput({
  value,
  type,
  onChange,
  disabled,
  label,
  options,
}: {
  value: unknown;
  type: string;
  onChange: (value: unknown) => void;
  disabled?: boolean;
  label: string;
  options?: unknown[];
}) {
  const { t } = useMetadataTranslation();
  const strings = options?.filter(
    (item): item is string => typeof item === "string",
  );
  if (type === "number")
    return (
      <InputNumber
        aria-label={label}
        disabled={disabled}
        value={typeof value === "number" ? value : null}
        onChange={(next) => {
          if (typeof next === "number") onChange(next);
        }}
        style={{ width: "100%" }}
      />
    );
  if (type === "boolean")
    return (
      <Switch
        aria-label={label}
        disabled={disabled}
        checked={value === true}
        onChange={onChange}
      />
    );
  if (type === "null") return <Typography.Text>{t("null")}</Typography.Text>;
  if (type === "list" && Array.isArray(value)) {
    const items = value as unknown[];
    if (items.some((item) => item !== null && typeof item === "object"))
      return (
        <Typography.Text>
          {JSON.stringify(items)} {t("unsupported")}
        </Typography.Text>
      );
    return (
      <Space orientation="vertical" style={{ width: "100%" }}>
        {items.map((item, index) => (
          <Space key={index} wrap style={{ width: "100%" }}>
            <Select
              aria-label={`${label}: ${index + 1} ${t("type")}`}
              disabled={disabled}
              style={{ width: 130 }}
              value={valueType(item)}
              options={["string", "number", "boolean", "null"].map((type) => ({
                value: type,
                label: t(type),
              }))}
              onChange={(type) => {
                onChange(
                  items.map((current, position) =>
                    position === index ? emptyValue(type) : current,
                  ),
                );
              }}
            />
            <div style={{ minWidth: 160, flex: "1 1 160px" }}>
              <MetadataValueInput
                label={`${label}: ${index + 1}`}
                disabled={disabled}
                type={valueType(item)}
                value={item}
                options={options}
                onChange={(next) => {
                  onChange(
                    items.map((current, position) =>
                      position === index ? next : current,
                    ),
                  );
                }}
              />
            </div>
            <Button
              type="text"
              disabled={disabled}
              danger
              icon={<TrashIcon size={16} />}
              aria-label={`${t("removeValue")}: ${index + 1}`}
              onClick={() => {
                onChange(items.filter((_, position) => position !== index));
              }}
            />
          </Space>
        ))}
        <Button
          disabled={disabled}
          onClick={() => {
            onChange([...items, ""]);
          }}
        >
          {t("addListValue")}
        </Button>
      </Space>
    );
  }
  if (type === "string" || type === "time")
    return strings?.length ? (
      <Select
        aria-label={label}
        disabled={disabled}
        value={typeof value === "string" ? value : ""}
        onChange={onChange}
        options={strings.map((item) => ({ value: item, label: item }))}
        style={{ width: "100%" }}
      />
    ) : (
      <Input
        aria-label={label}
        disabled={disabled}
        value={typeof value === "string" ? value : ""}
        onChange={(event) => {
          onChange(event.target.value);
        }}
      />
    );
  return (
    <Space orientation="vertical" style={{ width: "100%" }}>
      <Typography.Text style={{ overflowWrap: "anywhere" }}>
        {JSON.stringify(value)}
      </Typography.Text>
      <Typography.Text type="secondary">{t("unsupported")}</Typography.Text>
    </Space>
  );
}
const valueType = (value: unknown): string =>
  value === null
    ? "null"
    : Array.isArray(value)
      ? "list"
      : typeof value === "object"
        ? "json"
        : typeof value;

/** Form-compatible JSON-object editor. It preserves unknown values and emits
 * explicit removals/empty arrays; no onChange is emitted on mount or reload. */
export default function MetadataValuesEditor({
  value = "{}",
  onChange,
  fields = [],
  disabled,
  id,
}: {
  value?: string;
  onChange?: (value: string) => void;
  fields?: MetadataField[];
  disabled?: boolean;
  id?: string;
}) {
  const { t } = useMetadataTranslation();
  const { t: common } = useTranslation();
  const [newKey, setNewKey] = useState("");
  let values: Record<string, unknown> | undefined;
  try {
    values = parseObjectJSON(value);
  } catch {
    /* preserve incomplete JSON drafts */
  }
  const entries = Object.entries(values ?? {});
  const change = (key: string, next: unknown) =>
    onChange?.(JSON.stringify({ ...values, [key]: next }, null, 2));
  return (
    <Tabs
      defaultActiveKey={values ? "values" : "json"}
      items={[
        {
          key: "values",
          label: t("values"),
          disabled: !values,
          children: (
            <Space
              orientation="vertical"
              size="middle"
              style={{ width: "100%" }}
            >
              {!entries.length && (
                <Empty
                  image={Empty.PRESENTED_IMAGE_SIMPLE}
                  description={t("emptyValues")}
                />
              )}
              {entries.map(([key, item]) => {
                const field = fields.find(
                  (candidate) => (candidate.key ?? candidate.name) === key,
                );
                const type = field?.type ?? valueType(item);
                // An existing value that disagrees with its template must stay visible.
                const editableType =
                  type === "time" && typeof item === "string"
                    ? type
                    : type === valueType(item)
                      ? type
                      : valueType(item);
                return (
                  <div
                    key={key}
                    style={{
                      display: "flex",
                      flexWrap: "wrap",
                      gap: 8,
                      alignItems: "start",
                      width: "100%",
                    }}
                  >
                    <Space
                      orientation="vertical"
                      style={{ flex: "1 1 200px", minWidth: 0 }}
                    >
                      <Typography.Text
                        strong
                        style={{ overflowWrap: "anywhere" }}
                      >
                        {key}
                      </Typography.Text>
                      <MetadataValueInput
                        label={`${t("value")}: ${key}`}
                        value={item}
                        type={editableType}
                        onChange={(next) => change(key, next)}
                        disabled={disabled}
                        options={
                          field?.restrict_values === true &&
                          Array.isArray(field.enum)
                            ? field.enum
                            : undefined
                        }
                      />
                    </Space>
                    <Select
                      aria-label={`${t("type")}: ${key}`}
                      value={editableType}
                      disabled={disabled === true || editableType === "json"}
                      style={{ width: 150 }}
                      options={[
                        "string",
                        "number",
                        "list",
                        "time",
                        "boolean",
                        "null",
                        ...(editableType === "json" ? ["json"] : []),
                      ].map((type) => ({ value: type, label: t(type) }))}
                      onChange={(type) => change(key, emptyValue(type))}
                    />
                    <Button
                      type="text"
                      danger
                      disabled={disabled}
                      icon={<TrashIcon size={16} />}
                      aria-label={`${common("common.delete")}: ${key}`}
                      onClick={() =>
                        onChange?.(
                          JSON.stringify(
                            Object.fromEntries(
                              entries.filter(([name]) => name !== key),
                            ),
                            null,
                            2,
                          ),
                        )
                      }
                    />
                  </div>
                );
              })}
              <Space wrap style={{ width: "100%" }}>
                <Select
                  aria-label={t("addValue")}
                  mode="tags"
                  maxCount={1}
                  value={newKey ? [newKey] : []}
                  onChange={(keys) => {
                    setNewKey(keys.at(-1) ?? "");
                  }}
                  disabled={disabled}
                  style={{ width: 240, maxWidth: "100%" }}
                  options={fields
                    .filter(
                      (field) =>
                        !Object.hasOwn(
                          values ?? {},
                          String(field.key ?? field.name),
                        ),
                    )
                    .map((field) => ({
                      value: String(field.key ?? field.name),
                      label: String(field.key ?? field.name),
                    }))}
                />
                <Button
                  disabled={
                    disabled === true ||
                    !values ||
                    !newKey.trim() ||
                    Object.hasOwn(values, newKey.trim())
                  }
                  onClick={() => {
                    const field = fields.find(
                      (field) => (field.key ?? field.name) === newKey.trim(),
                    );
                    change(newKey.trim(), emptyValue(field?.type ?? "string"));
                    setNewKey("");
                  }}
                >
                  {t("addValue")}
                </Button>
              </Space>
            </Space>
          ),
        },
        {
          key: "json",
          label: t("advanced"),
          children: (
            <>
              <Input.TextArea
                id={id}
                aria-label={t("advanced")}
                rows={7}
                disabled={disabled}
                value={value}
                onChange={(event) => onChange?.(event.target.value)}
                spellCheck={false}
              />
              {!values && <Alert type="error" title={t("invalid")} />}
            </>
          ),
        },
      ]}
    />
  );
}
