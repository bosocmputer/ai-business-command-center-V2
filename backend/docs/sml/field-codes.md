---
status: current
last_verified: 2026-10-01
source_of_truth: [definitions given by the project owner, SML production data of the pilot shop]
tags: [sml, reference, dictionary]
---

# SML field codes used by reports

Definitions come from the project owner. Counts are from the pilot shop over 180 days and are only an illustration of how a shop uses a code; other shops differ.

| Table | Field | Value | Meaning |
|---|---|---|---|
| `ic_trans`, `ic_trans_detail` | `vat_type` | 0 | VAT added on top (ภาษีแยกนอก) |
| | | 1 | VAT included (ภาษีรวมใน) |
| | | 2 | VAT rate 0% (อัตราภาษี 0%), the tax base is reported at 0% |
| | | 3 | Not defined yet. Treat as an exception and warn |
| `ic_trans`, `ic_trans_detail` | `inquiry_type` | 0 | Credit sale (ขายเงินเชื่อ) |
| | | 1 | Cash sale (ขายเงินสด) |
| | | 2 | Credit sale, goods and services (ขายเงินเชื่อ สินค้าบริการ) |
| | | 3 | Cash sale, goods and services (ขายเงินสด สินค้าบริการ) |
| `ic_trans` | `last_status` | 0 | Normal document |
| | | 1 | Cancelled document |
| `ic_trans` | `is_pos` | 0 | Not a POS sale |
| | | 1 | POS sale |

Rules for reports:

- Count `last_status = 0` documents only. A cancellation also creates a code 45 document (see [trans-flag.md](trans-flag.md)).
- `ic_trans.is_cancel` is not used to mark cancellation by the pilot shop (0 on every document). Do not rely on it.
- Do not infer cash or credit sales from `credit_day` or `pay_amount`. Use `inquiry_type`.
- Do not infer VAT treatment from whether `total_vat_value` is zero. Use `vat_type`.
