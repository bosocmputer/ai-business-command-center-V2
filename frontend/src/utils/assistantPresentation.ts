import { ApiError } from '@/api';
import type { AssistantModel, AssistantSecretField } from '@/api';

export function secretLabel(field: AssistantSecretField): string {
  switch (field) {
    case 'openrouter-key': return 'OpenRouter key';
    case 'telegram-bot-token': return 'Telegram bot token';
    case 'line-channel-secret': return 'LINE Channel secret';
    case 'line-channel-token': return 'LINE Channel access token';
  }
}

export function modelStatusLabel(status: AssistantModel['status']): string {
  switch (status) {
    case 'CERTIFIED': return 'ผ่านการใช้งานจริง';
    case 'UNTESTED': return 'ยังไม่ได้ทดสอบครบ';
    case 'SLOW': return 'ช้า ไม่แนะนำ';
    case 'TEST_ONLY': return 'ทดสอบเท่านั้น';
  }
}

export function modelStatusSeverity(status: AssistantModel['status']): 'success' | 'warn' | 'danger' | 'secondary' {
  switch (status) {
    case 'CERTIFIED': return 'success';
    case 'UNTESTED': return 'warn';
    case 'SLOW': return 'secondary';
    case 'TEST_ONLY': return 'danger';
  }
}

const fieldMessages: Record<string, string> = {
  OPENROUTER_KEY_REQUIRED: 'ต้องตั้ง OpenRouter key ก่อนเปิดใช้เลขา',
  LINE_OWN_INCOMPLETE: 'ต้องตั้ง Channel secret และ Channel access token ของร้านก่อนเลือกช่อง LINE ของร้านเอง',
  LINE_CENTRAL_NOT_CONFIGURED: 'ยังไม่ได้ตั้งค่าช่อง LINE กลาง',
  MODEL_FOR_TEST_SHOPS_ONLY: 'โมเดลนี้เลือกได้เฉพาะร้านที่ตั้งเป็นร้านทดสอบ',
  UNKNOWN_MODEL: 'ไม่รู้จักโมเดลนี้',
  UNKNOWN_LINE_MODE: 'โหมด LINE ไม่ถูกต้อง',
  INVALID_FORMAT: 'รูปแบบของรหัสไม่ถูกต้อง ตรวจว่าคัดลอกมาครบและไม่มีช่องว่าง',
  DISABLE_FIRST: 'ปิดใช้เลขาของร้านก่อน จึงลบรหัสนี้ได้',
  NOT_A_GLOBAL_FIELD: 'ช่องนี้ใช้ร่วมกันทุกร้านไม่ได้'
};

/** A message for the admin that never repeats what was typed. */
export function assistantProblemMessage(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.code === 'PASSWORD_CONFIRMATION_FAILED') return 'รหัสผ่าน admin ไม่ถูกต้อง';
    if (error.code === 'PASSWORD_CONFIRMATION_LOCKED') return 'ใส่รหัสผ่านผิดหลายครั้ง ถูกล็อกชั่วคราว ลองใหม่ภายหลัง';
    if (error.code === 'VERSION_CONFLICT') return 'ข้อมูลถูกแก้จากที่อื่นแล้ว โหลดใหม่ให้แล้ว ลองอีกครั้ง';
    const field = error.fieldErrors?.find((item) => fieldMessages[item.code]);
    const known = field ? fieldMessages[field.code] : undefined;
    if (known) return known;
  }
  return 'ทำรายการไม่สำเร็จ ลองใหม่อีกครั้ง';
}
