// SlotItemErrorBoundary：插槽单项错误边界（如侧边栏场景）。
// 失败时渲染 null，不影响宿主导航。
import { Component, type ReactNode } from 'react'

export default class SlotItemErrorBoundary extends Component<{ children: ReactNode }, { failed: boolean }> {
  state = { failed: false }

  static getDerivedStateFromError(): { failed: boolean } {
    return { failed: true }
  }

  componentDidCatch(error: Error) {
    console.error('[ExtensionSlotItem]', error)
  }

  render() {
    return this.state.failed ? null : this.props.children
  }
}
