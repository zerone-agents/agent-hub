import { describe, it, expect } from 'vitest'
import {
  IDENTIFIER_PATTERN,
  AGENT_IDENTIFIER_PATTERN,
  IDENTIFIER_MAX_LENGTH,
  isValidIdentifier,
  isValidAgentIdentifier,
  identifierFormRules,
  agentIdentifierFormRules
} from './identifier'

describe('identifier charset', () => {
  describe('isValidIdentifier', () => {
    it.each([
      ['plain ascii', 'coder'],
      ['with digits', 'agent-01'],
      ['with dot', 'baoyu.skill.v1'],
      ['with underscore', 'web_search'],
      ['with hyphen', 'default-scene'],
      ['all allowed chars', 'Aa0._-z'],
      ['single char', 'x'],
      ['zip filename', 'webapp-testing.zip']
    ])('accepts %s', (_label, value) => {
      expect(isValidIdentifier(value)).toBe(true)
    })

    it.each([
      ['empty', ''],
      ['literal space', ' '],
      ['contains space', 'my agent'],
      ['chinese chars', '技能'],
      ['slash', 'a/b'],
      ['colon', 'host:8080'],
      ['at sign', 'user@x']
    ])('rejects %s', (_label, value) => {
      expect(isValidIdentifier(value)).toBe(false)
    })
  })

  describe('isValidAgentIdentifier', () => {
    it.each([
      ['plain ascii', 'coder'],
      ['with digits in middle', 'agent-01'],
      ['with hyphen', 'default-scene'],
      ['single char', 'x'],
      ['digits and hyphens', 'my-agent-123'],
      ['multiple segments', 'my-cool-agent-v2'],
      ['digit at end', 'agent1']
    ])('accepts %s', (_label, value) => {
      expect(isValidAgentIdentifier(value)).toBe(true)
    })

    it.each([
      ['uppercase', 'MyAgent'],
      ['underscore', 'my_agent'],
      ['dot', 'my.agent'],
      ['space', 'my agent'],
      ['chinese', '技能'],
      ['empty', ''],
      ['digit at start', '1agent'],
      ['hyphen at start', '-agent'],
      ['hyphen at end', 'agent-'],
      ['consecutive hyphens', 'my--agent'],
      ['hyphen-only segment', 'my--agent']
    ])('rejects %s', (_label, value) => {
      expect(isValidAgentIdentifier(value)).toBe(false)
    })
  })

  describe('IDENTIFIER_PATTERN', () => {
    it('matches the same character class as the backend regex', () => {
      // Mirror of internal/application/services/identifier_validator.go
      expect(IDENTIFIER_PATTERN.source).toBe('^[A-Za-z0-9._-]+$')
    })
  })

  describe('AGENT_IDENTIFIER_PATTERN', () => {
    it('matches the stricter agent-only character class', () => {
      expect(AGENT_IDENTIFIER_PATTERN.source).toBe('^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$')
    })
  })

  describe('IDENTIFIER_MAX_LENGTH', () => {
    it('matches the backend cap (64 runes)', () => {
      expect(IDENTIFIER_MAX_LENGTH).toBe(64)
    })
  })

  describe('identifierFormRules', () => {
    it('emits required / max-length / pattern rules in antd shape', () => {
      const rules = identifierFormRules('技能标识')
      expect(rules).toHaveLength(3)
      expect(rules[0]).toMatchObject({ required: true, message: '请输入技能标识' })
      expect(rules[1]).toMatchObject({ max: 64, message: '技能标识长度不能超过 64 个字符' })
      expect(rules[2]).toMatchObject({
        pattern: IDENTIFIER_PATTERN,
        message: '技能标识只能包含字母、数字、点、下划线和横线'
      })
    })

    it('interpolates the label into every message so each form reads naturally', () => {
      const rules = identifierFormRules('代理标识')
      expect(rules.every((r) => String(r.message).includes('代理标识'))).toBe(true)
    })
  })

  describe('agentIdentifierFormRules', () => {
    it('emits agent-only rules with the lowercase/hyphen pattern', () => {
      const rules = agentIdentifierFormRules('代理标识')
      expect(rules).toHaveLength(3)
      expect(rules[0]).toMatchObject({ required: true, message: '请输入代理标识' })
      expect(rules[1]).toMatchObject({ max: 64, message: '代理标识长度不能超过 64 个字符' })
      expect(rules[2]).toMatchObject({
        pattern: AGENT_IDENTIFIER_PATTERN,
        message: '代理标识只能包含小写字母、数字和连字符，必须以字母开头，连字符不能连续或出现在首尾'
      })
    })
  })
})
