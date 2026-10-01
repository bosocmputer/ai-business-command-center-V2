---
status: current
last_verified: 2026-10-01
source_of_truth: [SML ERP document-code table supplied by the project owner, SML production data of the pilot shop]
tags: [sml, reference, dictionary]
---

# SML document codes (`trans_flag`)

`trans_flag` says which SML menu a document came from. The table below is the list supplied by the project owner on 2026-10-01 (148 entries). It is reference data for the numbers dictionary: every report states which codes it counts.

Notes confirmed against the pilot shop's data:

- Code 44 is shared by two menus, 4.18 sales and 5.1 POS sales. `ic_trans.is_pos` tells them apart.
- A cancelled sale keeps its row with `ic_trans.last_status = 1` and gains a code 45 document that references it. Reports count `last_status = 0` only.
- Codes 300 and 144 appear in `cb_trans` (bank and cash book), not in this list.

| Menu | Thai name | English name | Table | `trans_flag` | `trans_type` |
|---|---|---|---|---|---|
| 2.11 | สินค้า/วัตถุดิบ คงเหลือยกมา | Stock Balance Forward | `ic_trans` | 54 | 3 |
| 2.12 | รับสินค้าสำเร็จรูป | Finish Goods Receive | `ic_trans` | 60 | 3 |
| 2.13 | ขอเบิกสินค้า/วัตถุดิบ | Stock Issue Request | `ic_trans` | 122 | 3 |
| 2.14 | เบิกสินค้า/วัตถุดิบ | Material/Goods Issue | `ic_trans` | 56 | 3 |
| 2.15 | รับคืนสินค้า/วัตถุดิบ จากการเบิก | Receive Return of goods from Issue | `ic_trans` | 58 | 3 |
| 2.16.1 | ยกเลิกรับสินค้าสำเร็จรูป | Cancel Finish Product | `ic_trans` | 61 | 3 |
| 2.16.2 | ยกเลิกขอเบิกสินค้า/วัตถุดิบ | Stock Issue Request Cancel | `ic_trans` | 123 | 3 |
| 2.16.3 | ยกเลิกใบเบิกสินค้า/วัตถุดิบ | Cancel Product issue/Material | `ic_trans` | 57 | 3 |
| 2.16.4 | ยกเลิกรับคืนสินค้า/วัตถุดิบจากการเบิก | Cancel Return Product/Material From Issue | `ic_trans` | 59 | 3 |
| 2.17 | ขอโอนสินค้า/วัตถุดิบ | Stock Transfer Request | `ic_trans` | 124 | 3 |
| 2.18 | โอนสินค้า/วัตถุดิบ [เข้า] | Transfer Goods/Raw material (IN) | `ic_trans` | 70 | 3 |
| 2.18 | โอนสินค้า/วัตถุดิบ [ออก] | Transfer Goods/Raw material (OUT) | `ic_trans` | 72 | 3 |
| 2.19.2 | ตรวจนับสินค้า | Check Stock | `ic_trans` | 76 | 3 |
| 2.2 | ปรับปรุงสต๊อกสินค้า/วัตถุดิบ (เกิน) | Stock Adjustment (Increase) | `ic_trans` | 66 | 3 |
| 2.21 | ปรับปรุงสต๊อกสินค้า/วัตถุดิบ (ขาด) | Stock Adjustment (Decrease) | `ic_trans` | 68 | 3 |
| 2.22.1 | ยกเลิกโอนสินค้า/วัตถุดิบ | Cancel Transfer WH Out | `ic_trans` | 73 | 3 |
| 2.22.2 | ยกเลิกปรับปรุงสต๊อกสินค้า/วัตถุดิบ (เกิน) | Cancel Adjust Stk/Over | `ic_trans` | 67 | 3 |
| 2.22.3 | ยกเลิกปรับปรุงสต๊อกสินค้า/วัตถุดิบ (ขาด) | Cancel Adjust Stk/Lost | `ic_trans` | 69 | 3 |
| 3.1 | บันทึกใบเสนอซื้อ | Purchase Requisition | `ic_trans` | 2 | 1 |
| 3.2 | อนุมัติใบเสนอซื้อ | Approve Requistion | `ic_trans` | 4 | 1 |
| 3.3.1 | ยกเลิกใบเสนอซื้อ | Cancel Purchase requisition | `ic_trans` | 3 | 1 |
| 3.4 | บันทึกใบสั่งซื้อ | Purchase Order | `ic_trans` | 6 | 1 |
| 3.5 | อนุมัติใบสั่งซื้อ | Approve Purchase Order | `ic_trans` | 8 | 1 |
| 3.6.1 | ยกเลิกใบสั่งซื้อ | Cancel Purchase Order | `ic_trans` | 7 | 1 |
| 3.8 | จ่ายเงินล่วงหน้า | Advance Payment | `ic_trans` | 10 | 1 |
| 3.9 | รับคืนเงินล่วงหน้า | Return of Receive Earnest Special | `ic_trans` | 20 | 1 |
| 3.10.1 | ยกเลิกจ่ายเงินล่วงหน้า | Cancel Prepayment | `ic_trans` | 150 | 1 |
| 3.12 | จ่ายเงินมัดจำ | Payment Deposit | `ic_trans` | 11 | 1 |
| 3.13 | รับคืนเงินมัดจำ | Deposit Payment Return | `ic_trans` | 25 | 1 |
| 3.14.1 | ยกเลิกจ่ายเงินมัดจำ | Cancel Payment Deposit | `ic_trans` | 151 | 1 |
| 3.15 | ซื้อสินค้า | Purchases order | `ic_trans` | 12 | 1 |
| 3.16 | เพิ่มหนี้เจ้าหนี้,เพิ่มสินค้า | Increased Debt (AP), Additional Products | `ic_trans` | 14 | 1 |
| 3.17 | ลดหนี้เจ้าหนี้, ลดสินค้า | Debit note AP, Return Goods | `ic_trans` | 16 | 1 |
| 3.18.1 | รับสินค้าแบบพาเชียล | Partial_Recieve product | `ic_trans` | 310 | 1 |
| 3.18.2 | ส่งคืน/ราคาผิด | Partial_Produce return or wrong price | `ic_trans` | 311 | 1 |
| 3.18.3 | ตั้งหนี้ | Partial_Increase Debt | `ic_trans` | 315 | 1 |
| 3.18.4 | เพิ่ิ่มหนี้ | Partial_Add debt | `ic_trans` | 316 | 1 |
| 3.18.5 | ลดหนี้ี้ ทยอยรับ | Partial_Credit Note | `ic_trans` | 317 | 1 |
| 3.18.6 | ยกเลิกรับสินค้า | Goods Receipt Cancel | `ic_trans` | 330 | 1 |
| 3.18.7 | ยกเลิกส่งคืนสินค้า/ราคาผิด | Cancel Return/Credit Note | `ic_trans` | 331 | 1 |
| 3.18.8 | ยกเลิกตั้งหนี้จากการรับสินค้า | Cancel Receive from Purchase/Receipt Invoice | `ic_trans` | 335 | 1 |
| 3.18.9 | ยกเลิกลดหนี้จากใบตั้งหนี้ | Cancel Receive from Receipt Credit Note | `ic_trans` | 337 | 1 |
| 3.18.10 | ยกเลิกเพิ่มหนี้จากใบตั้งหนี้ | Cancel Receive from Purchase/Receipt Add | `ic_trans` | 336 | 1 |
| 3.19.1 | ยกเลิกซื้อสินค้า | Cancel Purchases order | `ic_trans` | 13 | 1 |
| 3.19.2 | ยกเลิกส่งคืนสินค้า/ลดหนี้ | Cancel Return/Credit Note | `ic_trans` | 17 | 1 |
| 3.19.3 | ยกเลิกซื้อเพิ่มสินค้า/เพิ่มหนี้ | Cancel Purchase Add/Debt Increase | `ic_trans` | 15 | 1 |
| 4.1 | บันทึกใบเสนอราคา | Sale Quotation | `ic_trans` | 30 | 2 |
| 4.2 | อนุมัติใบเสนอราคา | Approve Quotation | `ic_trans` | 32 | 2 |
| 4.2 | เพิ่มหนี้ | Debit Note | `ic_trans` | 46 | 2 |
| 4.3.1 | ยกเลิกใบเสนอราคา | Cancel Sales Quotation | `ic_trans` | 31 | 2 |
| 4.4 | บันทึกใบสั่งซื้อ/สั่งจอง | Purchase Order/Booking | `ic_trans` | 34 | 2 |
| 4.5 | อนุมัติใบสั่งซื้อ/สั่งจอง | Approve Purchase Order/Inquiry Order | `ic_trans` | 38 | 2 |
| 4.6.1 | ยกเลิกใบสั่งซื้อ/สั่งจอง | Cancel Purchase Order / Sales Reserve | `ic_trans` | 39 | 2 |
| 4.7 | บันทึกใบสั่งขาย | Sale Order Record | `ic_trans` | 36 | 2 |
| 4.8 | อนุมัติใบสั่งขาย | Approve Sale Order | `ic_trans` | 52 | 2 |
| 4.9.1 | ยกเลิกใบสั่งขาย | Cancel Sales Order | `ic_trans` | 37 | 2 |
| 4.11 | รับเงินล่วงหน้า | Receive Earnest Special | `ic_trans` | 40 | 2 |
| 4.12 | คืนเงินล่วงหน้า | Prepayment Return | `ic_trans` | 42 | 2 |
| 4.13.1 | ยกเลิกรับเงินล่วงหน้า | Cancel AR. Earnest | `ic_trans` | 41 | 2 |
| 4.13.2 | ยกเลิกคืนเงินล่วงหน้า | Cancel Prepayment Return | `ic_trans` | 43 | 2 |
| 4.13.2 | ยกเลิกรับคืนเงินล่วงหน้า | Cancel Return of Receive Earnest Special | `ic_trans` | 161 | 1 |
| 4.15 | รับเงินมัดจำ | Deposit Receipt | `ic_trans` | 110 | 2 |
| 4.16 | คืนเงินมัดจำ | Refund Deposit | `ic_trans` | 112 | 2 |
| 4.17.1 | ยกเลิกรับเงินมัดจำ | Cancel Receive Debit | `ic_trans` | 111 | 2 |
| 4.17.2 | ยกเลิกคืนเงินมัดจำ | Cancel Deposit Return | `ic_trans` | 113 | 2 |
| 4.17.2 | ยกเลิกรับคืนเงินมัดจำ | Cancel Deposit Return | `ic_trans` | 152 | 1 |
| 4.18 | ขายสินค้า/บริการ | Sales and Service | `ic_trans` | 44 | 2 |
| 4.19 | รับคืนคืนสินค้า/ลดหนี้ | Credit Note | `ic_trans` | 48 | 2 |
| 4.22.1 | ยกเลิกขายสินค้า/บริการ | Cancel Sale/Service | `ic_trans` | 45 | 2 |
| 4.22.2 | ยกเลิกรับคืนสินค้า/ลดหนี้ | Cancel Returned/Refund Cr. Note | `ic_trans` | 49 | 2 |
| 4.22.3 | ยกเลิกขายเพิ่มสินค้า/เพิ่มหนี้ | Cancel Invoice Add /Debt Increase | `ic_trans` | 47 | 2 |
| 5.1 | ขาย POS | Sale POS | `ic_trans` | 44 | 2 |
| 5.2 | ตั้งหนี้ยกมา(เจ้าหนี้) | Bring Forward IR.(AP) | `ic_trans` | 81 | 4 |
| 5.3 | ลดหนี้ยกมา(เจ้าหนี้) | Credit Note Balance brought forward (AP) | `ic_trans` | 85 | 4 |
| 5.4 | เพิ่มหนี้ยกมา(เจ้าหนี้) | Dedit Note Balance brought forward (AP) | `ic_trans` | 83 | 4 |
| 5.7 | ตั้งหนี้อื่นๆ(เจ้าหนี้) | Other IR.(AP) | `ic_trans` | 87 | 1 |
| 5.8 | ลดหนี้อื่นๆ(เจ้าหนี้) | Other Debt (AP) | `ic_trans` | 91 | 1 |
| 5.9 | เพิ่มหนี้อื่นๆ(เจ้าหนี้) | Other Debit (AP) | `ic_trans` | 89 | 1 |
| 5.10.1 | ยกเลิกตั้งหนี้อื่นๆ(เจ้าหนี้) | Cancel Other IR.(AP) | `ic_trans` | 88 | 1 |
| 5.10.2 | ยกเลิกลดหนี้อื่นๆ(เจ้าหนี้) | Cancel Other Income Decrease (AP) | `ic_trans` | 92 | 1 |
| 5.10.3 | ยกเลิกเพิ่มหนี้อื่นๆ(เจ้าหนี้) | Cancel Other Debit Increase(AP) | `ic_trans` | 90 | 1 |
| 5.11 | ใบรับวางบิล(เจ้าหนี้) | Billing receipt (AP) | `ap_ar_trans` | 213 | 1 |
| 5.12 | จ่ายชำระหนี้(เจ้าหนี้) | Payments (AP) | `ap_ar_trans` | 19 | 1 |
| 5.13.1 | ยกเลิกใบรับวางบิล(เจ้าหนี้) | Cancel Billing receipt (AP) | `ap_ar_trans` | 214 | 1 |
| 5.13.2 | ยกเลิกจ่ายชำระหนี้(เจ้าหนี้) | Cancel Payments | `ap_ar_trans` | 23 | 1 |
| 6.2 | ตั้งหนี้ยกมา(ลูกหนี้) | Bring Forward IR.(AR) | `ic_trans` | 93 | 5 |
| 6.3 | เพิ่มหนี้ยกมา(ลูกหนี้) | Dedit Note Balance brought forward (AR) | `ic_trans` | 95 | 5 |
| 6.3 | ลดหนี้ยกมา(ลูกหนี้) | Credit Note Balance brought forward (AR) | `ic_trans` | 97 | 5 |
| 6.6 | ตั้งหนี้อื่นๆ(ลูกหนี้) | Other IR.(AR) | `ic_trans` | 99 | 2 |
| 6.7 | เพิ่มหนี้อื่นๆ(ลูกหนี้) | Other Debit (AR) | `ic_trans` | 101 | 2 |
| 6.8 | ลดหนี้อื่นๆ(ลูกหนี้) | Other Debt (AR) | `ic_trans` | 103 | 2 |
| 6.9.1 | ยกเลิกตั้งหนี้อื่นๆ(ลูกหนี้) | Cancel Other IR.(AR) | `ic_trans` | 100 | 2 |
| 6.9.2 | ยกเลิกลดหนี้อื่นๆ(ลูกหนี้) | Cancel Other Income Decrease (AR) | `ic_trans` | 104 | 2 |
| 6.9.8 | ยกเลิกเพิ่มหนี้อื่นๆ(ลูกหนี้) | Cancel Other Debit Increase(AR) | `ic_trans` | 102 | 2 |
| 6.1 | ใบวางบิล (ลูกหนี้) | Invoice (Account Payable) | `ap_ar_trans` | 235 | 2 |
| 6.11 | รับชำระหนี้/ออกใบเสร็จรับเงิน | Payment/Receipt | `ap_ar_trans` | 239 | 2 |
| 6.12.1 | ยกเลิกใบวางบิล(ลูกหนี้) | Cancel Pay bill | `ap_ar_trans` | 236 | 2 |
| 6.12.2 | ยกเลิกชำระหนี้/ยกเลิกใบเสร็จรับเงิน | Cancel Debt Billing/Cancel Receipt | `ap_ar_trans` | 240 | 2 |
| 7.1 | รายได้อื่น ๆ | Other income | `ic_trans` | 250 | 2 |
| 7.1.5 | แต้มคงเหลือยกมา | Points accumulated | `ic_trans` | 801 | 0 |
| 7.1.6 | บันทึกตัดแต้ม | Use Points | `ic_trans` | 802 | 0 |
| 7.2 | ลดหนี้รายได้อื่น ๆ | Decrease Debt Other Income | `ic_trans` | 252 | 2 |
| 7.3 | เพิ่มหนี้รายได้อื่น ๆ | &quot;Add debt,other revenue&quot; | `ic_trans` | 254 | 2 |
| 7.4.1 | ยกเลิกรายได้อื่ นๆ | Cancel other revenue | `ic_trans` | 251 | 2 |
| 7.4.2 | ยกเลิกลดหนี้รายได้อื่น ๆ | Cancel debt reduction of other revenue | `ic_trans` | 253 | 2 |
| 7.4.3 | ยกเลิกเพิ่มหนี้รายได้อื่น ๆ | Other income increased debt cancellation. | `ic_trans` | 255 | 2 |
| 7.5 | ค่าใช้จ่ายอื่น ๆ | Other Expense | `ic_trans` | 260 | 2 |
| 7.6 | ลดหนี้ค่าใช้จ่ายอื่น ๆ | Decrease Other Expense | `ic_trans` | 262 | 2 |
| 7.7 | เพิ่มหนี้ค่าใช้จ่ายอื่น ๆ | Expense Other Debt | `ic_trans` | 264 | 2 |
| 7.8.1 | ยกเลิกค่าใช้จ่ายอื่น ๆ | Cancel other charges | `ic_trans` | 261 | 2 |
| 7.8.2 | ยกเลิกลดหนี้ค่าใช้จ่ายอื่น ๆ | Cancel reduce debt other charges | `ic_trans` | 263 | 2 |
| 7.8.3 | ยกเลิกเพิ่มหนี้ค่าใช้จ่ายอื่น ๆ | Increased debt cancellation charges on. | `ic_trans` | 265 | 2 |
| 7.9.3 | บันทึกรับเงินสดย่อย | Petty Cash Return | `ic_trans` | 301 | 2 |
| 7.9.4.1 | ยกเลิกขอเบิกเงินสดย่อย | Cancel Cash Request | `ic_trans` | 302 | 2 |
| 7.9.4.2 | ยกเลิกรับคืนเงินสดย่อย | Cancel Cash Request | `ic_trans` | 303 | 2 |
| 7.10.1.3 | บันทึกฝากเงิน | Payin Cash Record | `ic_trans` | 401 | 2 |
| 7.10.1.4 | บันทึกถอนเงิน | Withdraw Record | `ic_trans` | 402 | 2 |
| 7.10.1.5 | บันทึกโอนเงินระหว่างธนาคาร [เข้า] | Transfer Between Bank | `ic_trans` | 420 | 0 |
| 7.10.1.5 | บันทึกโอนเงินระหว่างธนาคาร [ออก] | Transfer Between Bank | `ic_trans` | 422 | 0 |
| 7.10.1.6.1 | บันทึกยกเลิกฝากเงิน | Cancel Payin Record | `ic_trans` | 403 | 2 |
| 7.10.1.6.1 | บันทึกยกเลิกเงินโอนเข้า | Cancel Transfer Between Bank | `ic_trans` | 423 | 2 |
| 7.10.1.6.2 | บันทึกยกเลิกถอนเงิน | Withdraw cancel record | `ic_trans` | 404 | 2 |
| 7.10.2.1 | เช็ครับยกมา | Cheque Received Balance | `ic_trans` | 405 | 2 |
| 7.10.2.3 | บันทึกนำฝากเช็ครับ | Payin (Cheque Received) | `ic_trans` | 410 | 2 |
| 7.10.2.4 | บันทึกเช็ครับผ่าน | Recieve cheqe pass | `ic_trans` | 411 | 2 |
| 7.10.2.5 | บันทึกเช็ครับคืน | Cheque Return (Received) | `ic_trans` | 412 | 2 |
| 7.10.2.7 | บันทึกเปลี่ยนเช็ครับ | Renew Cheque (Received) | `ic_trans` | 416 | 2 |
| 7.10.2.9.1 | บันทึกยกเลิกนำฝากเช็ครับ | Cancel Chq Pay In Record | `ic_trans` | 430 | 2 |
| 7.10.2.9.2 | บันทึกยกเลิกเช็ครับผ่าน | Recieve cheqe pass | `ic_trans` | 431 | 2 |
| 7.10.2.9.3 | บันทึกยกเลิกเช็ครับคืน | Cheque Cancel Record | `ic_trans` | 432 | 2 |
| 7.10.2.9.4 | บันทึกยกเลิกเช็ครับยกเลิก | Cancel the canceled checks | `ic_trans` | 433 | 2 |
| 7.10.2.9.5 | บันทึกยกเลิกเปลี่ยนเช็คนำฝาก | Cancel Cheque Change | `ic_trans` | 436 | 2 |
| 7.10.3.1 | เช็คจ่ายยกมา | Payment Cheque Balance | `ic_trans` | 406 | 1 |
| 7.10.3.3 | บันทึกเช็คจ่ายผ่าน | Payment cheque | `ic_trans` | 451 | 1 |
| 7.10.3.3 | บันทึกยกเลิกเช็คจ่ายผ่าน | Cheque Cancel Record | `ic_trans` | 471 | 1 |
| 7.10.3.4 | บันทึกเช็คจ่ายคืน | Repay cheque | `ic_trans` | 453 | 1 |
| 7.10.3.6 | บันทึกเปลี่ยนเช็คจ่าย | Cheque Change (Payment) | `ic_trans` | 456 | 1 |
| 7.10.3.7.1 | บันทึกเช็คจ่ายยกเลิก | Cheque Cancel Record (Payment) | `ic_trans` | 452 | 1 |
| 7.10.3.7.1 | บันทึกยกเลิกเช็คจ่ายยกเลิก | Canceled checks paid Cancel | `ic_trans` | 472 | 1 |
| 7.10.3.7.3 | บันทึกยกเลิกเช็คจ่ายคืน | Cancel repay cheqe | `ic_trans` | 473 | 1 |
| 7.10.3.7.4 | บันทึกยกเลิกเปลี่ยนเช็คจ่าย | Cancel Cheque Change (Payment) | `ic_trans` | 476 | 1 |
| 7.10.4.2 | บันทึกขั้นเงินบัตรเครดิต | By credit card | `ic_trans` | 461 | 2 |
| 7.10.4.3 | บันทึกยกเลิกบัตรเครดิต | Cancel Credit Record | `ic_trans` | 462 | 2 |
| 7.10.9.3 | บันทึกยกเลิกเช็ครับ | Cheque Cancel Record | `ic_trans` | 413 | 2 |
| 8.7 | โอนข้อมูลเข้าสู่ระบบบัญชี | Transfers Account System | `as_trans` | 1801 | 0 |
| 9.9 | ประมวลผลสิ้นงวด | Year Process | `gl_journal` | 1998 | 0 |
| 9.1 | ประมวลผลสิ้ิ้นปี | Yesr Process | `gl_journal` | 1999 | 0 |
| 10.2.1 | ข้อมูลรายวัน | Daily Information | `gl_journal` | 0 | 1 |
