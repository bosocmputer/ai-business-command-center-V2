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
