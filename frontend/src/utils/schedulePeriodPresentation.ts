import type { ScheduleInput } from '@/api';

export type SchedulePeriodMode = 'DATE_RANGE' | 'AS_OF_DATE' | 'CURRENT_ONLY';

type SchedulePeriodPreset = ScheduleInput['periodPreset'];

const weekdayLabels = ['อาทิตย์', 'จันทร์', 'อังคาร', 'พุธ', 'พฤหัสบดี', 'ศุกร์', 'เสาร์'] as const;

export const schedulePeriodOptions: { label: string; value: SchedulePeriodPreset }[] = [
  { label: 'ข้อมูลของวันก่อนวันที่ส่ง', value: 'YESTERDAY' },
  { label: 'ตั้งแต่ 00:00 ถึงเวลาส่ง', value: 'TODAY_TO_NOW' },
  { label: 'ตั้งแต่วันที่ 1 ของเดือนถึงเวลาส่ง', value: 'MONTH_TO_DATE' },
  { label: 'ข้อมูลล่าสุด ณ เวลาส่ง', value: 'AS_OF_RUN' }
];

export function schedulePeriodExample(preset: SchedulePeriodPreset, localTime: string, daysOfWeek: number[]): string {
  const selectedDay = [...daysOfWeek].sort((left, right) => left - right).find((day) => day >= 0 && day <= 6);
  if (selectedDay === undefined || !/^([01][0-9]|2[0-3]):[0-5][0-9]$/.test(localTime)) {
    return 'ระบบจะคำนวณวันที่ของข้อมูลใหม่ทุกครั้งตามวันและเวลาที่ส่ง';
  }

  const deliveryDay = weekdayLabels[selectedDay];
  const prefix = `ตัวอย่าง: ส่งวัน${deliveryDay} เวลา ${localTime} น.`;
  if (preset === 'YESTERDAY') {
    const previousDay = weekdayLabels[(selectedDay + 6) % 7];
    return `${prefix} ระบบจะใช้ข้อมูลของวัน${previousDay}`;
  }
  if (preset === 'TODAY_TO_NOW') {
    return `${prefix} ระบบจะใช้ข้อมูลวัน${deliveryDay} ตั้งแต่ 00:00–${localTime} น.`;
  }
  if (preset === 'MONTH_TO_DATE') {
    return `${prefix} ระบบจะใช้ข้อมูลตั้งแต่วันที่ 1 ของเดือนจนถึงเวลา ${localTime} น. ของวัน${deliveryDay}`;
  }
  return `${prefix} ระบบจะใช้ข้อมูลล่าสุด ณ เวลา ${localTime} น.`;
}

export function schedulePeriodGroupDetail(mode: SchedulePeriodMode, preset: SchedulePeriodPreset, localTime: string): string {
  if (mode === 'CURRENT_ONLY') return `ใช้ข้อมูลล่าสุด ณ ${localTime} น. และไม่รองรับข้อมูลย้อนหลัง`;
  if (mode === 'AS_OF_DATE') return preset === 'YESTERDAY' ? 'ใช้ข้อมูล ณ วันก่อนวันที่ส่ง' : 'ใช้ข้อมูล ณ วันที่ส่ง';
  if (preset === 'YESTERDAY') return 'ใช้ข้อมูลของวันก่อนวันที่ส่ง';
  if (preset === 'MONTH_TO_DATE') return 'ใช้ข้อมูลตั้งแต่วันที่ 1 ของเดือนถึงเวลาส่ง';
  return 'ใช้ข้อมูลตั้งแต่ 00:00 ถึงเวลาส่ง';
}
