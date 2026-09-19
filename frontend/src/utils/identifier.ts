/**
 * Identifier charset rules shared with the backend.
 *
 * Backend reference: internal/application/services/identifier_validator.go
 * (validateIdentifier — non-empty, ≤64 chars, /^[A-Za-z0-9._-]+$/).
 *
 * Keep this in sync with that pattern. Used both for live form validation
 * and for client-side pre-checks like skill zip filenames, where we want
 * to fail fast with a clear message instead of bouncing off the API.
 */
export const IDENTIFIER_PATTERN = /^[A-Za-z0-9._-]+$/

/**
 * Agent identifier charset is stricter than the shared one:
 *   - starts with a lowercase letter
 *   - ends with a lowercase letter or digit
 *   - segments are lowercase letters/digits separated by single hyphens
 *   - no leading/trailing hyphens and no consecutive hyphens
 *
 * Backend reference: internal/application/services/agent_validator.go
 * (ValidateAgentName — /^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$/).
 */
export const AGENT_IDENTIFIER_PATTERN = /^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$/

export const IDENTIFIER_MAX_LENGTH = 64

export function isValidIdentifier(value: string): boolean {
  return IDENTIFIER_PATTERN.test(value)
}

export function isValidAgentIdentifier(value: string): boolean {
  return AGENT_IDENTIFIER_PATTERN.test(value)
}

/**
 * Antd `Form.Item` rules for a unique identifier field. The label is
 * interpolated into the error messages so each form gets natural copy
 * (e.g. "技能标识只能包含...").
 *
 * Usage:
 *   <Form.Item label={t('...')} name="name" rules={identifierFormRules(t('...'))}>
 */
import i18next from '@/i18n'

export function identifierFormRules(label: string) {
  return [
    { required: true, message: i18next.t('common.form.required', { label }) },
    { max: IDENTIFIER_MAX_LENGTH, message: i18next.t('common.form.maxLength', { label, max: IDENTIFIER_MAX_LENGTH }) },
    { pattern: IDENTIFIER_PATTERN, message: i18next.t('common.form.identifierCharset', { label }) }
  ]
}

/**
 * Antd `Form.Item` rules for an agent identifier. Use only for agent names,
 * which are restricted to lowercase letters, digits and hyphens.
 *
 * Usage:
 *   <Form.Item label="代理标识" name="name" rules={agentIdentifierFormRules('代理标识')}>
 */
export function agentIdentifierFormRules(label: string) {
  return [
    { required: true, message: i18next.t('common.form.required', { label }) },
    { max: IDENTIFIER_MAX_LENGTH, message: i18next.t('common.form.maxLength', { label, max: IDENTIFIER_MAX_LENGTH }) },
    { pattern: AGENT_IDENTIFIER_PATTERN, message: i18next.t('common.form.agentCharset', { label }) }
  ]
}
