---
status: current
last_verified: 2026-10-01
source_of_truth: [internal/report/query_plan.go, internal/report/summary_query_plan.go, internal/report/heavy_chunk.go, smlmcpconnect tool SQL, SML field and document codes in this folder]
tags: [sml, reference, dictionary, reports]
---

# Numbers dictionary

One word means one number. Every report and every assistant answer uses the definition here, and says which period and which document codes it counted. Code meanings are in [trans-flag.md](trans-flag.md) and [field-codes.md](field-codes.md).

The rules below describe what the V2 SQL does today. Where V2 and the older smlmcpconnect tools disagree, both are listed and one is marked as the rule to follow.

## Rules for every document query

- Count normal documents only: `last_status = 0`. A cancelled sale keeps its row with `last_status = 1` and also creates a code 45 document that points at it, so counting only `last_status = 0` never double counts.
- Only `last_status` 0 and 1 are used (confirmed by the owner). Count 0 and ignore anything else.
- Skip copies: `is_doc_copy = 0`.
- POS: code 44 is shared by the sales menu and the POS menu. V2's sales report drops a POS bill only when it also has a `doc_ref`. A shop that uses POS needs this rule checked.
- Cash or credit comes from `inquiry_type`, not from `credit_day` or `pay_amount`.
- VAT treatment comes from `vat_type` (0 added on top, 1 included, 2 rate 0%, 3 no effect on tax), not from whether `total_vat_value` is zero.
- Header `total_except_vat` was empty on every document of the pilot shop. Do not rely on it. Get amounts before VAT from line `sum_amount_exclude_vat`.

## Terms

| Term (Thai) | Definition | Where it is used |
|---|---|---|
| ยอดขาย (sales, including VAT), the headline figure | Header `total_amount` of codes 44 and 46 minus code 48. Includes cash and credit sales, so a return lowers the day's sales and a debit note raises it. Always labelled "including VAT". Decided by the owner on 2026-10-01. | Sales report and every answer that says "sales". The sales report counted code 44 only until this decision was applied, and smlmcpconnect's sales tools still do. |
| ยอดขายก่อน VAT | Line `sum_amount_exclude_vat` of codes 44 and 46 minus code 48. Used next to profit, and always named "before VAT". | Gross profit reports |
| VAT ขาย | Header `total_vat_value` of codes 44 and 46 minus code 48. Zero for `vat_type` 2 and 3. | Sales report |
| ยอดขายสุทธิ (net sales before VAT) | Lines of codes 44 and 46 minus lines of code 48, using `sum_amount_exclude_vat`. | Gross profit reports |
| ต้นทุนขาย | Line `sum_of_cost` of codes 44 and 46 minus code 48. | Gross profit reports |
| กำไรขั้นต้น | Net sales before VAT minus cost of sales. smlmcpconnect uses code 44 only, so it differs from V2 whenever there are returns or debit notes. Follow V2. | Gross profit by product and by customer |
| ยอดซื้อ | Sum of header `total_amount` of code 12 documents. | Purchase report |
| ลูกหนี้คงค้าง (receivable balance) | As of a date: debits minus credits per customer. Debits: codes 44 and 250 with `inquiry_type` 0 or 2, 46, and opening balances 93, 99, 95, 101, 254, 418. Credits: 48 with `inquiry_type` 0, 2 or 4, 97, 103, 262 with `inquiry_type` not 1 or 3, debt receipts 239 (`total_net_value`) and 1802. | Receivable movement report |
| รับชำระหนี้ | `ap_ar_trans` code 239, amount `total_net_value`. The channel split only shows cash and transfer, so a cheque or card payment shows no split. | Debt receipt report |
| เงินรับ (receipts) | Cash and bank book rows with `pay_type = 1`, `status = 0`, code not 144, whose source document is normal. It is every money received, not only from sales: debt receipts, advance receipts and other income all count. | Cash and bank receipts report |
| เงินจ่าย (payments) | Cash and bank book rows with `pay_type = 2` and `status = 0`. | Cash and bank payments report |
| ช่องทางชำระ | `cash_amount`, `card_amount`, `chq_amount`, `tranfer_amount`, `coupon_amount` on the cash book row. A row whose channels do not add up to `total_amount` shows the difference as "ไม่ระบุช่องทาง". Never present that difference as a person's mistake. | Receipts and payments |
| สต็อกคงเหลือ (stock on hand by movement) | Built from line movements up to the end date. A code list decides which documents add and which remove stock. It excludes item types 1 and 3, lines of item type 5, and lines of copies or POS bills with a `doc_ref`. It reproduces the SML stock card. | Stock balance report |
| คงเหลือตามทะเบียนสินค้า | `ic_inventory.balance_qty`, the figure stored on the item master. | Reorder report |
| ลูกหนี้ตามอายุหนี้ (`ar_aging`) | As of a date, per open document: `total_amount` minus debt receipts (code 239, matched by billing number and document date, payments up to the as-of date). Debit documents are 44 and 250 (`inquiry_type` 0 or 2), 46, the opening balances 93, 99, 95, 101 and the added-debt types 254 and 418, the same `ic_trans` types the receivable movement report counts. Credit documents (48 with `inquiry_type` 0, 2 or 4, 97, 103, and 262 unless `inquiry_type` is 1 or 3) carry a negative balance. Fixed-asset receipts (`as_trans` 1802) are in the movement report but not in aging, so a shop that uses them will see aging lower than movement by that amount; the pilot shop has none. The due date is the document's `due_date`, or the document date plus `ic_trans.credit_day` when that is above zero and there is no due date (zero and negative credit days are not terms). A second reading, age counted from the document date, is shown beside the due-date buckets for every document that still has a balance, in buckets 0-30, 31-60, 61-90, 91-180, 181-365 and over 365 days, with the amount over one year as a headline figure; it exists because the pilot shop records neither due dates nor credit days (34,710 of 34,712 sale documents and all 5,293 customers have credit day 0), so the due-date buckets alone leave about 95% of the balance in "not specified". Buckets by days past the due date: not yet due, 1 to 30, 31 to 60, 61 to 90, 91 to 120, over 120, plus "ไม่ระบุวันครบกำหนด" for documents without a due date (never aged from the document date) and "เครดิตคงค้าง" for negative balances. The grand total equals the receivable movement balance at the same date. A payment is applied to the customer of the document it was billed against, so a single customer can differ between the two reports when a receipt was booked under another customer; the totals still agree. | Receivable aging report |
| ถึงจุดสั่งซื้อ | Item master balance below the largest `purchase_point` of the item, for item types other than 5. | Reorder report |

Stock on hand by movement and the item master balance are different numbers. Name them differently in answers, and do not mix them in one sentence. They can disagree for individual items, and the value by movement can be negative when issues are recorded without matching receipts, so the figure must be shown with its basis.

## Customer reports (built)

| Term | Rule to follow |
|---|---|
| RFM (`customer_rfm`) | Per customer who has at least one code 44 sale in the period (customers with no code on the document are left out). Recency is days from the last code 44 sale to the last day of the period, frequency is the number of code 44 sales, monetary is net sales including VAT (44 + 46 - 48). Documents are counted as the sales report counts them: `last_status = 0`, `is_doc_copy` not 1, and POS bills that carry a `doc_ref` dropped. Scores run 1 to 5 and **5 is always the best**: shorter recency, more sales and more money all score higher. A score is the customer's percentile position among the shop's customers in the same period (`1 + floor(5 * percent_rank)`, capped at 5), so customers with equal values always get equal scores; NTILE is not used because it splits ties arbitrarily. Because it is a relative ranking, a score of 4 or 5 on frequency can mean only two purchases when most customers bought once, so the raw recency, frequency and amount are shown beside the scores. Segments (R, F): ลูกค้าดีเด่น R 4-5 and F 4-5; ลูกค้าประจำ R 3-5 and F 3-5 (not the previous one); ลูกค้าใหม่/เพิ่งกลับมา R 4-5 and F 1-2; เสี่ยงหาย R 1-2 and F 3-5; เงียบหาย R 1-2 and F 1-2; ต้องดูแล R 3 and F 1-2. A customer who only has returns does not appear. Not chunked: a percentile needs every customer at once and the result is one row per customer. |
| ความถี่การซื้อ (`purchase_frequency`) | Per customer who has at least one code 44 sale in the period. A purchase day is a date with at least one code 44 bill (several bills on one day count once, a change from the first draft of this rule that counted documents, because one delivery round is not a faster rhythm). Rhythm = (last purchase day - first purchase day) / (purchase days - 1), in days. Days since last purchase are measured to the last day of the period. Status: more than twice the rhythm and at least 7 days is เงียบเกินรอบมาก (OVERDUE); more than 1.5 times and at least 7 days is ช้ากว่ารอบ (LATE); otherwise ซื้อตามรอบ (ON_TRACK). A customer with one purchase day has no rhythm and is listed as ซื้อครั้งเดียว (SINGLE), so every customer who bought is accounted for. The average rhythm shown for the shop is total span over total gaps, not an average of averages. Amounts are the sum of code 44 `total_amount`, before returns, and rank the quiet customers by what they used to buy. Documents are counted as the sales report counts them. Not chunked, same reason as RFM. Read it with care on a shop whose customers buy by project: a customer whose project has ended is correctly flagged as quiet but has not necessarily left. |

## Where V2 and smlmcpconnect still differ

| Topic | V2 | smlmcpconnect | Rule |
|---|---|---|---|
| Profit | Codes 44 and 46 minus 48, before VAT | Code 44 only | V2 |
| Sales report POS rule | Drops POS bills that have a `doc_ref` | No rule | V2, and check on any shop that uses POS |
| Aging | `ar_aging`, built | Uses `due_date` | V2, with a "no due date" bucket |
| RFM | `customer_rfm`, built | Inverted scores | V2, 5 is the best |
| Stock | Movement based | Not compared | Name the basis in every answer |

## Decisions

Settled by the owner on 2026-10-01:

- `vat_type` 3 means no effect on tax. `inquiry_type` 4 does not exist. Only `last_status` 0 and 1 matter.
- A document with no `due_date` is shown as "no due date", as the data says.
- "Sales" is the net figure above (codes 44 and 46 minus 48), with "including VAT" and "before VAT" named separately. The live sales report still counts code 44 only; changing it changes the card the shop sees by a small amount, so it is applied when the first new reports are built and announced to the owner beforehand.
