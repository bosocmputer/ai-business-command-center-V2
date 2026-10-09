import io
import os
import tempfile
import unittest
import zipfile

import secretary_tools as tools


def make_docx(path, paragraphs):
    body = "".join(f"<w:p><w:r><w:t>{text}</w:t></w:r></w:p>" for text in paragraphs)
    with zipfile.ZipFile(path, "w") as archive:
        archive.writestr("word/document.xml", f'<w:document xmlns:w="x"><w:body>{body}</w:body></w:document>')


class MakeFileTests(unittest.TestCase):
    def setUp(self):
        self.box = tempfile.mkdtemp()

    def test_csv_has_a_byte_order_mark_and_thai_text(self):
        result = tools.make_file("csv", "ลูกหนี้.csv", "ชื่อ,ยอด\nก,100.50", outbox=self.box)
        raw = open(result["path"], "rb").read()
        self.assertTrue(raw.startswith(b"\xef\xbb\xbf"))
        self.assertIn("ยอด".encode("utf-8"), raw)
        self.assertTrue(result["path"].startswith(self.box))
        self.assertEqual(result["name"], "ลูกหนี้.csv")

    def test_xlsx_round_trips_through_the_reader(self):
        result = tools.make_file("xlsx", "ยอดขาย", '[["วัน","ยอด"],["1 ต.ค.",1500.5],["2 ต.ค.",20]]', outbox=self.box)
        self.assertTrue(result["name"].endswith(".xlsx"))
        with zipfile.ZipFile(result["path"]) as archive:
            text = tools.xlsx_text(archive)
        self.assertIn("วัน\tยอด", text)
        self.assertIn("1 ต.ค.\t1500.5", text)

    def test_names_cannot_escape_the_outbox(self):
        for name in ("../../etc/passwd", "/etc/cron.d/x", "..\\..\\x", ".hidden", ""):
            result = tools.make_file("txt", name, "x", outbox=self.box)
            self.assertTrue(os.path.realpath(result["path"]).startswith(os.path.realpath(self.box) + os.sep), name)
            self.assertFalse(os.path.basename(result["path"]).startswith("."))

    def test_refusals(self):
        with self.assertRaises(tools.ToolError):
            tools.make_file("exe", "a", "x", outbox=self.box)
        with self.assertRaises(tools.ToolError):
            tools.make_file("txt", "a", "   ", outbox=self.box)
        with self.assertRaises(tools.ToolError):
            tools.make_file("html", "a", "<p>hi</p><script>alert(1)</script>", outbox=self.box)
        with self.assertRaises(tools.ToolError):
            tools.make_file("html", "a", '<a onclick="x()">y</a>', outbox=self.box)
        with self.assertRaises(tools.ToolError):
            tools.make_file("txt", "a", "x" * (tools.MAX_FILE_BYTES + 1), outbox=self.box)

    def test_html_is_wrapped_when_it_is_only_a_fragment(self):
        result = tools.make_file("html", "ใบเสนอราคา", "<h1>ใบเสนอราคา</h1>", outbox=self.box)
        self.assertIn("<!doctype html>", open(result["path"], encoding="utf-8").read())

    def test_purge_removes_only_old_files(self):
        old = tools.make_file("txt", "old", "x", outbox=self.box)
        new = tools.make_file("txt", "new", "x", outbox=self.box)
        os.utime(old["path"], (1, 1))
        self.assertEqual(tools.purge_outbox(self.box), 1)
        self.assertFalse(os.path.exists(old["path"]))
        self.assertTrue(os.path.exists(new["path"]))


class ReadDocumentTests(unittest.TestCase):
    def setUp(self):
        self.root = tempfile.mkdtemp()
        self.outside = tempfile.mkdtemp()

    def test_reads_docx_and_xlsx_inside_the_attachment_folder(self):
        docx = os.path.join(self.root, "a.docx")
        make_docx(docx, ["ใบเสนอราคา", "รวม &amp; ภาษี"])
        self.assertIn("รวม & ภาษี", tools.read_document(docx, [self.root])["text"])
        made = tools.make_file("xlsx", "b", "ก,ข\n1,2", outbox=self.root)
        self.assertIn("ก\tข", tools.read_document(made["path"], [self.root])["text"])

    def test_refuses_outside_paths_symlinks_and_other_types(self):
        secret = os.path.join(self.outside, "s.docx")
        make_docx(secret, ["secret"])
        with self.assertRaises(tools.ToolError):
            tools.read_document(secret, [self.root])
        link = os.path.join(self.root, "link.docx")
        os.symlink(secret, link)
        with self.assertRaises(tools.ToolError):
            tools.read_document(link, [self.root])
        for name in ("config.yaml", "x.pdf"):
            path = os.path.join(self.root, name)
            open(path, "w").write("x")
            with self.assertRaises(tools.ToolError):
                tools.read_document(path, [self.root])
        with self.assertRaises(tools.ToolError):
            tools.read_document(os.path.join(self.root, "..", os.path.basename(self.outside), "s.docx"), [self.root])

    def test_broken_file_is_reported_not_raised_as_a_crash(self):
        path = os.path.join(self.root, "broken.xlsx")
        open(path, "wb").write(b"not a zip")
        with self.assertRaises(tools.ToolError):
            tools.read_document(path, [self.root])


class WebSearchTests(unittest.TestCase):
    def setUp(self):
        self.count_file = os.path.join(tempfile.mkdtemp(), "count.json")

    def search(self, query="ราคาเหล็กเส้นวันนี้", **kwargs):
        kwargs.setdefault("count_file", self.count_file)
        return tools.web_search(query, **kwargs)

    def test_not_enabled_without_a_key(self):
        with self.assertRaises(tools.ToolError):
            self.search(chain=[("serper", ""), ("serpapi", "")])

    def test_refuses_queries_that_carry_shop_data_before_anything_is_sent(self):
        sent = []
        for query in ("ยอดค้าง 12345678 บาท", "ติดต่อ somchai@example.com", "ลูกค้า-ab12cd ค้างชำระ", "x" * 300, "   "):
            with self.assertRaises(tools.ToolError, msg=query):
                self.search(query, chain=[("serper", "k")], fetch=lambda request: sent.append(request) or {})
        self.assertEqual(sent, [])

    def test_serper_results_are_cleaned_and_capped_and_the_key_stays_in_the_header(self):
        seen = {}

        def fetch(request):
            seen["url"], seen["key"] = request.full_url, request.get_header("X-api-key")
            return {"organic": [{"title": "<b>ราคาเหล็ก</b>", "link": "https://example.com/a", "snippet": "x" * 900}]}
        out = self.search(chain=[("serper", "secret-1")], fetch=fetch)
        self.assertEqual(out["results"][0]["title"], "ราคาเหล็ก")
        self.assertEqual(len(out["results"][0]["snippet"]), 400)
        self.assertEqual(seen["key"], "secret-1")
        self.assertNotIn("secret-1", seen["url"] + str(out))

    def test_serpapi_is_the_fallback_when_serper_fails(self):
        calls = []

        def fetch(request):
            calls.append(request.full_url.split("?")[0])
            if "serper.dev" in request.full_url:
                raise tools.ToolError("บริการค้นเว็บตอบกลับผิดปกติ (HTTP 429)")
            return {"organic_results": [{"title": "t", "link": "https://example.com/b", "snippet": "c"}]}
        out = self.search(chain=[("serper", "k1"), ("serpapi", "k2")], fetch=fetch)
        self.assertEqual(out["results"][0]["url"], "https://example.com/b")
        self.assertEqual(calls, ["https://google.serper.dev/search", "https://serpapi.com/search.json"])
        self.assertNotIn("k2", str(out))

    def test_a_failure_of_every_service_is_reported_without_the_key(self):
        def fetch(request):
            raise tools.ToolError("ติดต่อบริการค้นเว็บไม่ได้ในขณะนี้")
        with self.assertRaises(tools.ToolError) as context:
            self.search(chain=[("serper", "k1"), ("serpapi", "k2")], fetch=fetch)
        self.assertNotIn("k1", str(context.exception))

    def test_daily_cap_stops_a_loop(self):
        fetch = lambda request: {"organic": [{"title": "t", "link": "https://example.com", "snippet": "s"}]}
        for _ in range(3):
            self.search(chain=[("serper", "k")], fetch=fetch, cap=3)
        with self.assertRaises(tools.ToolError):
            self.search(chain=[("serper", "k")], fetch=fetch, cap=3)

    def test_unknown_provider_is_refused(self):
        with self.assertRaises(tools.ToolError):
            self.search(chain=[("other", "k")], fetch=lambda request: {})


if __name__ == "__main__":
    unittest.main()


class TableSumsTests(unittest.TestCase):
    def test_xlsx_read_returns_column_sums_computed_by_the_tool(self):
        root = tempfile.mkdtemp()
        made = tools.make_file("xlsx", "t", '[["รหัสบัญชี","ชื่อ","เดบิต","เครดิต"],["110100","เงินสด",1000.5,0],["110200","ธนาคาร",2000.25,10],["210100","เจ้าหนี้",0,500.75]]', outbox=root)
        result = tools.read_document(made["path"], [root])
        sums = {item["column"]: item["sum"] for item in result["sheets"][0]["column_sums"]}
        self.assertEqual(sums, {"เดบิต": "3000.75", "เครดิต": "510.75"})  # the code column is not summed
        self.assertEqual(result["sheets"][0]["rows"], 4)
        self.assertIn("ห้ามบวกเลข", result["note"])

    def test_make_file_gives_the_line_to_put_in_the_reply(self):
        box = tempfile.mkdtemp()
        result = tools.make_file("txt", "a", "x", outbox=box)
        self.assertEqual(result["reply_line"], "MEDIA:" + result["path"])


COLUMNS = [
    {"key": "doc_date", "label": "วันที่", "type": "date"}, {"key": "doc_no", "label": "เลขที่เอกสาร", "type": "text"},
    {"key": "cust_name", "label": "ลูกค้า", "type": "text"}, {"key": "qty", "label": "จำนวน", "type": "number"},
    {"key": "sum_amount", "label": "มูลค่ารายการ", "type": "number", "total": True}, {"key": "empty", "label": "ว่าง", "type": "text"},
]
ROWS = [
    ["2026-09-01", "IV-0001", "ลูกค้า-AB12", "2.0000", "250.50", ""],
    ["2026-09-02", "IV-0002", "ลูกค้า-CD34", "10", "1,000.25", ""],
    ["2026-09-30", "IV-0003", "ลูกค้า-AB12", "-1", "-100", ""],
]


class ExportFileTests(unittest.TestCase):
    def setUp(self):
        self.box = tempfile.mkdtemp()

    def make(self, kind="xlsx", rows=None, **kwargs):
        return tools.export_file(kind, kwargs.pop("name", "ขายกันยายน"), "รายงานขาย", "2026-09-01 ถึง 2026-09-30", "2026-10-01T12:00:00+07:00",
                                 COLUMNS, ROWS if rows is None else rows, notes=["ยอดนี้ยังไม่รวม VAT"], outbox=self.box)

    def test_every_row_goes_in_with_thai_headings_and_an_unused_column_is_left_out(self):
        result = self.make()
        self.assertEqual(result["rows"], 3)
        self.assertNotIn("ว่าง", result["sheets"][0]["columns"])
        self.assertEqual(result["reply_line"], "MEDIA:" + result["path"])
        with zipfile.ZipFile(result["path"]) as archive:
            sheets = tools.xlsx_sheets(archive)
        self.assertEqual([name for name, _ in sheets], ["sheet1.xml", "sheet2.xml"])
        data = sheets[0][1]
        self.assertEqual(data[0], ["วันที่", "เลขที่เอกสาร", "ลูกค้า", "จำนวน", "มูลค่ารายการ"])
        self.assertEqual(len(data), 4)
        self.assertEqual(data[2][1], "IV-0002")
        self.assertEqual(data[2][4], "1000.25")  # a number with a thousands comma becomes a number

    def test_numbers_are_numbers_and_dates_are_dates_in_the_sheet(self):
        result = self.make()
        with zipfile.ZipFile(result["path"]) as archive:
            xml = archive.read("xl/worksheets/sheet1.xml").decode("utf-8")
        self.assertIn('<c r="A2" s="4"><v>46266</v></c>', xml)  # 2026-09-01
        self.assertIn('<c r="D2" s="2"><v>2</v></c>', xml)  # a column of whole numbers is shown without decimals
        self.assertIn('<c r="E2" s="3"><v>250.5</v></c>', xml)
        self.assertIn('<pane ySplit="1"', xml)
        self.assertIn("<autoFilter", xml)

    def test_totals_are_worked_out_by_the_tool_for_money_columns_only(self):
        sums = {item["column"]: item["sum"] for item in self.make()["column_sums"]}
        self.assertEqual(sums, {"มูลค่ารายการ": "1150.75"})  # a quantity (units differ) is not added

    def test_a_report_with_documents_and_lines_gets_two_sheets_each_with_its_own_columns(self):
        columns = [
            {"key": "doc_no", "label": "เลขที่เอกสาร", "type": "text"}, {"key": "item_code", "label": "รหัสสินค้า", "type": "text"},
            {"key": "sum_amount", "label": "มูลค่ารายการ", "type": "number", "total": True}, {"key": "total_amount", "label": "ยอดขาย", "type": "number", "total": True},
        ]
        rows = [["IV-1", "", "", "300"], ["IV-1", "A01", "100", ""], ["IV-1", "A02", "200", ""], ["IV-2", "", "", "-50"]]
        result = tools.export_file("xlsx", "x", "รายงานขาย", "p", "t", columns, rows, split=("item_code", "เอกสาร", "รายการสินค้า"), outbox=self.box)
        self.assertEqual([(sheet["name"], sheet["rows"]) for sheet in result["sheets"]], [("เอกสาร", 2), ("รายการสินค้า", 2)])
        self.assertEqual(result["sheets"][0]["columns"], ["เลขที่เอกสาร", "ยอดขาย"])
        self.assertEqual(result["sheets"][1]["columns"], ["เลขที่เอกสาร", "รหัสสินค้า", "มูลค่ารายการ"])
        sums = {(item["sheet"], item["column"]): item["sum"] for item in result["column_sums"]}
        self.assertEqual(sums, {("เอกสาร", "ยอดขาย"): "250.00", ("รายการสินค้า", "มูลค่ารายการ"): "300.00"})
        with zipfile.ZipFile(result["path"]) as archive:
            self.assertEqual(len(tools.xlsx_sheets(archive)), 3)

    def test_the_explanation_sheet_says_what_the_file_is(self):
        result = self.make()
        with zipfile.ZipFile(result["path"]) as archive:
            about = tools.xlsx_sheets(archive)[1][1]
        text = "\n".join("\t".join(row) for row in about)
        for expected in ("รายงานขาย", "2026-09-01 ถึง 2026-09-30", "2026-10-01T12:00:00+07:00", "ยอดนี้ยังไม่รวม VAT", "1150.75", "อ่านอย่างเดียว"):
            self.assertIn(expected, text)

    def test_csv_has_a_byte_order_mark_and_plain_numbers(self):
        raw = open(self.make("csv")["path"], "rb").read().decode("utf-8")
        self.assertTrue(raw.startswith("\ufeff"))
        self.assertIn("วันที่,เลขที่เอกสาร,ลูกค้า,จำนวน,มูลค่ารายการ", raw)
        self.assertIn("2026-09-02,IV-0002,ลูกค้า-CD34,10,1000.25", raw)

    def test_refusals(self):
        with self.assertRaises(tools.ToolError):
            self.make(rows=[])
        with self.assertRaises(tools.ToolError):
            self.make("txt")
        with self.assertRaises(tools.ToolError):
            self.make(rows=[ROWS[0]] * (tools.EXPORT_MAX_ROWS + 1))

    def test_the_file_name_cannot_escape_the_outbox(self):
        result = self.make(name="../../etc/x")
        self.assertTrue(os.path.realpath(result["path"]).startswith(os.path.realpath(self.box) + os.sep))

    def test_text_in_the_rows_cannot_break_the_workbook(self):
        rows = [["2026-09-01", "<x>&\"", "ลูกค้า", "1", "2", ""]]
        result = self.make(rows=rows)
        with zipfile.ZipFile(result["path"]) as archive:
            self.assertEqual(tools.xlsx_sheets(archive)[0][1][1][1], '<x>&"')


class CollectExportTests(unittest.TestCase):
    def pages(self, total, per_page, ready_after=0):
        calls = {"n": 0, "queries": []}

        def fetch(query):
            calls["queries"].append(dict(query))
            calls["n"] += 1
            if calls["n"] <= ready_after:
                return {"status": "PREPARING", "message": "กำลังดึง", "retryAfterSeconds": 45}
            start = int(query["cursor"]) if query.get("cursor") else 0
            end = min(start + per_page, total)
            page = {"status": "READY", "reportKey": "sales_goods_services", "label": "รายงานขาย", "columns": COLUMNS,
                    "rows": [["2026-09-01", f"IV-{i}", "x", "1", "1", ""] for i in range(start, end)], "totalRows": total}
            if end < total:
                page["nextCursor"] = str(end)
            return page
        return fetch, calls

    def test_follows_every_page_to_the_end(self):
        fetch, calls = self.pages(1234, 500)
        out = tools.collect_export(fetch, {"dateFrom": "2026-09-01"})
        self.assertEqual(out["status"], "READY")
        self.assertEqual(len(out["rows"]), 1234)
        self.assertEqual(out["rows"][-1][1], "IV-1233")
        self.assertEqual(calls["queries"][0], {"dateFrom": "2026-09-01"})
        self.assertEqual([q.get("cursor") for q in calls["queries"][1:]], ["500", "1000"])

    def test_waits_while_preparing_then_gives_the_rows(self):
        fetch, calls = self.pages(3, 500, ready_after=2)
        slept = []
        out = tools.collect_export(fetch, {}, wait_seconds=60, poll_seconds=5, sleep=slept.append, clock=lambda: 0 + len(slept))
        self.assertEqual((out["status"], len(out["rows"])), ("READY", 3))
        self.assertEqual(len(slept), 2)

    def test_gives_up_waiting_and_says_preparing_so_the_owner_can_ask_again(self):
        fetch, _ = self.pages(3, 500, ready_after=99)
        slept = []
        out = tools.collect_export(fetch, {}, wait_seconds=10, poll_seconds=5, sleep=slept.append, clock=lambda: len(slept) * 5)
        self.assertEqual(out["status"], "PREPARING")
        self.assertNotIn("rows", out)

    def test_a_refusal_or_failure_passes_through_untouched(self):
        out = tools.collect_export(lambda q: {"status": "NO_DATA", "message": "ไม่มีข้อมูลเรื่องนี้ให้ดู"}, {})
        self.assertEqual(out, {"status": "NO_DATA", "message": "ไม่มีข้อมูลเรื่องนี้ให้ดู"})

    def test_a_page_that_fails_midway_is_an_error_not_a_short_file(self):
        answers = iter([{"status": "READY", "rows": [["a"]], "nextCursor": "1"}, {"status": "UNAVAILABLE", "message": "หมดอายุ"}])
        with self.assertRaises(tools.ToolError):
            tools.collect_export(lambda q: next(answers), {})

    def test_stops_at_the_row_cap_and_says_truncated(self):
        fetch, _ = self.pages(tools.EXPORT_MAX_ROWS + 900, 500)
        out = tools.collect_export(fetch, {})
        self.assertEqual(len(out["rows"]), tools.EXPORT_MAX_ROWS)
        self.assertTrue(out["truncated"])


class FileNameTests(unittest.TestCase):
    def test_a_file_name_has_no_spaces_so_the_media_line_is_not_cut(self):
        self.assertEqual(tools.safe_name("รายงานขาย 2026-10-01 ถึง 2026-10-09", "xlsx"), "รายงานขาย_2026-10-01_ถึง_2026-10-09.xlsx")
