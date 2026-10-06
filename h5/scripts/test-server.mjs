import { test } from "node:test";
import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { createServer } from "node:net";
import { once } from "node:events";

// Exercise the built start path and the default production asset prefix.
test("built CommonJS server starts without NODE_ENV and serves actual HTML/JS/CSS", async () => {
  const reservation = createServer();
  reservation.listen(0, "127.0.0.1");
  await once(reservation, "listening");
  const { port } = reservation.address();
  await new Promise((resolve) => reservation.close(resolve));
  const env = { ...process.env, PORT: String(port) };
  delete env.NODE_ENV;
  const child = spawn(process.execPath, ["dist/server.cjs"], {
    cwd: new URL("..", import.meta.url),
    env,
    stdio: ["ignore", "pipe", "pipe"],
  });
  let output = "";
  try {
    await new Promise((resolve, reject) => {
      const timeout = setTimeout(
        () => reject(new Error(`server did not start: ${output}`)),
        5000,
      );
      child.once("exit", (code) => {
        clearTimeout(timeout);
        reject(new Error(`server exited ${code}: ${output}`));
      });
      child.stderr.on("data", (data) => {
        output += data;
      });
      child.stdout.on("data", (data) => {
        output += data;
        if (output.includes("WorkBuddy server running")) {
          clearTimeout(timeout);
          resolve();
        }
      });
    });
    const root = `http://127.0.0.1:${port}`;
    for (const route of ["/", "/static/h5/"]) {
      const response = await fetch(root + route);
      assert.equal(response.status, 200);
      assert.match(response.headers.get("content-type"), /text\/html/);
      const html = await response.text();
      const assets = [
        ...html.matchAll(/(?:src|href)="(\/static\/h5\/assets\/[^\"]+)"/g),
      ].map((match) => match[1]);
      assert.ok(assets.some((asset) => asset.endsWith(".js")));
      assert.ok(assets.some((asset) => asset.endsWith(".css")));
      for (const asset of assets) {
        const response = await fetch(root + asset);
        assert.equal(response.status, 200);
        assert.match(
          response.headers.get("content-type"),
          asset.endsWith(".css") ? /text\/css/ : /javascript/,
        );
        assert.ok(!(await response.text()).trimStart().startsWith("<!doctype"));
      }
    }
    assert.doesNotMatch(output, /ERR_INVALID_ARG_TYPE/);
  } finally {
    if (child.exitCode === null && child.signalCode === null) {
      const closed = once(child, "exit");
      child.kill();
      await closed;
    }
  }
});
