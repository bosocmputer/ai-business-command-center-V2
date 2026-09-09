import { describe, expect, it } from 'vitest';
import { schedulePeriodExample, schedulePeriodGroupDetail, schedulePeriodOptions } from './schedulePeriodPresentation';

describe('schedule period presentation', () => {
  it('uses labels that describe dates relative to each delivery', () => {
    expect(schedulePeriodOptions.map((option) => option.label)).toEqual([
      'ข้อมูลของวันก่อนวันที่ส่ง',
      'ตั้งแต่ 00:00 ถึงเวลาส่ง',
      'ตั้งแต่วันที่ 1 ของเดือนถึงเวลาส่ง',
      'ข้อมูลล่าสุด ณ เวลาส่ง'
    ]);
  });

  it('explains the previous-day option with the configured weekday and time', () => {
    expect(schedulePeriodExample('YESTERDAY', '08:00', [2])).toBe(
      'ตัวอย่าง: ส่งวันอังคาร เวลา 08:00 น. ระบบจะใช้ข้อมูลของวันจันทร์'
    );
  });

  it('explains same-day and month-to-date options without referring to setup day', () => {
    expect(schedulePeriodExample('TODAY_TO_NOW', '09:00', [1])).toBe(
      'ตัวอย่าง: ส่งวันจันทร์ เวลา 09:00 น. ระบบจะใช้ข้อมูลวันจันทร์ ตั้งแต่ 00:00–09:00 น.'
    );
    expect(schedulePeriodExample('MONTH_TO_DATE', '09:00', [1])).toBe(
      'ตัวอย่าง: ส่งวันจันทร์ เวลา 09:00 น. ระบบจะใช้ข้อมูลตั้งแต่วันที่ 1 ของเดือนจนถึงเวลา 09:00 น. ของวันจันทร์'
    );
  });

  it('keeps current-only reports honest regardless of the selected period', () => {
    expect(schedulePeriodGroupDetail('CURRENT_ONLY', 'YESTERDAY', '08:00')).toBe(
      'ใช้ข้อมูลล่าสุด ณ 08:00 น. และไม่รองรับข้อมูลย้อนหลัง'
    );
  });
});
