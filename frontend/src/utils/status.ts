const labels: Record<string, string> = {
  QUEUED: 'รอคิว', CLAIMED: 'กำลังเตรียม', RUNNING: 'กำลังทำงาน', SUCCEEDED: 'สำเร็จ', FAILED: 'ไม่สำเร็จ', CANCELLED: 'ยกเลิกแล้ว', EXPIRED: 'หมดอายุ',
  PENDING: 'รอดำเนินการ', SENDING: 'กำลังส่ง', ACCEPTED: 'LINE รับแล้ว', RETRY_WAIT: 'รอส่งซ้ำ', UNCERTAIN: 'รอตรวจสอบ', FAILED_PERMANENT: 'ส่งไม่สำเร็จ',
  ACTIVE: 'ใช้งาน', DISABLED: 'ปิดใช้งาน', PAUSED: 'พักไว้', DRAFT: 'ฉบับร่าง', ARCHIVED: 'ลบแล้ว', REVOKED: 'ยกเลิกสิทธิ์',
  READY: 'พร้อม', UNTESTED: 'ยังไม่ได้ทดสอบ', UNCONFIGURED: 'ยังไม่ได้ตั้งค่า',
  SUCCESS: 'สำเร็จ', DENIED: 'ถูกปฏิเสธ',
  ADMIN: 'ผู้ดูแล', VIEWER: 'ผู้ดูรายงาน', WORKER: 'ระบบประมวลผล', SYSTEM: 'ระบบ'
};

export function statusLabel(value?: string | null): string {
  if (!value) return '—';
  return labels[value] ?? value;
}

const reportRunErrors: Record<string, string> = {
  REPORT_LEASE_EXPIRED: 'Worker หยุดติดตามงานระหว่างดึงข้อมูล ระบบปิดงานนี้เพื่อป้องกัน Query ซ้ำ',
  SML_TIMEOUT: 'Server ลูกค้าใช้เวลาตอบนานเกินกำหนด ระบบหยุดรอบนี้โดยไม่ส่ง LINE',
  REPORT_SET_INCOMPLETE: 'สร้างรายงานในรอบนี้ไม่ครบ ระบบจึงไม่ส่ง LINE',
  SML_ZIP_FORMAT_INVALID: 'Server ลูกค้าส่งผลลัพธ์กลับมาในรูปแบบ ZIP ที่ไม่ถูกต้อง',
  SML_ZIP_EMPTY: 'Server ลูกค้าส่งผลลัพธ์ ZIP ที่ไม่มีข้อมูลกลับมา',
  SML_ZIP_TOO_LARGE: 'ผลลัพธ์จาก Server ลูกค้ามีขนาดใหญ่เกินขอบเขตที่ปลอดภัย',
  SML_ZIP_READ_FAILED: 'ระบบอ่านผลลัพธ์ ZIP จาก Server ลูกค้าไม่สำเร็จ',
  SML_ZIP_INVALID: 'ผลลัพธ์ ZIP จาก Server ลูกค้าไม่สมบูรณ์'
};

export function reportRunErrorLabel(value?: string | null): string {
  if (!value) return '';
  return reportRunErrors[value] ?? '';
}

const auditActions: Record<string, string> = {
  TENANT_CREATED: 'สร้างร้านค้า',
  TENANT_UPDATED: 'แก้ไขข้อมูลร้านค้า',
  SML_CONNECTION_REPLACED: 'บันทึกการเชื่อมต่อ SML',
  SML_CONNECTION_TESTED: 'ทดสอบการเชื่อมต่อ SML',
  RECIPIENT_INVITED: 'สร้างลิงก์เชิญผู้รับ',
  RECIPIENT_PERMISSIONS_REPLACED: 'แก้ไขสิทธิ์รายงาน',
  RECIPIENT_IDENTITY_BOUND: 'เชื่อมบัญชี LINE',
  SCHEDULE_CREATED: 'สร้างตารางส่งรายงาน',
  SCHEDULE_UPDATED: 'แก้ไขตารางส่งรายงาน',
  SCHEDULE_ACTIVATED: 'เปิดตารางส่งรายงาน',
  SCHEDULE_PAUSED: 'พักตารางส่งรายงาน',
  SCHEDULE_ARCHIVED: 'ลบตารางส่งรายงาน',
  SCHEDULE_RESTORED: 'กู้คืนตารางส่งรายงาน',
  SCHEDULE_TEST_SEND_ENQUEUED: 'เริ่มทดสอบส่งรายงานจริง'
};

export function auditActionLabel(value?: string | null): string {
  if (!value) return '—';
  return auditActions[value] ?? value.replaceAll('_', ' ');
}
