import { writeStdout } from "../../src/output/stdout";

// 256KB payload with a trailer marker: guards against truncation at the
// 64KB pipe-buffer boundary when stdout is a non-TTY pipe. Spawned as a
// subprocess by tests/output-stdout.test.ts so the real writeStdout runs
// (the test process itself has writeStdout mocked by the preload).
const TRAILER = "END\n";
const SIZE = 256 * 1024;
writeStdout("x".repeat(SIZE - TRAILER.length) + TRAILER);
