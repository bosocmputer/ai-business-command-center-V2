export function formatDateTime(value?: string | null): string {
  if (!value) return '—';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '—';
  return new Intl.DateTimeFormat('th-TH', { dateStyle: 'medium', timeStyle: 'short', timeZone: 'Asia/Bangkok' }).format(date);
}

export function formatDateTimeWithMilliseconds(value?: string | null): string {
  if (!value) return '—';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '—';
  return new Intl.DateTimeFormat('th-TH', {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    fractionalSecondDigits: 3,
    hourCycle: 'h23',
    timeZone: 'Asia/Bangkok'
  }).format(date);
}

export function formatTime(value?: string | null): string {
  if (!value) return '—';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '—';
  return new Intl.DateTimeFormat('th-TH', {
    hour: '2-digit',
    minute: '2-digit',
    hourCycle: 'h23',
    timeZone: 'Asia/Bangkok'
  }).format(date);
}

export function formatSourceCollection(start?: string | null, finish?: string | null, consistency?: 'STATEMENT' | 'SERIAL_WINDOW' | 'CHUNK_WINDOW'): string {
  if (!finish) return 'ยังไม่มีเวลาที่ดึงข้อมูลจาก SML';
  const startedAt = start ? new Date(start) : undefined;
  const finishedAt = new Date(finish);
  if (Number.isNaN(finishedAt.getTime())) return 'ยังไม่มีเวลาที่ดึงข้อมูลจาก SML';
  const hasWindow = startedAt && !Number.isNaN(startedAt.getTime()) && (consistency === 'CHUNK_WINDOW' || finishedAt.getTime() - startedAt.getTime() >= 60_000);
  if (hasWindow) return `รวบรวมจาก SML ระหว่าง ${formatTime(start)}–${formatTime(finish)} น.`;
  return `ดึงจาก SML สำเร็จเมื่อ ${formatDateTime(finish)}`;
}

export function formatDate(value?: string | null): string {
  if (!value) return '—';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '—';
  return new Intl.DateTimeFormat('th-TH', { dateStyle: 'medium', timeZone: 'Asia/Bangkok' }).format(date);
}

export function formatMetric(value: unknown): string {
  if (value === null || value === undefined || value === '') return '—';
  const text = String(value);
  const number = Number(text.replaceAll(',', ''));
  if (Number.isFinite(number)) return new Intl.NumberFormat('th-TH', { maximumFractionDigits: 2 }).format(number);
  return text;
}

export function formatDateOnly(value: Date): string {
  if (Number.isNaN(value.getTime())) return '';
  const year = String(value.getFullYear()).padStart(4, '0');
  const month = String(value.getMonth() + 1).padStart(2, '0');
  const day = String(value.getDate()).padStart(2, '0');
  return `${year}-${month}-${day}`;
}

const metricLabels: Record<string, string> = {
  doc_date: 'วันที่เอกสาร',
  doc_no: 'เลขที่เอกสาร',
  doc_time: 'เวลาเอกสาร',
  doc_ref: 'เลขที่อ้างอิง',
  doc_ref_date: 'วันที่อ้างอิง',
  cust_code: 'รหัสลูกค้า/ผู้จำหน่าย',
  cust_name: 'ชื่อลูกค้า/ผู้จำหน่าย',
  ar_code: 'รหัสลูกหนี้',
  ar_detail: 'ชื่อลูกหนี้',
  item_code: 'รหัสสินค้า',
  item_name: 'ชื่อสินค้า',
  ic_code: 'รหัสสินค้า',
  ic_name: 'ชื่อสินค้า',
  barcode: 'บาร์โค้ด',
  wh_code: 'คลัง',
  shelf_code: 'ที่เก็บ',
  branch_code: 'สาขา',
  unit_code: 'หน่วย',
  unit_name: 'ชื่อหน่วย',
  qty: 'จำนวน',
  price: 'ราคา',
  sum_amount: 'มูลค่ารายการ',
  total_value: 'มูลค่าก่อนส่วนลด',
  total_discount: 'ส่วนลดรวม',
  total_except_discount: 'มูลค่าหลังส่วนลด',
  total_except_vat: 'มูลค่าก่อนภาษี',
  total_vat_value: 'ภาษีมูลค่าเพิ่ม',
  vat_rate: 'อัตราภาษี',
  vat_type: 'ประเภทภาษี',
  cashier_code: 'รหัสผู้ทำรายการ',
  cash_amount: 'เงินสด',
  transfer_amount: 'เงินโอน',
  card_amount: 'บัตร',
  chq_amount: 'เช็ค',
  coupon_amount: 'คูปอง',
  total_net_value: 'ยอดสุทธิ',
  document_count: 'จำนวนเอกสาร',
  total_amount: 'ยอดรวม',
  gross_profit_amount: 'กำไรขั้นต้น',
  gross_margin_percent: 'อัตรากำไร (%)',
  item_count: 'จำนวนสินค้า',
  balance_amount: 'มูลค่าคงเหลือ',
  reorder_item_count: 'สินค้าถึงจุดสั่งซื้อ',
  shortage_qty: 'จำนวนขาด',
  customer_count: 'จำนวนลูกหนี้',
  net_movement_amount: 'ยอดเคลื่อนไหวสุทธิ',
  receipt_count: 'จำนวนเอกสารรับชำระ',
  total_received_amount: 'ยอดรับชำระ'
};

export function metricLabel(key: string): string {
  return metricLabels[key] ?? key.replaceAll('_', ' ');
}

export function errorMessage(error: unknown): string {
  if (error && typeof error === 'object' && 'code' in error) {
    const code = String(error.code);
    const messages: Record<string, string> = {
      INVALID_CREDENTIALS: 'ชื่อผู้ใช้หรือรหัสผ่านไม่ถูกต้อง',
      LOGIN_LOCKED: 'กรอกรหัสผ่านผิดหลายครั้ง ระบบล็อกชั่วคราว กรุณาลองใหม่ภายหลัง',
      REPORT_ACCESS_FORBIDDEN: 'LINE บัญชีนี้ไม่มีสิทธิ์เปิดรายงานดังกล่าว',
      REPORT_CONCURRENCY_LIMIT: 'ร้านนี้กำลังสร้างรายงานอยู่ กรุณารอสักครู่แล้วลองใหม่',
      REPORT_ROWS_EXPIRED: 'ข้อมูลรายละเอียดหมดอายุแล้ว กรุณาอัปเดตรายงานใหม่',
      REPORT_RUN_NOT_FOUND: 'ไม่พบรายงานนี้หรือรายงานหมดอายุแล้ว',
      REPORT_NOT_CANCELLABLE: 'รายงานทำงานเสร็จแล้ว จึงยกเลิกไม่ได้',
      VIEWER_CONTEXT_CHANGED: 'สิทธิ์หรือร้านค้าที่กำลังดูเปลี่ยนแล้ว กรุณาตรวจสอบและลองใหม่',
      DASHBOARD_REFRESH_NOT_READY: 'ภาพรวมยังประมวลผลไม่เสร็จ กรุณารอสักครู่',
      IDEMPOTENCY_CONFLICT: 'คำขอเดิมถูกใช้กับช่วงข้อมูลอื่นแล้ว กรุณาลองใหม่',
      SML_TEST_BUSY: 'มีรายงานหรือการทดสอบอื่นกำลังติดต่อ Server ลูกค้าอยู่ กรุณารอให้งานเดิมเสร็จก่อน',
      SML_TEST_COOLDOWN: 'เพิ่งทดสอบการเชื่อมต่อร้านนี้ กรุณารอให้ครบช่วงพักก่อนลองใหม่',
      SML_UNREACHABLE: 'Server Dashboard ติดต่อ Java Web Service ของร้านไม่สำเร็จ กรุณาตรวจ Server ลูกค้า Network และ Port',
      SML_TIMEOUT: 'ส่งคำขอไปยัง Server ลูกค้าแล้ว แต่ไม่ได้รับผลยืนยันภายในเวลา กรุณาอย่าเริ่มทดสอบซ้ำทันที',
      VALIDATION_ERROR: 'ข้อมูลที่กรอกไม่ถูกต้อง กรุณาตรวจสอบอีกครั้ง',
      VERSION_CONFLICT: 'ข้อมูลถูกแก้ไขจากที่อื่นแล้ว กรุณาโหลดหน้าใหม่ก่อนบันทึก'
    };
    if (messages[code]) return messages[code];
  }
  if (error && typeof error === 'object' && 'message' in error) return String(error.message);
  return 'เกิดข้อผิดพลาด กรุณาลองใหม่';
}
