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
  HOST_IN_USE: 'ชื่อบริการผู้ช่วยนี้ถูกใช้กับร้านอื่นแล้ว แต่ละร้านต้องใช้ชื่อไม่ซ้ำกัน',
  INVALID_HOST: 'ชื่อบริการผู้ช่วยใช้ได้เฉพาะตัวอักษรเล็ก a-z ตัวเลข และขีดกลาง (ไม่เกิน 62 ตัว)',
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
    if (error.code === 'ALREADY_EXISTS') return 'ร้านนี้มีเลขา AI ที่สร้างไว้แล้ว';
    if (error.code === 'HAS_MANUAL_ASSISTANT') return 'ร้านนี้มีเลขา AI ที่ตั้งไว้ด้วยมือบนเซิร์ฟเวอร์แล้ว สร้างซ้ำไม่ได้';
    if (error.code === 'AI_CHAT_DISABLED') return 'ต้องเปิดสิทธิ์ “คุยกับผู้ช่วย AI” ให้ผู้รับคนนี้ก่อน (หน้ากำหนดสิทธิ์ของผู้รับ)';
    const field = error.fieldErrors?.find((item) => fieldMessages[item.code]);
    const known = field ? fieldMessages[field.code] : undefined;
    if (known) return known;
  }
  return 'ทำรายการไม่สำเร็จ ลองใหม่อีกครั้ง';
}

export type AssistantInstanceStatus = 'PENDING' | 'CREATING' | 'RUNNING' | 'STOPPING' | 'STOPPED' | 'ERROR';

/** What an admin reads about the machine of a shop's assistant. The code is a short word from the server, never free text. */
export function instanceStatusLabel(status: AssistantInstanceStatus, code?: string): string {
  switch (status) {
    case 'PENDING': return 'รอเซิร์ฟเวอร์รับงาน (ภายใน 1 นาที)';
    case 'CREATING': return 'กำลังสร้างและเริ่มเลขา AI (ใช้เวลาราว 1–3 นาที)';
    case 'RUNNING': return 'ทำงานอยู่';
    case 'STOPPING': return 'กำลังหยุด';
    case 'STOPPED': return 'หยุดอยู่';
    case 'ERROR': return code ? `ผิดพลาด (${code})` : 'ผิดพลาด';
  }
}

export function instanceStatusSeverity(status: AssistantInstanceStatus): 'success' | 'warn' | 'danger' | 'secondary' | 'info' {
  switch (status) {
    case 'RUNNING': return 'success';
    case 'PENDING':
    case 'CREATING':
    case 'STOPPING': return 'info';
    case 'ERROR': return 'danger';
    case 'STOPPED': return 'secondary';
  }
}

/** Whether the page should keep asking the server how the work is going. */
export function instanceIsChanging(status: AssistantInstanceStatus): boolean {
  return status === 'PENDING' || status === 'CREATING' || status === 'STOPPING';
}
