import type { IncidentDiagnosis } from '@/api';

const javaWSAreas = new Set(['CUSTOMER_NETWORK', 'CUSTOMER_JAVA_WS']);

export function isLegacyDiagnosis(diagnosis: IncidentDiagnosis): boolean {
  return !diagnosis.protocolEvidence;
}

export function javaWSStatusLabel(diagnosis: IncidentDiagnosis): string {
  const protocol = diagnosis.protocolEvidence;
  if (!protocol) {
    return javaWSAreas.has(diagnosis.assessment.problemArea)
      ? 'หลักฐานจากระบบรุ่นเดิมไม่เพียงพอสำหรับยืนยันสถานะ Java Web Service'
      : 'ไม่มีหลักฐาน Protocol เพิ่มเติมสำหรับเหตุนี้';
  }
  if (protocol.requestCount < 1) return 'ยังไม่มีหลักฐานว่าคำขอถูกส่งไปยัง Java Web Service';
  if (protocol.httpStatus !== undefined) {
    if (protocol.httpStatus < 200 || protocol.httpStatus >= 300) return `Java Web Service ตอบกลับด้วย HTTP ${protocol.httpStatus}`;
    if (protocol.soapValid === false) return 'เชื่อมต่อสำเร็จ แต่คำตอบไม่ใช่ SOAP ที่ระบบรองรับ';
    if (protocol.base64Valid === false) return 'เชื่อมต่อสำเร็จ แต่ข้อมูลใน SOAP ไม่ใช่ Base64 ที่ถูกต้อง';
    if (protocol.zipSignatureValid === false) return 'เชื่อมต่อสำเร็จและได้รับคำตอบ แต่ข้อมูลไม่ใช่ ZIP';
    if (protocol.zipSignatureValid === true) return 'เชื่อมต่อและรับข้อมูลจาก Java Web Service สำเร็จแล้ว';
    return `เชื่อมต่อสำเร็จและได้รับ HTTP ${protocol.httpStatus}`;
  }
  return 'ส่งคำขอแล้ว แต่ยังไม่มีหลักฐานยืนยันคำตอบจาก Java Web Service';
}

export function protocolRequestLabel(diagnosis: IncidentDiagnosis): string {
  const protocol = diagnosis.protocolEvidence;
  if (!protocol) return 'ไม่มีข้อมูลจำนวนคำขอจากหลักฐานรุ่นเดิม';
  const retry = protocol.retryCount === 0 ? 'ไม่มี Retry' : `Retry ${protocol.retryCount.toLocaleString('th-TH')} ครั้ง`;
  return `${protocol.requestCount.toLocaleString('th-TH')} ครั้ง · ${retry}`;
}

export function matchingSuccessLabel(diagnosis: IncidentDiagnosis): string {
  if (diagnosis.priorMatchingSuccess && diagnosis.subsequentMatchingSuccess) return 'พบรายงานเงื่อนไขเดียวกันที่สำเร็จทั้งก่อนและหลังเหตุ';
  if (diagnosis.priorMatchingSuccess) return 'พบรายงานเงื่อนไขเดียวกันที่สำเร็จก่อนเกิดเหตุ';
  if (diagnosis.subsequentMatchingSuccess) return 'พบรายงานเงื่อนไขเดียวกันที่สำเร็จหลังเกิดเหตุ';
  return 'ยังไม่พบรายงานสำเร็จที่ตรงเงื่อนไขครบสำหรับเปรียบเทียบ';
}
