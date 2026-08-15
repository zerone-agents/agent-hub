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
