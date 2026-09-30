import { mount } from '@vue/test-utils';
import { describe, expect, it } from 'vitest';
import RecipientAiChatCard from './RecipientAiChatCard.vue';

const ToggleSwitchStub = {
  props: ['modelValue', 'disabled', 'inputId'],
  emits: ['update:modelValue'],
  template: '<input type="checkbox" :id="inputId" :checked="modelValue" :disabled="disabled" @change="$emit(\'update:modelValue\', $event.target.checked)" />'
};
const MessageStub = { template: '<div class="message"><slot /></div>' };

function mountCard(props: { enabled: boolean; status: 'PENDING' | 'ACTIVE' | 'REVOKED'; saving?: boolean }) {
  return mount(RecipientAiChatCard, {
    props: { saving: false, ...props },
    global: { stubs: { ToggleSwitch: ToggleSwitchStub, Message: MessageStub } }
  });
}

describe('RecipientAiChatCard', () => {
  it('shows the current state and says the assistant is not live yet', () => {
    const off = mountCard({ enabled: false, status: 'ACTIVE' });
    expect(off.find('.ai-chat-state').text()).toBe('ปิด');
    expect((off.find('input').element as HTMLInputElement).checked).toBe(false);
    expect(off.text()).toContain('ตอนนี้ยังไม่มีผู้ช่วย AI เปิดให้ใช้งาน');
    expect(mountCard({ enabled: true, status: 'ACTIVE' }).find('.ai-chat-state').text()).toBe('เปิด');
  });

  it('emits the requested value when an active recipient flips the switch', async () => {
    const wrapper = mountCard({ enabled: false, status: 'ACTIVE' });
    await wrapper.find('input').setValue(true);
    expect(wrapper.emitted('change')).toEqual([[true]]);
  });

  it('keeps the switch locked for a recipient who has not confirmed LINE', () => {
    const wrapper = mountCard({ enabled: false, status: 'PENDING' });
    expect((wrapper.find('input').element as HTMLInputElement).disabled).toBe(true);
    expect(wrapper.find('.ai-chat-hint').text()).toContain('ยืนยัน LINE');
  });

  it('locks the switch while a save is in flight and shows no hint for an active recipient', () => {
    const wrapper = mountCard({ enabled: true, status: 'ACTIVE', saving: true });
    expect((wrapper.find('input').element as HTMLInputElement).disabled).toBe(true);
    expect(wrapper.find('.ai-chat-hint').exists()).toBe(false);
  });
});
