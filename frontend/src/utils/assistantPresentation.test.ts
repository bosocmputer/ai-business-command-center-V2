import { describe, expect, it } from 'vitest';
import { ApiError } from '@/api';
import { assistantProblemMessage, instanceIsChanging, instanceStatusLabel, instanceStatusSeverity, modelStatusLabel, secretLabel, type AssistantInstanceStatus } from './assistantPresentation';

const problem = (code: string, fieldCode?: string) => new ApiError(422, { code, message: 'x', requestId: 'r', retryable: false, fieldErrors: fieldCode ? [{ field: 'f', code: fieldCode, message: 'm' }] : undefined } as never);

describe('assistant presentation', () => {
  it('explains a refusal by its code and never echoes a value', () => {
    expect(assistantProblemMessage(problem('VALIDATION_ERROR', 'OPENROUTER_KEY_REQUIRED'))).toContain('OpenRouter key');
    expect(assistantProblemMessage(problem('PASSWORD_CONFIRMATION_FAILED'))).toContain('รหัสผ่าน admin');
    expect(assistantProblemMessage(problem('PASSWORD_CONFIRMATION_LOCKED'))).toContain('ล็อก');
    expect(assistantProblemMessage(new Error('sk-or-v1-secret-value'))).not.toContain('sk-or');
  });

  it('names the secrets and the model states in Thai', () => {
    expect(secretLabel('line-channel-token')).toContain('access token');
    expect(modelStatusLabel('TEST_ONLY')).toBe('ทดสอบเท่านั้น');
    expect(modelStatusLabel('SLOW')).toContain('ช้า');
  });
});

describe('the machine of an assistant', () => {
  it('names every state in plain Thai and shows the code only for an error', () => {
    expect(instanceStatusLabel('RUNNING')).toBe('ทำงานอยู่');
    expect(instanceStatusLabel('ERROR', 'START_TIMEOUT')).toBe('ผิดพลาด (START_TIMEOUT)');
    expect(instanceStatusLabel('ERROR')).toBe('ผิดพลาด');
    expect(instanceStatusLabel('PENDING')).toContain('1 นาที');
  });

  it('keeps asking the server only while it is working', () => {
    expect(['PENDING', 'CREATING', 'STOPPING'].every((status) => instanceIsChanging(status as AssistantInstanceStatus))).toBe(true);
    expect(['RUNNING', 'STOPPED', 'ERROR'].some((status) => instanceIsChanging(status as AssistantInstanceStatus))).toBe(false);
    expect(instanceStatusSeverity('ERROR')).toBe('danger');
    expect(instanceStatusSeverity('RUNNING')).toBe('success');
  });
});
