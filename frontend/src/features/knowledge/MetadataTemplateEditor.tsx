import {
  Button,
  Empty,
  Input,
  Select,
  Space,
  Switch,
  Tabs,
  Typography,
} from "antd";
import { PlusIcon, TrashIcon } from "@phosphor-icons/react";
import { useTranslation } from "react-i18next";
import {
  metadataFields,
  metadataNumberOptions,
  type MetadataField,
} from "./metadata";
import { useMetadataTranslation } from "./useMetadataTranslation";
import { Alert } from "antd";

export default function MetadataTemplateEditor({
  value = "[]",
  onChange,
  disabled,
  id,
  arrayOnly = false,
}: {
  value?: string;
  onChange?: (value: string) => void;
  disabled?: boolean;
  id?: string;
  arrayOnly?: boolean;
}) {
  const { t } = useTranslation();
  const { t: mt } = useMetadataTranslation();
  let fields: MetadataField[] | undefined;
  let schema: Record<string, unknown> | undefined;
  try {
    const parsed: unknown = JSON.parse(value);
    if (
      Array.isArray(parsed) &&
      parsed.every(
        (item: unknown) =>
          item && typeof item === "object" && !Array.isArray(item),
      )
    )
      fields = parsed as MetadataField[];
    else if (
      !arrayOnly &&
      parsed &&
      typeof parsed === "object" &&
      !Array.isArray(parsed) &&
      (Object.keys(parsed).length === 0 || "properties" in parsed)
    ) {
      schema = parsed as Record<string, unknown>;
      fields = metadataFields(schema);
    }
  } catch {
    /* the JSON tab retains incomplete drafts */
  }
  const update = (index: number, patch: Record<string, unknown>) => {
    if (!fields) return;
    const rows = fields.map((field, rowIndex) =>
      rowIndex === index ? { ...field, ...patch } : field,
    );
    if (schema) {
      const properties = {
        ...(schema.properties as Record<string, unknown> | undefined),
      };
      const oldKey = fields[index].key ?? "";
      const key = typeof patch.key === "string" ? patch.key : oldKey;
      // Keep duplicate/blank name drafts in JSON rather than silently dropping another schema property.
      if (!key.trim() || (key !== oldKey && Object.hasOwn(properties, key)))
        return;
      const original = properties[oldKey] as Record<string, unknown>;
      const { key: _key, name: _name, ...changes } = patch;
      if (changes.type === "list") changes.type = "array";
      if (changes.type === "time") {
        changes.type = "string";
        changes.format = "date-time";
      }
      if (
        (original.type === "array" || changes.type === "array") &&
        changes.enum !== undefined
      ) {
        const items =
          original.items &&
          typeof original.items === "object" &&
          !Array.isArray(original.items)
            ? (original.items as Record<string, unknown>)
            : { type: "string" };
        let options = changes.enum;
        if (
          (items.type === "number" || items.type === "integer") &&
          Array.isArray(options)
        ) {
          try {
            options = metadataNumberOptions(options as unknown[]);
          } catch {
            /* Keep invalid numeric drafts for the form validator. */
          }
        }
        changes.items =
          original.items === false ? false : { ...items, enum: options };
        Reflect.deleteProperty(changes, "enum");
      }
      const nextProperties = Object.fromEntries(
        Object.entries(properties).map(([name, definition]) =>
          name === oldKey
            ? [key, { ...original, ...changes }]
            : [name, definition],
        ),
      );
      const required = Array.isArray(schema.required)
        ? schema.required.map((name: unknown) => (name === oldKey ? key : name))
        : undefined;
      onChange?.(
        JSON.stringify(
          {
            ...schema,
            properties: nextProperties,
            ...(required ? { required } : {}),
          },
          null,
          2,
        ),
      );
    } else onChange?.(JSON.stringify(rows, null, 2));
  };
  const remove = (index: number) => {
    if (!fields) return;
    if (!schema) {
      onChange?.(
        JSON.stringify(
          fields.filter((_, row) => row !== index),
          null,
          2,
        ),
      );
      return;
    }
    const key = fields[index].key;
    onChange?.(
      JSON.stringify(
        {
          ...schema,
          properties: Object.fromEntries(
            Object.entries(schema.properties as Record<string, unknown>).filter(
              ([name]) => name !== key,
            ),
          ),
          ...(Array.isArray(schema.required)
            ? {
                required: schema.required.filter(
                  (name: unknown) => name !== key,
                ),
              }
            : {}),
        },
        null,
        2,
      ),
    );
  };
  const add = () => {
    if (!schema) {
      onChange?.(
        JSON.stringify(
          [...(fields ?? []), { key: "", type: "string" }],
          null,
          2,
        ),
      );
      return;
    }
    const properties = {
      ...(schema.properties as Record<string, unknown> | undefined),
    };
    let key = "field";
    let index = 1;
    while (Object.hasOwn(properties, key)) key = `field${index++}`;
    onChange?.(
      JSON.stringify(
        { ...schema, properties: { ...properties, [key]: { type: "string" } } },
        null,
        2,
      ),
    );
  };
  const rows = fields;
  return (
    <Tabs
      defaultActiveKey={fields ? "fields" : "json"}
      items={[
        {
          key: "fields",
          label: t("knowledge.manage.fieldEditor"),
          disabled: !fields,
          children: (
            <Space
              orientation="vertical"
              size="middle"
              style={{ width: "100%" }}
            >
              {rows?.length === 0 && (
                <Alert type="info" showIcon title={mt("clearTemplate")} />
              )}
              {rows?.length === 0 && (
                <Empty
                  image={Empty.PRESENTED_IMAGE_SIMPLE}
                  description={t("knowledge.manage.noMetadataFields")}
                />
              )}
              {rows?.map((field, index) => (
                <div
                  key={index}
                  style={{
                    border: "1px solid var(--border)",
                    borderRadius: 8,
                    padding: 12,
                  }}
                >
                  <Space wrap style={{ width: "100%", marginBottom: 8 }}>
                    <Input
                      aria-label={t("knowledge.manage.metadataFieldKey", {
                        n: index + 1,
                      })}
                      placeholder={t("knowledge.manage.field")}
                      disabled={disabled}
                      value={field.key ?? field.name ?? ""}
                      style={{ width: 200, maxWidth: "100%" }}
                      onChange={(e) => {
                        update(index, {
                          key: e.target.value,
                          ...(field.name !== undefined
                            ? { name: e.target.value }
                            : {}),
                        });
                      }}
                    />
                    <Select
                      aria-label={t("knowledge.manage.metadataFieldType", {
                        n: index + 1,
                      })}
                      disabled={disabled}
                      value={field.type}
                      placeholder={t("knowledge.manage.inferredType")}
                      style={{ width: 130 }}
                      options={["string", "number", "list", "time"].map(
                        (type) => ({
                          value: type,
                          label: t(`knowledge.manage.metadataType_${type}`),
                        }),
                      )}
                      onChange={(type) => {
                        update(index, { type });
                      }}
                    />
                    <Button
                      disabled={disabled}
                      type="text"
                      danger
                      aria-label={t("knowledge.manage.removeMetadataField", {
                        n: index + 1,
                      })}
                      icon={<TrashIcon size={16} />}
                      onClick={() => {
                        remove(index);
                      }}
                    />
                  </Space>
                  <Input
                    aria-label={t("knowledge.manage.metadataFieldDescription", {
                      n: index + 1,
                    })}
                    placeholder={t("knowledge.manage.extractionDescription")}
                    disabled={disabled}
                    value={
                      typeof field.description === "string"
                        ? field.description
                        : typeof field.descriptions === "string"
                          ? field.descriptions
                          : ""
                    }
                    onChange={(e) => {
                      update(index, { description: e.target.value });
                    }}
                    style={{ marginBottom: 8 }}
                  />
                  <Select
                    mode="tags"
                    aria-label={t("knowledge.manage.metadataFieldEnum", {
                      n: index + 1,
                    })}
                    disabled={
                      disabled === true ||
                      (Boolean(schema) && field.items === false)
                    }
                    placeholder={t("knowledge.manage.enumHint")}
                    style={{ width: "100%" }}
                    value={
                      Array.isArray(field.enum) ? field.enum.map(String) : []
                    }
                    onChange={(values) => {
                      update(index, { enum: values });
                    }}
                  />
                  {schema && field.items === false && (
                    <Typography.Text type="secondary">
                      {mt("noListItems")}
                    </Typography.Text>
                  )}
                  {!arrayOnly && !schema && (
                    <Space style={{ marginTop: 8 }}>
                      <Switch
                        size="small"
                        aria-label={t(
                          "knowledge.manage.restrictMetadataValues",
                          { n: index + 1 },
                        )}
                        disabled={disabled}
                        checked={field.restrict_values === true}
                        onChange={(checked) => {
                          update(index, { restrict_values: checked });
                        }}
                      />
                      <Typography.Text type="secondary">
                        {t("knowledge.manage.restrictMetadataHint")}
                      </Typography.Text>
                    </Space>
                  )}
                </div>
              ))}
              <Button
                disabled={(disabled ?? false) || !rows}
                icon={<PlusIcon size={16} />}
                onClick={add}
              >
                {t("knowledge.manage.addMetadataField")}
              </Button>
            </Space>
          ),
        },
        {
          key: "json",
          label: arrayOnly
            ? t("knowledge.manage.definitionJSON")
            : t("knowledge.manage.definitionSchema"),
          children: (
            <Input.TextArea
              id={id}
              value={value}
              onChange={(e) => onChange?.(e.target.value)}
              disabled={disabled}
              rows={7}
              spellCheck={false}
            />
          ),
        },
      ]}
    />
  );
}
