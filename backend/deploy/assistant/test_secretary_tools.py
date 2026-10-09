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
    def test_not_enabled_without_a_key(self):
        with self.assertRaises(tools.ToolError):
            tools.web_search("ราคาเหล็ก", key="")

    def test_refuses_queries_that_carry_shop_data(self):
        for query in ("ยอดค้าง 12345678 บาท", "ติดต่อ somchai@example.com", "ลูกค้า-ab12cd ค้างชำระ", "x" * 300, "   "):
            with self.assertRaises(tools.ToolError, msg=query):
                tools.web_search(query, key="k", fetch=lambda request: {})

    def test_brave_and_tavily_results_are_cleaned_and_capped(self):
        brave = {"web": {"results": [{"title": "<b>ราคาเหล็ก</b>", "url": "https://example.com/a", "description": "x" * 900}]}}
        out = tools.web_search("ราคาเหล็กเส้นวันนี้", provider="brave", key="k", fetch=lambda request: brave)
        self.assertEqual(out["results"][0]["title"], "ราคาเหล็ก")
        self.assertEqual(len(out["results"][0]["snippet"]), 400)
        tavily = {"results": [{"title": "t", "url": "https://example.com/b", "content": "c"}]}
        seen = {}

        def fetch(request):
            seen["auth"] = request.get_header("Authorization")
            return tavily
        out = tools.web_search("ภาษีมูลค่าเพิ่มอัตราปัจจุบัน", provider="tavily", key="secret", fetch=fetch)
        self.assertEqual(out["results"][0]["url"], "https://example.com/b")
        self.assertEqual(seen["auth"], "Bearer secret")
        self.assertNotIn("secret", str(out))

    def test_unknown_provider_is_refused(self):
        with self.assertRaises(tools.ToolError):
            tools.web_search("ราคา", provider="other", key="k", fetch=lambda request: {})


if __name__ == "__main__":
    unittest.main()
