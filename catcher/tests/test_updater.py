"""Run with Python and POSIX sh (Git Bash on Windows); no Android side effects."""
import hashlib
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest


SCRIPT = Path(__file__).resolve().parents[1] / "scripts" / "catcher.sh"
BASH = shutil.which("bash") if os.name != "nt" else r"D:\Program Files\Git\bin\bash.exe"
OLD = b"#!/bin/sh\necho OLD\n"
NEW = b"#!/bin/sh\necho NEW\n"


class UpdaterTests(unittest.TestCase):
    def run_case(self, *, old=OLD, automatic=1, mode="ok", example=0, config=True):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            work = root / "work"
            work.mkdir()
            binary = work / "Catcher-android-arm64"
            if old is not None:
                binary.write_bytes(old)
            if config:
                (work / "config-android.yaml").write_text("keep: me\n")
            helpers = root / "helpers"
            helpers.mkdir()
            (helpers / "id").write_text("#!/bin/sh\necho 0\n", newline="\n")
            (helpers / "curl").write_text('''#!/bin/sh
while [ "$#" -gt 0 ]; do
    case "$1" in
        -o) out="$2"; shift 2 ;;
        *) url="$1"; shift ;;
    esac
done
echo "$url" >> "$TEST_LOG"
case "$url" in
    *sha256=1)
        [ "$TEST_MODE" = offline ] && exit 22
        if [ "$TEST_MODE" = malformed ]; then echo invalid > "$out"
        else echo "$TEST_HASH" > "$out"; fi ;;
    *.example) echo 'example: yes' > "$out" ;;
    *)
        if [ "$TEST_MODE" = partial ]; then echo partial > "$out"; exit 18; fi
        if [ "$TEST_MODE" = corrupt ]; then echo corrupt > "$out"
        else cp "$TEST_BINARY" "$out"; fi ;;
esac
''', newline="\n")
            for helper in helpers.iterdir():
                helper.chmod(0o755)
            remote = root / "remote"
            remote.write_bytes(NEW)
            script = SCRIPT.read_text(encoding="utf-8")
            script = script.replace('ROOT_DIR="/data/local/tmp/catcher"', f'ROOT_DIR="{work.as_posix()}"')
            script = script.replace("AUTO_UPDATE_BINARY=1", f"AUTO_UPDATE_BINARY={automatic}", 1)
            script = script.replace("AUTO_UPDATE_CONFIG_EXAMPLE=1", f"AUTO_UPDATE_CONFIG_EXAMPLE={example}", 1)
            # Only omit Android DNS setup; exercise the actual updater and exec.
            script = script[:script.index("if [ ! -f /etc/resolv.conf ]")] + 'exec "./$BIN_NAME" -config "$CONFIG_NAME"\n'
            launch = root / "launch.sh"
            launch.write_text(script, encoding="utf-8", newline="\n")
            log = root / "requests"
            env = dict(os.environ, TEST_LOG=log.as_posix(), TEST_MODE=mode,
                       TEST_HASH=hashlib.sha256(NEW).hexdigest(), TEST_BINARY=remote.as_posix())
            result = subprocess.run(
                [BASH, "-c", 'helper="$1"; if command -v cygpath >/dev/null; then helper=$(cygpath -u "$helper"); fi; export PATH="$helper:$PATH"; sh "$2"', "test", helpers.as_posix(), launch.as_posix()],
                env=env, capture_output=True, timeout=15,
            )
            return (result.returncode, binary.read_bytes() if binary.exists() else None,
                    log.read_text().splitlines() if log.exists() else [],
                    (work / "config-android.yaml").read_text() if (work / "config-android.yaml").exists() else None,
                    [p.name for p in work.iterdir() if p.name.startswith(".catcher-")])

    def test_disabled_does_not_contact_server(self):
        code, binary, requests, config, leftovers = self.run_case(automatic=0)
        self.assertEqual((code, binary, requests, config, leftovers), (0, OLD, [], "keep: me\n", []))

    def test_same_binary_only_requests_hash(self):
        code, binary, requests, _, _ = self.run_case(old=NEW)
        self.assertEqual((code, binary, len(requests)), (0, NEW, 1))
        self.assertTrue(requests[0].endswith("?sha256=1"))

    def test_update_preserves_configuration(self):
        code, binary, requests, config, leftovers = self.run_case(example=1)
        self.assertEqual((code, binary, len(requests), config, leftovers), (0, NEW, 3, "keep: me\n", []))

    def test_first_install_even_when_disabled(self):
        code, binary, requests, config, _ = self.run_case(old=None, automatic=0, config=False)
        self.assertEqual((code, binary, len(requests), config), (0, NEW, 3, "example: yes\n"))

    def test_failures_preserve_old_binary(self):
        for mode in ("offline", "malformed", "partial", "corrupt"):
            with self.subTest(mode=mode):
                code, binary, _, config, leftovers = self.run_case(mode=mode)
                self.assertEqual((code, binary, config, leftovers), (0, OLD, "keep: me\n", []))

    def test_failed_first_install_stops(self):
        for mode in ("offline", "malformed", "partial", "corrupt"):
            with self.subTest(mode=mode):
                code, binary, _, _, leftovers = self.run_case(old=None, mode=mode)
                self.assertEqual((code, binary, leftovers), (1, None, []))


if __name__ == "__main__":
    unittest.main()
