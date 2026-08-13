import { EyeInvisibleOutlined, EyeOutlined } from '@ant-design/icons';
import { Input, type InputProps, type InputRef } from 'antd';
import { createStyles } from 'antd-style';
import { forwardRef, useState } from 'react';

const useStyles = createStyles(({ css }) => ({
  // 浏览器自动填充时 Chrome 只给内层 input 涂蓝色 inset 阴影，
  // affix-wrapper（边框所在层）保持白色，出现"框里套框"。
  // 用 :has() 检出 autofilled 状态，把 wrapper 背景同步成自动填充蓝。
  root: css`
    &:has(input:-webkit-autofill) {
      background-color: #e8f0fe;
    }
  `,
}));

/**
 * 密码输入框。
 *
 * 与 antd `Input.Password` 的区别：
 * 1. 眼睛切换按钮 `tabIndex={-1}`，不进入 Tab 顺序——多个密码框之间
 *    Tab 直接跳到下一个输入框，而不是先停在眼睛按钮上。
 * 2. 自动填充时 wrapper 背景跟随内层 input 变蓝，视觉与普通 Input 一致。
 */
const PasswordInput = forwardRef<InputRef, Omit<InputProps, 'type' | 'suffix'>>(
  ({ value, onChange, onPressEnter, className, ...rest }, ref) => {
    const { styles, cx } = useStyles();
    const [visible, setVisible] = useState(false);
    return (
      <Input
        {...rest}
        ref={ref}
        className={cx(styles.root, className)}
        type={visible ? 'text' : 'password'}
        value={value}
        onChange={onChange}
        onPressEnter={onPressEnter}
        suffix={
          <span
            role="button"
            tabIndex={-1}
            aria-label={visible ? '隐藏密码' : '显示密码'}
            aria-pressed={visible}
            onMouseDown={(e) => { e.preventDefault(); }}
            onClick={() => { setVisible((v) => !v); }}
            style={{ cursor: 'pointer', color: 'var(--ant-color-text-tertiary)' }}
          >
            {visible ? <EyeOutlined /> : <EyeInvisibleOutlined />}
          </span>
        }
      />
    );
  },
);

PasswordInput.displayName = 'PasswordInput';

export default PasswordInput;
