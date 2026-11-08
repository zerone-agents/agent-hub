import {
  containsKnowledgeFields,
  type KnowledgeObject,
} from "@/api/knowledgeManagement";

// Connector source IDs must be supported by the ingestion worker.
// s3_compatible is a bucket_type for source=s3, not a separate source ID.
export const CONNECTOR_SOURCES = [
  "rss",
  "s3",
  "notion",
  "r2",
  "google_cloud_storage",
  "oci_storage",
  "slack",
  "confluence",
  "jira",
  "google_drive",
  "gmail",
  "discord",
  "webdav",
  "moodle",
  "dropbox",
  "box",
  "airtable",
  "asana",
  "github",
  "gitlab",
  "imap",
  "bitbucket",
  "zendesk",
  "seafile",
  "mysql",
  "postgresql",
  "dingtalk_ai_table",
  "rest_api",
  "sharepoint",
  "teams",
  "onedrive",
  "outlook",
  "azure_blob",
  "salesforce",
  "mssql",
];

// Deletion synchronization is available only for supported connector sources.
export const DELETION_SYNC_SOURCES = new Set([
  "moodle",
  "mysql",
  "postgresql",
  "mssql",
  "imap",
  "dingtalk_ai_table",
  "rss",
  "webdav",
  "airtable",
  "google_drive",
  "gmail",
  "bitbucket",
  "s3",
  "r2",
  "oci_storage",
  "google_cloud_storage",
  "confluence",
  "notion",
  "jira",
  "box",
  "github",
  "gitlab",
  "dropbox",
  "seafile",
  "asana",
  "zendesk",
]);

export function supportsDeletionSync(source: string, config?: KnowledgeObject) {
  return (
    DELETION_SYNC_SOURCES.has(source) &&
    (source !== "zendesk" ||
      (config?.zendesk_content_type ?? "articles") === "articles")
  );
}

export function isConnectorSchedule(value: unknown, minimum: number) {
  return (
    typeof value === "number" && Number.isSafeInteger(value) && value >= minimum
  );
}

export function writableConnectorConfig(config: KnowledgeObject) {
  const result = { ...config };
  // Worker-owned, per-KB state must never be replayed from a stale editor.
  delete result.imap_sync_scopes;
  delete result.rdbms_sync_scopes;
  return result;
}

/** Apply only editor changes, keeping concurrent credentials/extension updates. */
export function mergeConnectorConfig(
  previous: KnowledgeObject,
  edited: KnowledgeObject,
  current: KnowledgeObject,
): KnowledgeObject | undefined {
  const same = (a: unknown, b: unknown) =>
    containsKnowledgeFields(a, b) && containsKnowledgeFields(b, a);
  const result = { ...current };
  for (const key of new Set([
    ...Object.keys(previous),
    ...Object.keys(edited),
  ])) {
    if (same(previous[key], edited[key])) continue;
    if (same(current[key], edited[key])) continue;
    if (same(previous[key], current[key])) {
      if (key in edited) result[key] = edited[key];
      else Reflect.deleteProperty(result, key);
    } else if (
      [previous[key], edited[key], current[key]].every(
        (value) =>
          value !== null && typeof value === "object" && !Array.isArray(value),
      )
    ) {
      const merged = mergeConnectorConfig(
        previous[key] as KnowledgeObject,
        edited[key] as KnowledgeObject,
        current[key] as KnowledgeObject,
      );
      if (!merged) return undefined;
      result[key] = merged;
    } else return undefined;
  }
  return result;
}

export const CONNECTOR_REQUIRED_FIELDS: Partial<Record<string, string>> = {
  rss: "feed_url",
  s3: "bucket_name; credentials.authentication_method=access_key: aws_access_key_id + aws_secret_access_key; iam_role: aws_role_arn. S3 compatible: bucket_type=s3_compatible; credentials.endpoint_url + aws_access_key_id + aws_secret_access_key + addressing_style",
  r2: "bucket_name; credentials.r2_access_key_id + r2_secret_access_key + account_id",
  google_cloud_storage:
    "bucket_name; credentials.access_key_id + secret_access_key",
  oci_storage:
    "bucket_name; credentials.namespace + region + access_key_id + secret_access_key",
  notion: "root_page_id; credentials.notion_integration_token",
  slack: "credentials.slack_bot_token",
  confluence:
    "wiki_base; credentials.confluence_username + confluence_access_token",
  jira: "base_url; credentials.jira_user_email + jira_api_token (Cloud), or jira_username + jira_password (Server)",
  google_drive:
    "credentials.google_primary_admin + google_tokens (OAuth JSON string), or google_service_account_key; include_my_drives / include_shared_drives / include_files_shared_with_me",
  gmail:
    "credentials.google_primary_admin + google_tokens (OAuth JSON string), or google_service_account_key",
  discord: "credentials.discord_bot_token; server_ids / channels",
  webdav: "base_url; credentials.username + password",
  moodle: "moodle_url; credentials.moodle_token",
  dropbox: "credentials.dropbox_access_token",
  box: "folder_id; credentials.box_tokens (OAuth JSON string)",
  airtable: "base_id + table_name_or_id; credentials.airtable_access_token",
  asana: "asana_workspace_id; credentials.asana_api_token_secret",
  github: "repository_owner + repository_name; credentials.github_access_token",
  gitlab:
    "gitlab_url + project_name; credentials.gitlab_access_token; include_issues / include_mrs / include_code_files",
  imap: "imap_host; credentials.imap_username + imap_password",
  bitbucket:
    "workspace; credentials.bitbucket_account_email + bitbucket_api_token",
  zendesk: "credentials.zendesk_subdomain + zendesk_email + zendesk_token",
  seafile:
    "seafile_url; credentials.seafile_token, or username + password; library scope: repo_id + credentials.repo_token",
  mysql:
    "host + database; credentials.username + password; query / id_column / timestamp_column",
  postgresql:
    "host + database; credentials.username + password; query / id_column / timestamp_column",
  dingtalk_ai_table: "table_id + operator_id; credentials.access_token",
  rest_api:
    "url + content_fields; auth_type=none / api_key_header / bearer / basic; credentials depend on auth_type",
  sharepoint: "credentials.site_url + tenant_id + client_id + client_secret",
  teams: "credentials.tenant_id + client_id + client_secret",
  onedrive:
    "drive_ids (string array, explicit); credentials.tenant_id + client_id + client_secret",
  outlook:
    "user_ids (string array, explicit); credentials.tenant_id + client_id + client_secret",
  azure_blob:
    "auth_mode=account_key: credentials.account_name + account_key + container_name; connection_string: connection_string + container_name; sas_token: container_url + sas_token",
  salesforce: "credentials.instance_url + client_id + client_secret",
  mssql:
    "host + database + query (single SELECT); credentials.username + password",
};

const microsoftCredentials = {
  tenant_id: "<tenant-id>",
  client_id: "<client-id>",
  client_secret: "<client-secret>",
};
export const CONNECTOR_EXAMPLES: Partial<Record<string, KnowledgeObject>> = {
  rest_api: {
    url: "https://api.example.com/items",
    content_fields: ["title", "body"],
    items_path: "items[*]",
    id_field: "id",
    auth_type: "bearer",
    credentials: { token: "<token>" },
    pagination_type: "none",
  },
  sharepoint: {
    credentials: {
      ...microsoftCredentials,
      site_url: "https://example.sharepoint.com/sites/knowledge",
    },
  },
  teams: { credentials: microsoftCredentials, batch_size: 32 },
  onedrive: {
    credentials: microsoftCredentials,
    drive_ids: ["<drive-id>"],
    folder_path: "/Documents",
    batch_size: 32,
  },
  outlook: {
    credentials: microsoftCredentials,
    user_ids: ["support@example.com"],
    folder: "inbox",
    batch_size: 32,
  },
  azure_blob: {
    auth_mode: "account_key",
    credentials: {
      account_name: "<account-name>",
      account_key: "<account-key>",
      container_name: "documents",
    },
    prefix: "reports/",
    batch_size: 2,
  },
  salesforce: {
    credentials: {
      instance_url: "https://example.my.salesforce.com",
      client_id: "<client-id>",
      client_secret: "<client-secret>",
    },
    objects: ["Account", "Contact"],
    api_version: "v59.0",
    batch_size: 100,
  },
  mssql: {
    host: "database.example.com",
    port: 1433,
    database: "knowledge",
    query: "SELECT id, body, updated_at FROM dbo.records",
    content_columns: "body",
    id_column: "id",
    timestamp_column: "updated_at",
    credentials: { username: "<read-only-user>", password: "<password>" },
  },
};

const object = (value: unknown): KnowledgeObject =>
  value !== null && typeof value === "object" && !Array.isArray(value)
    ? (value as KnowledgeObject)
    : {};

const hasControlCharacter = (value: string) => {
  for (let index = 0; index < value.length; index++) {
    const code = value.charCodeAt(index);
    if (code < 32 || code === 127) return true;
  }
  return false;
};

/** Check required configuration shapes; the knowledge service performs full validation. */
export function connectorConfigIssue(
  source: string,
  config: KnowledgeObject,
): string | undefined {
  const credentials = object(config.credentials);
  const requireText = (
    values: KnowledgeObject,
    keys: string[],
    prefix = "",
  ) => {
    const missing = keys
      .filter(
        (key) =>
          typeof values[key] !== "string" || values[key].trim().length === 0,
      )
      .map((key) => prefix + key);
    return missing.length ? missing.join(", ") : undefined;
  };
  const credentialKeys = [
    "sharepoint",
    "teams",
    "onedrive",
    "outlook",
  ].includes(source)
    ? [
        "tenant_id",
        "client_id",
        "client_secret",
        ...(source === "sharepoint" ? ["site_url"] : []),
      ]
    : source === "salesforce"
      ? ["instance_url", "client_id", "client_secret"]
      : source === "mssql"
        ? ["username", "password"]
        : [];
  const missing = requireText(credentials, credentialKeys, "credentials.");
  if (missing) return missing;
  if (["onedrive", "outlook"].includes(source)) {
    const key = source === "onedrive" ? "drive_ids" : "user_ids";
    const ids = config[key];
    if (
      !Array.isArray(ids) ||
      ids.some(
        (id) =>
          typeof id !== "string" ||
          id.length === 0 ||
          id !== id.trim() ||
          id === "." ||
          id === ".." ||
          hasControlCharacter(id),
      ) ||
      new Set(ids).size !== ids.length
    )
      return key;
  }
  if (["teams", "onedrive", "outlook"].includes(source)) {
    for (const key of ["batch_size", "sync_batch_size"]) {
      if (key in config && !isConnectorSchedule(config[key], 1)) return key;
    }
  }
  if (
    source === "outlook" &&
    "folder" in config &&
    (typeof config.folder !== "string" ||
      config.folder.length === 0 ||
      config.folder !== config.folder.trim() ||
      [".", ".."].includes(config.folder) ||
      hasControlCharacter(config.folder))
  )
    return "folder";
  if (
    source === "onedrive" &&
    "folder_path" in config &&
    (typeof config.folder_path !== "string" ||
      config.folder_path
        .split("/")
        .some(
          (part) => [".", ".."].includes(part) || hasControlCharacter(part),
        ))
  )
    return "folder_path";
  if (source === "mssql")
    return requireText(config, ["host", "database", "query"]);
  if (source === "azure_blob") {
    const keys = {
      account_key: ["account_name", "account_key", "container_name"],
      connection_string: ["connection_string", "container_name"],
      sas_token: ["container_url", "sas_token"],
    }[String(config.auth_mode)];
    if (!keys) return "auth_mode";
    return requireText(credentials, keys, "credentials.");
  }
  if (source === "rest_api") {
    if (requireText(config, ["url"])) return "url";
    const fields =
      typeof config.content_fields === "string"
        ? config.content_fields.split(",")
        : config.content_fields;
    if (
      !Array.isArray(fields) ||
      !fields.length ||
      fields.some((field) => typeof field !== "string" || !field.trim())
    )
      return "content_fields";
    const auth = { ...object(config.auth_config), ...credentials };
    if (config.auth_type === "bearer")
      return requireText(auth, ["token"], "credentials.");
    if (config.auth_type === "basic")
      return (
        requireText(auth, ["username"], "credentials.") ??
        (auth.password == null ? "credentials.password" : undefined)
      );
    if (config.auth_type === "api_key_header")
      return (
        requireText(
          object(config.auth_config),
          ["header_name"],
          "auth_config.",
        ) ??
        (!auth.api_key && !auth.api_key_value
          ? "credentials.api_key"
          : undefined)
      );
  }
}
