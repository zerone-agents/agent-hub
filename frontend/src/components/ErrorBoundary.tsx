import { Component, type ReactNode } from 'react'
import { Result, Button } from 'antd'
// class 组件不能用 useTranslation hook，走 i18next 实例直调
// （语言切换后已渲染的 fallback 页不重渲染，可接受——错误边界是最后防线）。
import i18next from '@/i18n'

interface Props {
  children: ReactNode
}

interface State {
  error: Error | null
}

export class ErrorBoundary extends Component<Props, State> {
  state: State = { error: null }

  static getDerivedStateFromError(error: Error): State {
    return { error }
  }

  componentDidCatch(error: Error, info: { componentStack: string }) {
    console.error('[ErrorBoundary]', error, info)
  }

  handleReload = () => {
    this.setState({ error: null })
    window.location.reload()
  }

  render() {
    if (this.state.error) {
      return (
        <Result
          status="500"
          title={i18next.t('components.errorBoundary.title')}
          subTitle={this.state.error.message}
          extra={
            <Button type="primary" onClick={this.handleReload}>
              {i18next.t('components.errorBoundary.refresh')}
            </Button>
          }
        />
      )
    }
    return this.props.children
  }
}

export default ErrorBoundary
