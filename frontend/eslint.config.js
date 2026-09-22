import js from '@eslint/js'
import globals from 'globals'
import reactHooks from 'eslint-plugin-react-hooks'
import reactRefresh from 'eslint-plugin-react-refresh'
import tseslint from 'typescript-eslint'

export default tseslint.config(
  { ignores: ['dist', 'node_modules', 'coverage', 'src/features/knowledge/KnowledgeForm.test.tsx'] },
  {
    extends: [
      js.configs.recommended,
      ...tseslint.configs.strictTypeChecked,
      ...tseslint.configs.stylisticTypeChecked
    ],
    files: ['**/*.{ts,tsx}'],
    languageOptions: {
      ecmaVersion: 2022,
      globals: globals.browser,
      parserOptions: {
        project: './tsconfig.json',
        tsconfigRootDir: import.meta.dirname
      }
    },
    plugins: {
      'react-hooks': reactHooks,
      'react-refresh': reactRefresh
    },
    rules: {
      ...reactHooks.configs.flat.recommended.rules,
      // react-refresh/only-export-components only affects vite dev HMR —
      // it has no impact on production builds, tests, or CI. Disable to
      // avoid noise from files that legitimately co-export helpers.
      'react-refresh/only-export-components': 'off',
      '@typescript-eslint/restrict-template-expressions': [
        'error',
        {
          allowNumber: true,
          allowBoolean: false,
          allowAny: false,
          allowNullish: true,
          allowRegExp: false
        }
      ],
      '@typescript-eslint/no-unused-vars': [
        'error',
        {
          argsIgnorePattern: '^_',
          varsIgnorePattern: '^_',
          caughtErrorsIgnorePattern: '^_'
        }
      ],
      // Allow async functions in JSX event handler positions and callback
      // arguments — React's idiomatic pattern is `onClick={async () => {...}}`
      // and wrapping every such handler in `void` adds noise without catching
      // real bugs. Conditional misuse (await in `if`) is still flagged.
      '@typescript-eslint/no-misused-promises': [
        'error',
        {
          checksVoidReturn: {
            attributes: false,
            arguments: false
          }
        }
      ]
    }
  },
  {
    // i18n 防回流（#149 P6）：src 业务代码的中文字面量/JSX 文本必须走
    // t()/i18next.t() + src/i18n/locales 资源。豁免：测试文件（断言用中文）、
    // locales 资源本体；确需保留中文的点（双语数据设计/逻辑哨兵值/console
    // 日志）在行内 eslint-disable-next-line 并注明理由。
    files: ['src/**/*.{ts,tsx}'],
    ignores: ['**/*.test.*', 'src/i18n/locales/**', 'src/test/**'],
    rules: {
      'no-restricted-syntax': ['error',
        {
          selector: 'Literal[value=/\\p{Script=Han}/u]',
          message: 'i18n: 字符串字面量含中文——抽取到 src/i18n/locales 并走 t()/i18next.t()（确需保留请 eslint-disable-next-line 并注明理由，见 #149）。'
        },
        {
          selector: 'TemplateElement[value.raw=/\\p{Script=Han}/u]',
          message: 'i18n: 模板字符串含中文——改用 t() 插值 {{var}}（确需保留请 eslint-disable-next-line 并注明理由，见 #149）。'
        },
        {
          selector: 'JSXText[value=/\\p{Script=Han}/u]',
          message: 'i18n: JSX 文本含中文——抽取到 src/i18n/locales 并用 {t(\'...\')}（见 #149）。'
        }
      ]
    }
  },
  {
    // Relax type-aware rules in test files — mocks and fixtures legitimately
    // use `any`, non-null assertions, and floating promises.
    files: ['**/*.test.{ts,tsx}', 'src/test/**'],
    rules: {
      '@typescript-eslint/no-explicit-any': 'off',
      '@typescript-eslint/no-non-null-assertion': 'off',
      '@typescript-eslint/no-floating-promises': 'off',
      '@typescript-eslint/no-unsafe-assignment': 'off',
      '@typescript-eslint/no-unsafe-member-access': 'off',
      '@typescript-eslint/no-unsafe-call': 'off',
      '@typescript-eslint/no-unsafe-return': 'off',
      '@typescript-eslint/no-unsafe-argument': 'off',
      '@typescript-eslint/require-await': 'off',
      '@typescript-eslint/unbound-method': 'off',
      '@typescript-eslint/no-unnecessary-type-assertion': 'off',
      '@typescript-eslint/non-nullable-type-assertion-style': 'off'
    }
  },
  {
    // vite.config.ts is not in tsconfig.json; give it its own project
    // so type-aware rules can run without polluting src/ config.
    files: ['vite.config.ts'],
    languageOptions: {
      parserOptions: {
        project: './tsconfig.node.json',
        tsconfigRootDir: import.meta.dirname
      }
    }
  }
)
