from __future__ import annotations

import json
import pathlib
import subprocess
import tempfile
import unittest

import context_impact


class ContextImpactGateTest(unittest.TestCase):
    def setUp(self) -> None:
        self.tempdir = tempfile.TemporaryDirectory()
        self.root = pathlib.Path(self.tempdir.name)
        self.run_command("git", "init", "-q")
        self.run_command("git", "config", "user.email", "context@example.invalid")
        self.run_command("git", "config", "user.name", "Context Test")
        self.write("src/router.ts", "export const routes = []\n")
        self.write("src/api/client.ts", "export const client = {}\n")
        self.write("src/router.test.ts", "test('router', () => {})\n")
        self.write("docs/knowledge/routes.md", "# Routes\n")
        self.write("docs/knowledge/requests.md", "# Requests\n")
        self.write(
            "docs/knowledge/context-map.json",
            json.dumps(
                {
                    "version": 1,
                    "rules": [
                        {
                            "id": "viewer-routing",
                            "sourcePatterns": ["src/router.ts"],
                            "excludePatterns": ["**/*.test.*"],
                            "contextDocuments": ["docs/knowledge/routes.md"],
                            "fallback": False,
                        },
                        {
                            "id": "request-lifecycle",
                            "sourcePatterns": ["src/api/client.ts"],
                            "excludePatterns": ["**/*.test.*"],
                            "contextDocuments": ["docs/knowledge/requests.md"],
                            "fallback": False,
                        },
                        {
                            "id": "frontend-source",
                            "sourcePatterns": ["src/**"],
                            "excludePatterns": ["**/*.test.*"],
                            "contextDocuments": ["docs/knowledge/*.md"],
                            "fallback": True,
                        },
                    ],
                },
                indent=2,
            )
            + "\n",
        )
        self.run_command("git", "add", ".")
        self.run_command("git", "commit", "-qm", "base")
        self.base = self.rev("HEAD")

    def tearDown(self) -> None:
        self.tempdir.cleanup()

    def run_command(self, *args: str) -> subprocess.CompletedProcess[str]:
        return subprocess.run(args, cwd=self.root, check=True, text=True, capture_output=True)

    def rev(self, ref: str) -> str:
        return self.run_command("git", "rev-parse", ref).stdout.strip()

    def write(self, path: str, content: str) -> None:
        target = self.root / path
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_text(content, encoding="utf-8")

    def commit(self, message: str) -> str:
        self.run_command("git", "add", ".")
        self.run_command("git", "commit", "-qm", message)
        return self.rev("HEAD")

    def gate(self, head: str, body: str = "") -> context_impact.GateResult:
        return context_impact.evaluate(
            root=self.root,
            map_path=self.root / "docs/knowledge/context-map.json",
            base=self.base,
            head=head,
            pr_body=body,
        )

    def test_source_change_with_mapped_document_passes(self) -> None:
        self.write("src/router.ts", "export const routes = ['/app']\n")
        self.write("docs/knowledge/routes.md", "# Routes\n\n- /app\n")
        result = self.gate(self.commit("update route and note"))
        self.assertEqual(result.impacted_rule_ids, ("viewer-routing",))

    def test_valid_pr_acknowledgement_passes_without_document_change(self) -> None:
        self.write("src/router.ts", "export const routes = ['/app']\n")
        head = self.commit("refactor route")
        result = self.gate(
            head,
            "Context-Reviewed: viewer-routing\n"
            "Context-Reason: Internal refactor only; route contracts and behavior are unchanged.",
        )
        self.assertEqual(result.acknowledged_rule_ids, ("viewer-routing",))

    def test_missing_document_and_acknowledgement_fails(self) -> None:
        self.write("src/router.ts", "export const routes = ['/app']\n")
        with self.assertRaisesRegex(context_impact.ImpactError, "viewer-routing"):
            self.gate(self.commit("unreviewed route"))

    def test_test_only_change_does_not_trigger(self) -> None:
        self.write("src/router.test.ts", "test('router changed', () => {})\n")
        result = self.gate(self.commit("test only"))
        self.assertEqual(result.impacted_rule_ids, ())

    def test_docs_only_change_does_not_trigger(self) -> None:
        self.write("docs/knowledge/routes.md", "# Routes\n\nEditorial clarification.\n")
        result = self.gate(self.commit("docs only"))
        self.assertEqual(result.impacted_rule_ids, ())

    def test_new_source_file_uses_fallback_rule(self) -> None:
        self.write("src/new-feature.ts", "export const feature = true\n")
        head = self.commit("new source")
        result = self.gate(
            head,
            "Context-Reviewed: frontend-source\n"
            "Context-Reason: New internal source was reviewed and does not alter documented behavior.",
        )
        self.assertEqual(result.impacted_rule_ids, ("frontend-source",))

    def test_every_impacted_rule_must_be_satisfied(self) -> None:
        self.write("src/router.ts", "export const routes = ['/app']\n")
        self.write("src/api/client.ts", "export const client = { timeout: 1 }\n")
        head = self.commit("two subsystems")
        with self.assertRaisesRegex(context_impact.ImpactError, "request-lifecycle"):
            self.gate(
                head,
                "Context-Reviewed: viewer-routing\n"
                "Context-Reason: Route behavior was reviewed and remains unchanged after refactoring.",
            )

    def test_unknown_acknowledgement_id_fails_closed(self) -> None:
        self.write("src/router.ts", "export const routes = ['/app']\n")
        head = self.commit("route refactor")
        with self.assertRaisesRegex(context_impact.ImpactError, "unknown"):
            self.gate(
                head,
                "Context-Reviewed: viewer-routing, unknown-rule\n"
                "Context-Reason: Internal refactor only; route contracts and behavior are unchanged.",
            )

    def test_short_acknowledgement_reason_fails_closed(self) -> None:
        self.write("src/router.ts", "export const routes = ['/app']\n")
        head = self.commit("route refactor")
        with self.assertRaisesRegex(context_impact.ImpactError, "20 characters"):
            self.gate(head, "Context-Reviewed: viewer-routing\nContext-Reason: no change")

    def test_invalid_base_reference_fails_closed(self) -> None:
        with self.assertRaisesRegex(context_impact.ImpactError, "base"):
            context_impact.evaluate(
                root=self.root,
                map_path=self.root / "docs/knowledge/context-map.json",
                base="not-a-commit",
                head=self.base,
                pr_body="",
            )

    def test_unsafe_map_path_fails_validation(self) -> None:
        map_path = self.root / "docs/knowledge/context-map.json"
        payload = json.loads(map_path.read_text(encoding="utf-8"))
        payload["rules"][0]["sourcePatterns"] = ["../outside/**"]
        map_path.write_text(json.dumps(payload), encoding="utf-8")
        with self.assertRaisesRegex(context_impact.ImpactError, "unsafe"):
            context_impact.validate_repository(root=self.root, map_path=map_path)

    def test_missing_generated_marker_fails_validation(self) -> None:
        with self.assertRaisesRegex(context_impact.ImpactError, "markers"):
            context_impact.validate_repository(
                root=self.root,
                map_path=self.root / "docs/knowledge/context-map.json",
                marker_pairs=(("docs/knowledge/routes.md", "<!-- START -->", "<!-- END -->"),),
            )

    def test_sensitive_customer_context_is_detected(self) -> None:
        samples = {
            "UUID": "123e4567-e89b-42d3-a456-426614174000",
            "token-like value": "sk-abcdefghijklmnopqrstuvwxyz123456",
            "customer IP address": "10.20.30.40:8092",
            "customer dynamic endpoint": "http://customer.thddns.com:8080",
        }
        for expected, sample in samples.items():
            with self.subTest(expected=expected):
                self.assertIn(expected, context_impact.sensitive_context_labels(sample))


if __name__ == "__main__":
    unittest.main()
