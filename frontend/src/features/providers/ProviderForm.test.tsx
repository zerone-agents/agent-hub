import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ConfigProvider } from "antd";
import { antdTheme } from "@/lib/antd-theme";
import ProviderForm from "./ProviderForm";
import type { Provider } from "@/api/providers";

const { updateProvider, probeProvider } = vi.hoisted(() => ({
  updateProvider: vi.fn(),
  probeProvider: vi.fn(),
}));

vi.mock("@/queries/useProviders", () => ({
  useCreateProvider: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useUpdateProvider: () => ({ mutateAsync: updateProvider, isPending: false }),
  useProbeProvider: () => ({ mutateAsync: probeProvider, isPending: false }),
  useProbeConfig: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useProviderAttrRules: () => ({ data: {} }),
}));

// The component calls message.success/error after a probe; mock them so the
// antd Notification state update doesn't fire React act() warnings in jsdom.
vi.mock("antd", async (importOriginal) => {
  const actual = await importOriginal<typeof import("antd")>();
  return {
    ...actual,
    message: { ...actual.message, success: vi.fn(), error: vi.fn() },
  };
});

const editingProvider: Provider = {
  id: 1,
  key: "test-provider",
  name: "Test Provider",
  description: "",
  descriptionEn: "",
  protocol: "openai",
  authStyle: "api_key",
  baseUrl: "https://api.example.com",
  defaultModels: [],
  fields: [],
  attributes: {},
  iconKey: "openai",
  builtin: false,
  lockedApiKey: "sk-o****1234",
  createdAt: "",
  updatedAt: "",
};

function renderForm() {
  return render(
    <ConfigProvider theme={antdTheme}>
      <ProviderForm open editingProvider={editingProvider} onClose={vi.fn()} />
    </ConfigProvider>,
  );
}

// antd Select opens on pointer/mouse-down. Clicking the combobox control for
// a labelled form item, then clicking the visible option text, drives a
// selection reliably across antd in jsdom (KnowledgeForm.test 先例).
async function pickOption(
  user: ReturnType<typeof userEvent.setup>,
  itemLabel: string,
  optionText: string,
) {
  const formItem = screen.getByText(itemLabel).closest<HTMLElement>(".ant-form-item");
  const control = formItem
    ? within(formItem).getByRole("combobox")
    : screen.getByRole("combobox");
  await user.click(control);
  const opt = await screen.findByText(optionText);
  await user.click(opt);
}

describe("ProviderForm", () => {
  beforeEach(() => {
    updateProvider.mockReset();
    probeProvider.mockReset();
  });

  // Helper: match the "更新" button regardless of whether antd inserts
  // a space between CJK characters in the accessible name ("更 新").
  // Antd does this as an aria workaround; the regex tolerates both forms.
  const updateBtn = () =>
    screen.findByRole("button", { name: /更.?新/ });

  it("omits an unchanged masked API key from an update", async () => {
    updateProvider.mockResolvedValue({});
    renderForm();

    await userEvent.setup().click(await updateBtn());

    await waitFor(() => { expect(updateProvider).toHaveBeenCalledTimes(1); });
    expect(updateProvider.mock.calls[0][0].data).not.toHaveProperty(
      "lockedApiKey",
    );
  });

  it("includes a replacement API key in an update", async () => {
    updateProvider.mockResolvedValue({});
    renderForm();

    const user = userEvent.setup();
    const input = await screen.findByPlaceholderText("sk-...");
    await user.clear(input);
    await user.type(input, "sk-replacement-secret-5678");
    await user.click(await updateBtn());

    await waitFor(() => { expect(updateProvider).toHaveBeenCalledTimes(1); });
    expect(updateProvider.mock.calls[0][0].data).toHaveProperty(
      "lockedApiKey",
      "sk-replacement-secret-5678",
    );
  });

  it("probes with the form's unsaved protocol and auth style in edit mode", async () => {
    probeProvider.mockResolvedValue({
      data: { data: { success: true, latencyMs: 12 } },
    });
    renderForm();

    const user = userEvent.setup();
    await pickOption(user, "Protocol", "Anthropic");
    await pickOption(user, "Auth Style", "Auth Token (Bearer header)");
    await user.click(
      await screen.findByRole("button", { name: /测\s*试\s*连\s*接/ }),
    );

    await waitFor(() => { expect(probeProvider).toHaveBeenCalledTimes(1); });
    expect(probeProvider.mock.calls[0][0]).toMatchObject({
      id: 1,
      protocol: "anthropic",
      authStyle: "auth_token",
    });
  });
});
