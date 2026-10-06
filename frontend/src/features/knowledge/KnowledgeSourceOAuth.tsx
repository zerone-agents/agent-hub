import { useEffect, useRef, useState } from "react";
import { Alert, Button, Collapse, Form, Input, Space } from "antd";
import { useTranslation } from "react-i18next";
import {
  knowledgeRequest,
  parseObjectJSON,
  type KnowledgeObject,
} from "@/api/knowledgeManagement";
import { parseApiError } from "@/api/client";
import PrimaryButton from "@/components/PrimaryButton";

interface OAuthValues {
  client_json: string;
  primary_email: string;
  client_id: string;
  client_secret: string;
  redirect_uri?: string;
}
interface OAuthFlow {
  flow_id: string;
  authorization_url: string;
  expires_in: number;
}
export function sourceOAuthCredentials(
  source: string,
  credentials: string,
  primaryEmail: string,
): KnowledgeObject {
  return source === "box"
    ? { box_tokens: credentials }
    : {
        google_tokens: credentials,
        google_primary_admin: primaryEmail,
        authentication_method: "oauth_interactive",
      };
}
export default function KnowledgeSourceOAuth({
  id,
  source,
  onAuthorized,
}: {
  id: string;
  source: string;
  onAuthorized: (credentials: KnowledgeObject) => void;
}) {
  const { t } = useTranslation();
  const active = useRef(true);
  useEffect(() => {
    active.current = true;
    return () => {
      active.current = false;
    };
  }, []);
  const [primaryEmail, setPrimaryEmail] = useState("");
  const [form] = Form.useForm<OAuthValues>();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [pending, setPending] = useState(false);
  const [applied, setApplied] = useState(false);
  const [flow, setFlow] = useState<OAuthFlow | null>(null);
  const [completed, setCompleted] = useState<KnowledgeObject | null>(null);
  const provider = source === "box" ? "box" : "google";
  const params =
    provider === "google"
      ? { source: source === "gmail" ? "gmail" : "google-drive" }
      : undefined;
  const start = async () => {
    try {
      const values = await form.validateFields();
      setBusy(true);
      setError("");
      setPending(false);
      setApplied(false);
      setCompleted(null);
      setFlow(null);
      let redirect_uri = values.redirect_uri?.trim();
      if (redirect_uri === "") redirect_uri = undefined;
      const body =
        provider === "google"
          ? { credentials: parseObjectJSON(values.client_json), redirect_uri }
          : {
              client_id: values.client_id.trim(),
              client_secret: values.client_secret,
              redirect_uri,
            };
      const result = await knowledgeRequest<OAuthFlow>(
        "post",
        id,
        `sources/oauth/${provider}/start`,
        body,
        params,
      );
      const url = new URL(result.authorization_url);
      if (
        !result.flow_id ||
        url.protocol !== "https:" ||
        ![
          "accounts.google.com",
          "account.box.com",
          "app.box.com",
          "box.com",
        ].includes(url.hostname)
      )
        throw new Error(t("knowledge.manage.oauthInvalidResponse"));
      if (!active.current) return;
      setPrimaryEmail(values.primary_email);
      setFlow(result);
    } catch (err) {
      setError(parseApiError(err));
    } finally {
      setBusy(false);
    }
  };
  const read = async () => {
    if (!flow) return;
    setBusy(true);
    setError("");
    try {
      let credentials = completed;
      if (!credentials) {
        await form.validateFields();
        const result = await knowledgeRequest<{
          pending?: boolean;
          credentials?: string;
        }>(
          "post",
          id,
          `sources/oauth/${provider}/result`,
          { flow_id: flow.flow_id },
          params,
        );
        if (result.pending) {
          setPending(true);
          return;
        }
        if (!result.credentials)
          throw new Error(t("knowledge.manage.oauthInvalidResponse"));
        credentials = sourceOAuthCredentials(
          source,
          result.credentials,
          primaryEmail,
        );
        // Results are consumed once upstream. Keep a local copy until the user
        // has applied it to a valid source configuration, never in storage.
        setCompleted(credentials);
      }
      if (!active.current) return;
      onAuthorized(credentials);
      setApplied(true);
      setPending(false);
    } catch (err) {
      setError(parseApiError(err));
    } finally {
      setBusy(false);
    }
  };
  return (
    <Collapse
      items={[
        {
          key: "oauth",
          label: t("knowledge.manage.oauthSetup"),
          children: (
            <>
              <Alert
                type="info"
                title={t("knowledge.manage.oauthHint")}
                style={{ marginBottom: 12 }}
              />
              {error && <Alert type="error" title={error} />}
              {pending && (
                <Alert type="info" title={t("knowledge.manage.oauthPending")} />
              )}
              {applied && (
                <Alert
                  type="success"
                  title={t("knowledge.manage.oauthApplied")}
                />
              )}
              <Form form={form} component={false} layout="vertical">
                {provider === "google" ? (
                  <>
                    <Form.Item
                      name="client_json"
                      label={t("knowledge.manage.oauthClientJSON")}
                      rules={[{ required: true }]}
                    >
                      <Input.TextArea rows={4} />
                    </Form.Item>
                    <Form.Item
                      name="primary_email"
                      label={t("knowledge.manage.primaryEmail")}
                      rules={[{ required: true }, { type: "email" }]}
                    >
                      <Input />
                    </Form.Item>
                  </>
                ) : (
                  <>
                    <Form.Item
                      name="client_id"
                      label="Client ID"
                      rules={[{ required: true, whitespace: true }]}
                    >
                      <Input />
                    </Form.Item>
                    <Form.Item
                      name="client_secret"
                      label="Client secret"
                      rules={[{ required: true }]}
                    >
                      <Input.Password autoComplete="off" />
                    </Form.Item>
                  </>
                )}
                <Form.Item
                  name="redirect_uri"
                  label={t("knowledge.manage.redirectURI")}
                  extra={t("knowledge.manage.redirectURIHint")}
                  rules={[{ type: "url" }]}
                >
                  <Input />
                </Form.Item>
                <Space wrap>
                  <Button loading={busy} onClick={start}>
                    {t("knowledge.manage.startOAuth")}
                  </Button>
                  {flow && (
                    <>
                      <a
                        href={flow.authorization_url}
                        target="_blank"
                        rel="noopener noreferrer"
                      >
                        {t("knowledge.manage.openOAuth")}
                      </a>
                      <PrimaryButton loading={busy} onClick={read}>
                        {t("knowledge.manage.readOAuth")}
                      </PrimaryButton>
                    </>
                  )}
                </Space>
              </Form>
            </>
          ),
        },
      ]}
    />
  );
}
