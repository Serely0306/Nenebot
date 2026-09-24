"""Execute the generated script against Surge's callback API using Node.js."""
import shutil
import subprocess
import unittest

from sekai.services.help_texts import build_ios_upload_script


@unittest.skipUnless(shutil.which("node"), "Node.js required")
class IosScriptRuntimeTests(unittest.TestCase):
    def test_capture_chunk_upload_and_failure_passthrough(self):
        result = subprocess.run(
            ["node", "-e", NODE_TEST],
            input=build_ios_upload_script("example.com", "cn", "suite"),
            text=True, capture_output=True, timeout=20,
        )
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("runtime cases passed", result.stdout)


NODE_TEST = r'''
const vm = require('node:vm');
const assert = require('node:assert/strict');
const script = require('node:fs').readFileSync(0, 'utf8');
const originalUrl = 'https://mkcn-prod-public-60001-2.dailygn.com/api/suite/user/123?isLogin=true';
async function run(body, mode = 'ok', status = 200) {
  const posts = [], logs = [];
  let done = 0, pending = 0, maxPending = 0, timer, deferred;
  const context = {
    Uint8Array, ArrayBuffer,
    $request: {url: originalUrl}, $response: {status, body},
    console: {log: (...args) => logs.push(args.join(' '))},
    setTimeout: callback => { timer = callback; return 1; },
    clearTimeout: () => {},
    $done: value => { assert.equal(Object.keys(value).length, 0); done++; },
    $httpClient: {post: (options, callback) => {
      assert.equal(typeof callback, 'function');
      posts.push(options);
      maxPending = Math.max(maxPending, ++pending);
      const complete = () => {
        pending--;
        if (mode === 'network') return callback('network failure');
        if (mode === 'throw') throw new Error('sync failure');
        if (mode === 'http') return callback(null, {status: 500}, 'server failure');
        if (mode === 'redirect') return callback(null, {status: 302}, 'redirect');
        if (mode === 'html') return callback(null, {status: 200}, '<html>error</html>');
        if (mode === 'rejected') return callback(null, {status: 200}, '{"success":false}');
        const last = Number(options.headers['X-Chunk-Index']) === Number(options.headers['X-Total-Chunks']) - 1;
        callback(null, {status: 200}, JSON.stringify({success: true, complete: last}));
      };
      if (mode === 'throw') { pending--; throw new Error('sync failure'); }
      if (mode === 'timeout') { deferred = complete; return; }
      queueMicrotask(complete);
      // Real API returns no Promise.
    }}
  };
  const promise = vm.runInNewContext(script, context, {timeout: 1000});
  if (mode === 'timeout') { timer(); deferred(); }
  await promise;
  assert.equal(done, 1);
  assert.deepEqual(Array.from(context.$response.body || []), Array.from(body || []));
  return {posts, logs, maxPending};
}
(async () => {
  const backing = new Uint8Array(2 * 1024 * 1024 + 127);
  for (let i = 0; i < backing.length; i++) backing[i] = i % 251;
  const input = backing.subarray(19, backing.length - 13);
  for (const body of [input, input.slice().buffer]) {
    const {posts, maxPending} = await run(body);
    assert.equal(posts.length, 3);
    assert.equal(maxPending, 1);
    assert.deepEqual(Buffer.concat(posts.map(p => Buffer.from(p.body))), Buffer.from(input));
    assert.equal(new Set(posts.map(p => p.headers['X-Upload-Id'])).size, 1);
    posts.forEach((p, i) => {
      assert.equal(p.url, 'https://example.com/api/ios/upload?region=cn&data_type=suite');
      assert.equal(p.headers['X-Original-Url'], originalUrl);
      assert.equal(p.headers['X-Chunk-Index'], String(i));
      assert.equal(p.headers['X-Total-Chunks'], '3');
      assert.equal(p.headers['Content-Type'], 'application/octet-stream');
    });
  }
  for (const mode of ['network', 'http', 'redirect', 'html', 'rejected', 'throw', 'timeout']) {
    const {posts, logs} = await run(input, mode);
    assert.equal(posts.length, 1, mode);
    assert.ok(logs.some(log => /fail|timed out/i.test(log)), mode);
  }
  for (const body of [undefined, new Uint8Array(0), 'not binary']) {
    assert.equal((await run(body)).posts.length, 0);
  }
  assert.equal((await run(input, 'ok', 500)).posts.length, 0);
  console.log('runtime cases passed');
})().catch(error => { console.error(error); process.exitCode = 1; });
'''


if __name__ == "__main__":
    unittest.main()
