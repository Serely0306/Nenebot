import hashlib
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

from flask import Flask

# The download route does not need the production asset configuration.
with patch("sekai.services.sekai_assets.SekaiAssetRepository"):
    from sekai.web import bp


class DownloadHashTests(unittest.TestCase):
    def test_hash_tracks_actual_bytes_and_download_still_works(self):
        app = Flask(__name__)
        app.register_blueprint(bp)
        client = app.test_client()
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "Catcher-android-arm64"
            with patch("sekai.web.DOWNLOAD_FILES", {path.name: directory}):
                url = "/upload/download/" + path.name
                self.assertEqual(client.get(url + "?sha256=1").status_code, 404)
                for payload in (b"first", b"other"):
                    path.write_bytes(payload)
                    response = client.get(url + "?sha256=1")
                    self.assertEqual(response.status_code, 200)
                    self.assertEqual(response.get_data(as_text=True).strip(), hashlib.sha256(payload).hexdigest())
                    self.assertEqual(response.headers["Cache-Control"], "no-store")
                    with client.get(url) as download:
                        self.assertEqual(download.data, payload)
                self.assertEqual(client.get("/upload/download/unlisted?sha256=1").status_code, 404)


if __name__ == "__main__":
    unittest.main()
