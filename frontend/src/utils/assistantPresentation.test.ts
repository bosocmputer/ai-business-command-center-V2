import { describe, expect, it } from 'vitest';
import { ApiError } from '@/api';
import { assistantProblemMessage, modelStatusLabel, secretLabel } from './assistantPresentation';

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
