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
 *   <Form.Item label="技能标识" name="name" rules={identifierFormRules('技能标识')}>
 */
export function identifierFormRules(label: string) {
  return [
    { required: true, message: `请输入${label}` },
    { max: IDENTIFIER_MAX_LENGTH, message: `${label}长度不能超过 ${IDENTIFIER_MAX_LENGTH} 个字符` },
    { pattern: IDENTIFIER_PATTERN, message: `${label}只能包含字母、数字、点、下划线和横线` }
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
    { required: true, message: `请输入${label}` },
    { max: IDENTIFIER_MAX_LENGTH, message: `${label}长度不能超过 ${IDENTIFIER_MAX_LENGTH} 个字符` },
    { pattern: AGENT_IDENTIFIER_PATTERN, message: `${label}只能包含小写字母、数字和连字符，必须以字母开头，连字符不能连续或出现在首尾` }
  ]
}
