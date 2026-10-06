import { it, expect, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ConfigProvider } from "antd";
import KnowledgeSourceOAuth, {
  sourceOAuthCredentials,
} from "./KnowledgeSourceOAuth";
const h = vi.hoisted(() => ({ request: vi.fn() }));
vi.mock("@/api/knowledgeManagement", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/api/knowledgeManagement")>()),
  knowledgeRequest: h.request,
}));
it("keeps pending consent separate from applied credentials", async () => {
  h.request.mockReset();
  h.request
    .mockResolvedValueOnce({
      flow_id: "flow1",
      authorization_url: "https://account.box.com/api/oauth2/authorize",
      expires_in: 900,
    })
    .mockResolvedValueOnce({ pending: true })
    .mockResolvedValueOnce({ credentials: '{"refresh_token":"test-token"}' });
  const applied = vi.fn();
  const user = userEvent.setup();
  render(
    <ConfigProvider>
      <KnowledgeSourceOAuth id="kb1" source="box" onAuthorized={applied} />
    </ConfigProvider>,
  );
  await user.click(screen.getByText("Google / Box 浏览器授权"));
  await user.type(
    screen.getByRole("textbox", { name: "Client ID" }),
    "client1",
  );
  await user.type(screen.getByLabelText("Client secret"), "secret1");
  await user.click(screen.getByRole("button", { name: "发起授权" }));
  const link = await screen.findByRole("link", { name: "打开授权页面" });
  expect(link).toHaveAttribute(
    "href",
    "https://account.box.com/api/oauth2/authorize",
  );
  await user.click(
    screen.getByRole("button", { name: "读取授权结果并填入配置" }),
  );
  await screen.findByText("仍在等待授权完成，可完成授权后再次读取。");
  expect(applied).not.toHaveBeenCalled();
  await user.click(
    screen.getByRole("button", { name: "读取授权结果并填入配置" }),
  );
  await waitFor(() => {
    expect(applied).toHaveBeenCalledWith({
      box_tokens: '{"refresh_token":"test-token"}',
    });
  });
  expect(h.request).toHaveBeenLastCalledWith(
    "post",
    "kb1",
    "sources/oauth/box/result",
    { flow_id: "flow1" },
    undefined,
  );
});
it("uses the exact frozen Google worker credential shape", () => {
  expect(
    sourceOAuthCredentials("google_drive", "tokens-json", "admin@example.com"),
  ).toEqual({
    google_tokens: "tokens-json",
    google_primary_admin: "admin@example.com",
    authentication_method: "oauth_interactive",
  });
});
