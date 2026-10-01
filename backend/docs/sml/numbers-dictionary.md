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
- `last_status = 2` ("hold") appears in smlmcpconnect's labels. The owner has not confirmed it and the pilot shop has none, so it is excluded and not interpreted.
- Skip copies: `is_doc_copy = 0`.
- POS: code 44 is shared by the sales menu and the POS menu. V2's sales report drops a POS bill only when it also has a `doc_ref`. A shop that uses POS needs this rule checked.
- Cash or credit comes from `inquiry_type`, not from `credit_day` or `pay_amount`.
- VAT treatment comes from `vat_type` (0 added on top, 1 included, 2 rate 0%, 3 undefined and treated as an exception), not from whether `total_vat_value` is zero.
- Header `total_except_vat` was empty on every document of the pilot shop. Do not rely on it. Get amounts before VAT from line `sum_amount_exclude_vat`.

## Terms

| Term (Thai) | Definition | Where it is used |
|---|---|---|
| ยอดขาย (sales, including VAT) | Sum of header `total_amount` of code 44 documents. Includes cash and credit sales. Excludes code 46 (debit notes) and 48 (returns and credit notes). | Sales report. smlmcpconnect sales tools use the same rule. |
| ยอดขายก่อน VAT | Sum of line `sum_amount_exclude_vat` of code 44 documents. | Gross profit reports |
| VAT ขาย | Sum of header `total_vat_value` of code 44 documents. Zero for `vat_type` 2. | Sales report |
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
| ถึงจุดสั่งซื้อ | Item master balance below the largest `purchase_point` of the item, for item types other than 5. | Reorder report |

Stock on hand by movement and the item master balance are different numbers. Name them differently in answers, and do not mix them in one sentence. They can disagree for individual items, and the value by movement can be negative when issues are recorded without matching receipts, so the figure must be shown with its basis.

## Terms still to be built

| Term | Rule to follow |
|---|---|
| ลูกหนี้ตามอายุหนี้ (receivables aging) | smlmcpconnect's rule: per document, balance is `total_amount` minus payments from `ap_ar_trans_detail` (code 239, matched by billing number and date); buckets by days past `due_date` (due today, 1 to 30, 31 to 60, 61 to 90, 91 to 120, over 120). It needs `due_date`, which the pilot shop leaves empty on most open documents. Offer age counted from the document date as the fallback and say which was used. |
| RFM (recency, frequency, monetary) | Do not copy smlmcpconnect's SQL. It orders recency ascending and frequency and amount descending when scoring with NTILE, so score 1 is the best customer, but its segment rules treat 5 as the best. The best customers end up labelled "Hibernating". Score so that 5 is the best: recency ordered descending (shorter is better), frequency and monetary ordered ascending. Count code 44 documents with `last_status = 0` and `is_doc_copy = 0`, amount from `total_amount`. |
| ความถี่การซื้อ | Average days between a customer's code 44 documents in the window: `(last date - first date) / (documents - 1)`, for customers with at least two documents. |

## Where V2 and smlmcpconnect still differ

| Topic | V2 | smlmcpconnect | Rule |
|---|---|---|---|
| Profit | Codes 44 and 46 minus 48, before VAT | Code 44 only | V2 |
| Sales report POS rule | Drops POS bills that have a `doc_ref` | No rule | V2, and check on any shop that uses POS |
| Aging | Not built | Uses `due_date` | Build with the fallback above |
| RFM | Not built | Inverted scores | Rewrite |
| Stock | Movement based | Not compared | Name the basis in every answer |

## Questions for the owner

1. Should "ยอดขาย" in the assistant's answers include debit notes (46) and subtract returns (48), or stay as code 44 only as the sales report does? The profit reports already include them, so two reports can disagree on "sales".
2. What does `vat_type` 3 mean, and `inquiry_type` 4 (it appears in receivable SQL)?
3. Is `last_status` 2 a real value in SML ("hold")?
4. For receivables aging, which fallback does the shop prefer when `due_date` is empty?
