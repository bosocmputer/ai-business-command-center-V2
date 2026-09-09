---
name: Nextstep Dashboard
description: A calm, accountable Sakai interface for SML operations and executive reporting.
colors:
  primary-50: "#ecfdf5"
  primary-100: "#d1fae5"
  primary-500: "#10b981"
  primary-600: "#059669"
  primary-700: "#047857"
  surface-ground: "#f1f5f9"
  surface-subtle: "#f8fafc"
  surface-card: "#ffffff"
  surface-border: "#e2e8f0"
  text: "#334155"
  text-muted: "#64748b"
  surface-dark: "#0f172a"
  surface-darkest: "#020617"
typography:
  headline:
    fontFamily: "Inter, Noto Sans Thai, system-ui, sans-serif"
    fontSize: "1.65rem"
    fontWeight: 700
    lineHeight: 1.25
    letterSpacing: "-0.02em"
  title:
    fontFamily: "Inter, Noto Sans Thai, system-ui, sans-serif"
    fontSize: "1.25rem"
    fontWeight: 600
    lineHeight: 1.35
  body:
    fontFamily: "Inter, Noto Sans Thai, system-ui, sans-serif"
    fontSize: "1rem"
    fontWeight: 400
    lineHeight: 1.5
  label:
    fontFamily: "Inter, Noto Sans Thai, system-ui, sans-serif"
    fontSize: "0.875rem"
    fontWeight: 600
    lineHeight: 1.4
rounded:
  control: "6px"
  surface: "12px"
  pill: "999px"
spacing:
  xs: "4px"
  sm: "8px"
  md: "16px"
  lg: "24px"
  xl: "32px"
components:
  button-primary:
    backgroundColor: "{colors.primary-600}"
    textColor: "{colors.surface-card}"
    rounded: "{rounded.control}"
    padding: "10px 16px"
  button-primary-hover:
    backgroundColor: "{colors.primary-700}"
    textColor: "{colors.surface-card}"
    rounded: "{rounded.control}"
  card:
    backgroundColor: "{colors.surface-card}"
    textColor: "{colors.text}"
    rounded: "{rounded.surface}"
    padding: "24px"
  input:
    backgroundColor: "{colors.surface-card}"
    textColor: "{colors.text}"
    rounded: "{rounded.control}"
    padding: "10px 12px"
---

# Design System: Nextstep Dashboard

## Overview

**Creative North Star: "ห้องควบคุมที่สงบ"**

Nextstep Dashboard เป็นเครื่องมือทำงานที่ข้อมูลและสถานะนำสายตา ใช้โครงสร้าง Sakai ที่คุ้นเคยเพื่อให้ผู้ดูแลและผู้บริหารทำงานได้ทันทีโดยไม่ต้องเรียนรู้ affordance ใหม่ พื้นผิวสว่างเหมาะกับการอ่านรายงานในที่ทำงานและบนโทรศัพท์ระหว่างวัน ส่วน dark mode รักษาบทบาทสีและลำดับชั้นเดียวกัน

หน้าจอใช้ tonal layering และเส้นขอบบางเป็นหลัก สีเขียวมรกตสงวนไว้สำหรับ primary action, current selection และ success ที่ยืนยันแล้ว ไม่ใช้ card ซ้ำ รูปทรงใหญ่ หรือ motion เพื่อทำให้หน้าดูเต็ม

**Key Characteristics:**

- สงบแต่ไม่จืด ข้อมูลสำคัญมี hierarchy ชัด
- ตารางและฟอร์มหนาแน่นพอสำหรับงาน Admin
- Dashboard อ่านคำตอบได้ก่อนและเปิดรายละเอียดเมื่อจำเป็น
- สถานะใช้ข้อความ ไอคอน และสีร่วมกัน
- รองรับไทย เดสก์ท็อป และ LINE/LIFF ตั้งแต่โครงสร้าง

## Colors

ใช้ Emerald เป็นเสียงหลักบน Slate neutrals เพื่อสื่อความพร้อมและความน่าเชื่อถือ โดยรักษา contrast ของข้อความเป็นอันดับแรก

### Primary

- **Operational Emerald**: ใช้กับปุ่มหลัก เมนูที่เลือก focus ring และข้อมูลสำเร็จที่ยืนยันแล้ว
- **Deep Emerald**: ใช้กับ hover/active และข้อความบนพื้นเขียวอ่อน

### Neutral

- **Slate Ground**: พื้นหลัง app shell เพื่อแยกจาก content surface โดยไม่ใช้เงาหนัก
- **Clear Surface**: พื้นของ card, table, dialog และ form
- **Slate Ink**: ข้อความหลักและตัวเลข
- **Muted Slate**: ข้อความรองที่ยังผ่าน contrast; ห้ามใช้กับข้อมูลสำคัญ
- **Cool Divider**: เส้นแบ่ง table, card และ section

**The One Accent Rule.** สี Emerald ใช้กับ action, selection และ state เท่านั้น ห้ามกระจายสี accent เพื่อการตกแต่ง

## Typography

**Display Font:** Inter พร้อม Noto Sans Thai และ system sans fallback
**Body Font:** Inter พร้อม Noto Sans Thai และ system sans fallback

**Character:** sans-serif หนึ่งชุดให้ภาษาไทย ตัวเลข และ label มีจังหวะสม่ำเสมอ ใช้ weight และ spacing สร้าง hierarchy แทนการเพิ่ม font family

### Hierarchy

- **Headline** (700, 1.65rem, 1.25): ชื่อหน้าเพียงหนึ่งรายการต่อหน้า; mobile ลดเป็น 1.4rem
- **Title** (600, 1.25rem, 1.35): หัวข้อ section, dialog และ chart
- **Body** (400, 1rem, 1.5): ข้อความและคำอธิบาย โดย prose จำกัด 65–75ch
- **Label** (600, 0.875rem, 1.4): form label, table header และ metric label
- **Data** (600–700, ตามบริบท): ใช้ tabular numerals และจัดหน่วยให้คงที่

**The Thai-First Rule.** ใช้คำธุรกิจภาษาไทยเป็นคำหลัก; key, UUID และ status code เป็นรายละเอียดรองหรือข้อมูลสำหรับคัดลอกเท่านั้น

## Elevation

ใช้ tonal layering และ divider เป็นโครงสร้างหลัก พื้นผิวปกติไม่มีเงากว้าง เงาใช้เฉพาะ overlay, popover และ dialog เพื่ออธิบายลำดับชั้นที่ซ้อนจริง

**The Flat-by-Default Rule.** Card ที่พักอยู่บนหน้าใช้ border หรือ tonal contrast อย่างใดอย่างหนึ่ง ห้ามจับคู่เส้นขอบกับเงากว้างเพื่อการตกแต่ง

## Components

### Buttons

- **Shape:** มุมโค้งแบบ control (6px); icon-only action ใช้ rounded variant ของ PrimeVue
- **Primary:** Emerald พร้อมข้อความขาว ใช้หนึ่ง primary action ต่อกลุ่มงาน
- **Hover / Focus:** สีเข้มขึ้นและใช้ focus ring จาก PrimeVue; transition 150–200ms
- **Secondary:** outlined หรือ text ตามลำดับความสำคัญ; destructive action ใช้ danger severity

### Chips

- ใช้ PrimeVue `Tag` สำหรับสถานะและ `Chip` เมื่อผู้ใช้เลือกค่า
- สถานะต้องมีข้อความที่เข้าใจได้ ไม่ใช้สีเพียงอย่างเดียว

### Cards / Containers

- **Corner Style:** 10–12px สำหรับ content surface; ห้ามเกิน 16px
- **Background:** Clear Surface บน Slate Ground
- **Shadow Strategy:** ไม่มีเงาที่ rest; overlay เท่านั้นที่ยกชั้น
- **Border:** divider บางเมื่อจำเป็นต้องแยกขอบเขต
- **Internal Padding:** 16px บน mobile, 24px บน desktop

### Inputs / Fields

- ใช้ PrimeVue control แบบ `fluid`, visible label และ helper/error ใต้ field
- focus ใช้ Emerald focus ring; error ใช้ severity และข้อความแก้ไขได้
- disabled field ต้องมีเหตุผลใกล้ control ไม่พึ่ง tooltip เพียงอย่างเดียว

### Navigation

- ใช้ Sakai topbar/sidebar และ PrimeIcons
- active item ใช้ Emerald tint และน้ำหนักตัวอักษร ไม่ใช้เส้นสีด้านข้างหนา
- mobile เปลี่ยน sidebar เป็น overlay; ปิดหลังเลือก route และคืน focus อย่างถูกต้อง

### Executive Charts

- ใช้ line สำหรับ trend ที่มีจุดเพียงพอ, horizontal bar สำหรับ ranking และ stacked bar สำหรับ composition
- ใช้ PrimeVue Chart กับ theme token; comparison ใช้สีรองและรูปแบบเส้นที่แยกได้โดยไม่พึ่งสี
- ทุก chart มีช่วงข้อมูล หน่วย freshness และตารางข้อมูลที่ screen reader อ่านได้

## Do's and Don'ts

### Do:

- **Do** ใช้ Sakai/PrimeVue component และ semantic severity ก่อน custom CSS
- **Do** แสดงช่วงข้อมูล เวลาไทย และสถานะคุณภาพใกล้ KPI/กราฟทุกชุด
- **Do** ใช้ skeleton สำหรับ content loading และแยก empty, stale, partial, error ให้ชัด
- **Do** วาง action หลักตำแหน่งเดิมในหน้าประเภทเดียวกัน
- **Do** ทดสอบที่ 320, 360, 768, 1024 และ 1440px ทั้ง light/dark

### Don't:

- **Don't** ทำหน้าแบบ SaaS marketing หรือ card grid ซ้ำกันเพื่อเติมพื้นที่
- **Don't** ใช้ custom UI ที่แยกจาก Sakai/PrimeVue หรือ component คนละภาษาในแต่ละหน้า
- **Don't** ใช้ purple gradient, glassmorphism, เงาหนัก หรือมุม card มากกว่า 16px
- **Don't** ใช้ border-left/right หนากว่า 1px เป็น accent
- **Don't** แสดง database field, UUID, status code หรือศัพท์อังกฤษเป็นคำหลักเมื่อมีคำไทยที่ชัดกว่า
- **Don't** ใช้ pie, radar หรือ polar chart เมื่อ ranking/stacked bar อ่านค่าได้แม่นกว่า
- **Don't** รวมจำนวนสินค้าคนละหน่วยหรือแสดง comparison จากข้อมูล truncated/invalid
