import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ConfigProvider } from "antd";
import { antdTheme } from "@/lib/antd-theme";
import ProviderForm from "./ProviderForm";
import type { Provider } from "@/api/providers";

const { updateProvider } = vi.hoisted(() => ({ updateProvider: vi.fn() }));

vi.mock("@/queries/useProviders", () => ({
  useCreateProvider: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useUpdateProvider: () => ({ mutateAsync: updateProvider, isPending: false }),
  useProbeProvider: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useProbeConfig: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useProviderAttrRules: () => ({ data: {} }),
}));

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

describe("ProviderForm", () => {
  beforeEach(() => {
    updateProvider.mockReset();
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
});
