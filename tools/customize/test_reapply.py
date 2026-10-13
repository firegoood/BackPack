"""Exercise upstream reapplication without changing the user's checkout."""
import subprocess
import contextlib
import io
from pathlib import Path
import tempfile
import unittest
from unittest import mock

import reapply


class ReapplicationTests(unittest.TestCase):
    def upstream(self, path):
        return subprocess.check_output(["git", "show", "HEAD:" + path], cwd=reapply.ROOT).decode("utf-8")

    def test_reapplication_is_idempotent_on_upstream_and_customized_surfaces(self):
        for path in reapply.SPEC["required"]:
            target = reapply.ROOT / path
            with self.subTest(path=path):
                current = target.read_text(encoding="utf-8")
                updated = reapply.transform(path, current)
                self.assertEqual(updated, reapply.transform(path, updated))
                exists = subprocess.run(["git", "cat-file", "-e", "HEAD:" + path],
                                        cwd=reapply.ROOT, capture_output=True).returncode == 0
                if exists:
                    upstream = reapply.transform(path, self.upstream(path))
                    self.assertEqual(upstream, reapply.transform(path, upstream))

    def test_managed_https_default_username_stays_compatible(self):
        path = "internal/manage/managedwebapi.go"
        original = (reapply.ROOT / path).read_text(encoding="utf-8")
        self.assertIn('defaultString(m.Username, "backpack")', reapply.transform(path, original))

    def test_web_transport_parity_has_update_guards(self):
        for path in ["internal/manage/managedwebapi.go", "internal/manage/managedwebapi_test.go",
                     "internal/manage/webapi.go", "internal/manage/directwebapi.go",
                     "internal/webui/panel/views/add.html", "internal/webui/panel/views/edit.html",
                     "internal/webui/panel/js/ui/managed.js"]:
            self.assertIn(path, reapply.SPEC["required"])

    def test_check_refuses_discarded_feature_integration(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            spec = {**reapply.SPEC, "required": {"integration.go": ["applyManagedHTTPS"]}}
            for body in [None, "upstream removed the integration"]:
                with self.subTest(body=body):
                    if body is not None:
                        (root / "integration.go").write_text(body, encoding="utf-8")
                    output = io.StringIO()
                    with mock.patch.object(reapply, "ROOT", root), \
                         mock.patch.object(reapply, "SPEC", spec), \
                         mock.patch.object(reapply, "files", return_value=iter([])), \
                         mock.patch("sys.argv", ["reapply.py", "--check"]), \
                         contextlib.redirect_stdout(output):
                        self.assertEqual(1, reapply.main())
                    self.assertIn("integration.go", output.getvalue())

    def test_upstream_transport_crypto_and_signatures_keep_their_identity(self):
        paths = ["internal/utils/network/noise.go", "internal/utils/network/quicbind.go",
                 "internal/utils/network/wssbind.go", "internal/utils/network/kcp.go",
                 "internal/tunnel/l3/session.go", "internal/tunnel/l3/carrier_quic.go",
                 "internal/tunnel/direct/handshake.go", "internal/app/releasemsg.go",
                 "internal/manage/sharelink.go", "internal/app/app.go"]
        for path in paths:
            original = self.upstream(path)
            updated = reapply.transform(path, original)
            for literal in reapply.SPEC["preserve_literals"]:
                if literal in original:
                    with self.subTest(path=path, literal=literal):
                        self.assertEqual(original.count(literal), updated.count(literal))

    def test_release_install_and_update_use_the_fork_and_fullpack_assets(self):
        app = reapply.transform("internal/app/app.go", self.upstream("internal/app/app.go"))
        install = reapply.transform("install.sh", self.upstream("install.sh"))
        self.assertIn('RepoOwner = "firegoood"', app)
        self.assertIn('RepoName  = "BackPack"', app)
        self.assertIn('REPO="firegoood/BackPack"', install)
        self.assertIn('ASSET="fullpack_linux_${ARCH}.tar.gz"', install)
        extract = reapply.transform("internal/manage/update.go", self.upstream("internal/manage/update.go"))
        self.assertIn('filepath.Base(hdr.Name) != "fullpack"', extract)
        self.assertIn('"backpack", cfg.Server.Token', extract)

    def test_readme_preserves_the_authors_attribution_and_trademark_policy(self):
        for path in ["README.md", "README_FA.md"]:
            updated = reapply.transform(path, self.upstream(path))
            self.assertIn("Based on BackPack by Amin Mohammadi (AminMGMT)", updated)
            self.assertIn("https://github.com/AminMGMT/BackPack", updated)
            self.assertIn("TRADEMARK.md", updated)
            self.assertIn("FullPack", updated)

    def test_ci_and_release_refuse_naming_drift(self):
        for path in [".github/workflows/ci.yml", ".github/workflows/release.yml"]:
            updated = reapply.transform(path, self.upstream(path))
            self.assertEqual(1, updated.count("- name: Verify FullPack customizations"))
            self.assertIn("python3 tools/customize/reapply.py --check", updated)


if __name__ == "__main__":
    unittest.main()
