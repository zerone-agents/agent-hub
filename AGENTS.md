# Agent Guidelines

## Frontend Conventions

### Use PrimaryButton for all primary-filled buttons

All primary-filled buttons (`type="primary"` Buttons) must use the shared `frontend/src/components/PrimaryButton` component instead of writing `<Button type="primary">` directly, to keep the primary button style consistent across pages (accent fill, unified disabled state).

```tsx
// ✅ Correct
import PrimaryButton from '@/components/PrimaryButton'
<PrimaryButton icon={<PlusIcon size={16} />}>New</PrimaryButton>

// ❌ Wrong
<Button type="primary">New</Button>
```

Modal OK buttons don't need to be replaced wholesale — inject the style via `okButtonProps` with `usePrimaryButtonStyle()` instead (see the comment at the top of `frontend/src/components/PrimaryButton.tsx` for details).

## Release Process

Releases are tag-driven: pushing a `hub-vX.Y.Z` tag on `main` triggers `release.yml`, which builds and pushes the Docker image to both registries after all checks (lint / test / frontend-test-full / frontend-lint) pass.

### Pre-release checklist

1. Confirm CI and CodeQL on the latest `main` push are all green.
2. Run local pre-checks: `gofmt -l` and `go vet` (the release lint job enforces gofmt).

### Cut a release

```bash
git checkout main && git pull
git tag hub-vX.Y.Z
git push origin hub-vX.Y.Z
```

The workflow pushes the image (both `vX.Y.Z` and `latest` tags) to two registries:

```
# Primary (China)
swr.cn-east-3.myhuaweicloud.com/zerone/agent-hub:vX.Y.Z
swr.cn-east-3.myhuaweicloud.com/zerone/agent-hub:latest

# International / fallback
docker.io/zeroneai/agent-hub:vX.Y.Z
docker.io/zeroneai/agent-hub:latest
```

### Create the GitHub Release

`release.yml` does NOT create the GitHub Release automatically. After the workflow succeeds, create it manually with `gh release create`:

- Title format: `Hub-vX.Y.Z`, marked as **Latest**
- Notes follow the fixed structure:
  1. `## 🎨/🐛/👥 <theme>` section opening with "Patch 版本：" / "Minor 版本："
  2. `## ✨ 变更` — changes with PR numbers
  3. `## 🐳 镜像` — code block with both registries (see above)
  4. `## ✅ 验证` — verification evidence
  5. `## 🔗 完整变更` — compare link (`hub-vPrevious...hub-vX.Y.Z`)

Note: image mirrors such as daocloud may return 403 when pulling Docker Hub — that is a proxy issue; verify pushes via the Docker Hub API or the GitHub Actions logs instead.
