import { mount } from '@vue/test-utils';
import { describe, expect, it, vi } from 'vitest';
import type { AgentTokenStatus } from '@/api';
import RecipientAiChatCard from './RecipientAiChatCard.vue';

const ToggleSwitchStub = {
  props: ['modelValue', 'disabled', 'inputId'],
  emits: ['update:modelValue'],
  template: '<input type="checkbox" :id="inputId" :checked="modelValue" :disabled="disabled" @change="$emit(\'update:modelValue\', $event.target.checked)" />'
};
const ButtonStub = { props: ['label', 'disabled', 'loading'], emits: ['click'], template: '<button type="button" :disabled="disabled" @click="$emit(\'click\')">{{ label }}</button>' };
const MessageStub = { template: '<div class="message"><slot /></div>' };

type Props = { enabled: boolean; status: 'PENDING' | 'ACTIVE' | 'REVOKED'; saving?: boolean; agent?: AgentTokenStatus; agentBusy?: boolean; issuedToken?: string };
const now = new Date('2026-10-01T05:00:00Z');

function mountCard(props: Props) {
  return mount(RecipientAiChatCard, {
    props: { saving: false, now, ...props },
    global: { stubs: { ToggleSwitch: ToggleSwitchStub, Message: MessageStub, Button: ButtonStub } }
  });
}

const tokenState = (overrides: Partial<AgentTokenStatus['info']> = {}, enabled = true): AgentTokenStatus => ({
  enabled, info: { status: 'ACTIVE', namesVisible: false, calls24h: 7, createdAt: '2026-09-01T00:00:00Z', expiresAt: '2026-12-30T00:00:00Z', lastUsedAt: '2026-10-01T04:00:00Z', ...overrides }
});
const buttonByLabel = (wrapper: ReturnType<typeof mountCard>, label: string) => wrapper.findAll('button').find((button) => button.text() === label);

describe('RecipientAiChatCard switch', () => {
  it('shows the current state and says the switch waits for a token', () => {
    const off = mountCard({ enabled: false, status: 'ACTIVE' });
    expect(off.find('.ai-chat-state').text()).toBe('ปิด');
    expect((off.find('#ai-chat-switch').element as HTMLInputElement).checked).toBe(false);
    expect(off.text()).toContain('ผู้ช่วยจะใช้ได้เมื่อมีโทเคน');
    expect(mountCard({ enabled: true, status: 'ACTIVE' }).find('.ai-chat-state').text()).toBe('เปิด');
  });

  it('emits the requested value when an active recipient flips the switch', async () => {
    const wrapper = mountCard({ enabled: false, status: 'ACTIVE' });
    await wrapper.find('#ai-chat-switch').setValue(true);
    expect(wrapper.emitted('change')).toEqual([[true]]);
  });

  it('keeps the switch locked for a recipient who has not confirmed LINE, or while saving', () => {
    const pending = mountCard({ enabled: false, status: 'PENDING' });
    expect((pending.find('#ai-chat-switch').element as HTMLInputElement).disabled).toBe(true);
    expect(pending.find('.ai-chat-hint').text()).toContain('ยืนยัน LINE');
    const saving = mountCard({ enabled: true, status: 'ACTIVE', saving: true });
    expect((saving.find('#ai-chat-switch').element as HTMLInputElement).disabled).toBe(true);
    expect(saving.find('.ai-chat-hint').exists()).toBe(false);
  });
});

describe('RecipientAiChatCard assistant token', () => {
  it('explains that the whole assistant is off instead of offering a token that cannot be used', () => {
    const wrapper = mountCard({ enabled: true, status: 'ACTIVE', agent: tokenState({ status: 'NONE' }, false) });
    expect(wrapper.find('.ai-chat-system-off').text()).toContain('AGENT_API_ENABLED');
    expect(buttonByLabel(wrapper, 'ออกโทเคน')).toBeUndefined();
  });

  it('shows no token section while the recipient has not been given the assistant', () => {
    const wrapper = mountCard({ enabled: false, status: 'ACTIVE', agent: tokenState({ status: 'NONE' }) });
    expect(wrapper.find('.ai-chat-token').exists()).toBe(false);
  });

  it('says plainly when there is no token and issues one with the names choice', async () => {
    const wrapper = mountCard({ enabled: true, status: 'ACTIVE', agent: tokenState({ status: 'NONE' }) });
    expect(wrapper.find('.ai-chat-token-status').text()).toContain('ยังไม่มีโทเคน');
    expect(buttonByLabel(wrapper, 'ยกเลิกโทเคน')).toBeUndefined();
    await wrapper.find('#ai-names-switch').setValue(true);
    await buttonByLabel(wrapper, 'ออกโทเคน')!.trigger('click');
    expect(wrapper.emitted('issue')).toEqual([[true]]);
  });

  it('shows when a live token expires, how much it was used, and what the names setting means', () => {
    const wrapper = mountCard({ enabled: true, status: 'ACTIVE', agent: tokenState() });
    expect(wrapper.find('.ai-chat-token-status').text()).toContain('ใช้งานได้');
    expect(wrapper.find('.ai-chat-token-status').text()).toContain('อีก 90 วัน');
    expect(wrapper.text()).toContain('24 ชั่วโมงที่ผ่านมา 7 คำขอ');
    expect(wrapper.text()).toContain('ถูกส่งให้ผู้ให้บริการโมเดล AI');
    expect(wrapper.find('.ai-chat-expiring').exists()).toBe(false);
  });

  it('warns two weeks before a token expires and says so when it already has', () => {
    const soon = mountCard({ enabled: true, status: 'ACTIVE', agent: tokenState({ expiresAt: '2026-10-10T00:00:00Z' }) });
    expect(soon.find('.ai-chat-expiring').text()).toContain('อีก 9 วัน');
    const expired = mountCard({ enabled: true, status: 'ACTIVE', agent: tokenState({ status: 'EXPIRED', expiresAt: '2026-09-20T00:00:00Z' }) });
    expect(expired.find('.ai-chat-token-status').text()).toContain('หมดอายุแล้ว');
    expect(expired.find('.ai-chat-expiring').exists()).toBe(false);
  });

  it('asks before replacing a live token and before revoking it, and does neither if the admin backs out', async () => {
    const wrapper = mountCard({ enabled: true, status: 'ACTIVE', agent: tokenState({ namesVisible: true }) });
    await buttonByLabel(wrapper, 'ออกโทเคนใหม่')!.trigger('click');
    expect(wrapper.emitted('issue')).toBeUndefined();
    expect(wrapper.find('.ai-chat-confirm').text()).toContain('โทเคนเดิมจะใช้ไม่ได้ทันที');
    await buttonByLabel(wrapper, 'ไม่ทำ')!.trigger('click');
    expect(wrapper.find('.ai-chat-confirm').exists()).toBe(false);

    await buttonByLabel(wrapper, 'ออกโทเคนใหม่')!.trigger('click');
    await wrapper.findAll('.ai-chat-confirm button')[0]!.trigger('click');
    // The names choice follows the token that exists until the admin changes it.
    expect(wrapper.emitted('issue')).toEqual([[true]]);

    await buttonByLabel(wrapper, 'ยกเลิกโทเคน')!.trigger('click');
    expect(wrapper.find('.ai-chat-confirm').text()).toContain('เข้าถึงข้อมูลร้านไม่ได้ทันที');
    await wrapper.findAll('.ai-chat-confirm button')[0]!.trigger('click');
    expect(wrapper.emitted('revoke')).toHaveLength(1);
  });

  it('shows a new token once, lets the admin copy it, and clears it when closed', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true });
    const wrapper = mountCard({ enabled: true, status: 'ACTIVE', agent: tokenState(), issuedToken: 'abcc_secret-value' });
    expect(wrapper.text()).toContain('แสดงครั้งเดียว');
    expect((wrapper.find('.ai-chat-token-value').element as HTMLInputElement).value).toBe('abcc_secret-value');
    await buttonByLabel(wrapper, 'คัดลอก')!.trigger('click');
    expect(writeText).toHaveBeenCalledWith('abcc_secret-value');
    expect(buttonByLabel(wrapper, 'คัดลอกแล้ว')).toBeDefined();
    await buttonByLabel(wrapper, 'ปิด')!.trigger('click');
    expect(wrapper.emitted('dismissToken')).toHaveLength(1);
  });

  it('does not show a token that was never issued in this visit', () => {
    expect(mountCard({ enabled: true, status: 'ACTIVE', agent: tokenState() }).find('.ai-chat-issued').exists()).toBe(false);
  });
});
