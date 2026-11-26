#!/usr/bin/env bun
// Start a built binary from apps/bin without caring about the host OS.
// The binaries load ../../.env themselves, so the working directory stays at the
// repo root.
//
// PORT is the backend port; the reverse proxy must listen on the public panel
// port instead, so it gets PROXY_PORT (which wins over PORT in main.go) or
// PANEL_PORT from the environment.
import { spawn } from 'node:child_process';
import { readFileSync } from 'node:fs';
import { join, resolve } from 'node:path';

const name = process.argv[2];
if (!name) {
  console.error('usage: bun scripts/run.mjs <backend|reverse-proxy>');
  process.exit(2);
}

const root = resolve(import.meta.dir, '..');
const exe = process.platform === 'win32' ? '.exe' : '';

function readEnv(key) {
  if (process.env[key]) return process.env[key];
  try {
    const line = readFileSync(join(root, '.env'), 'utf8')
      .split(/\r?\n/)
      .find((l) => l.trim().startsWith(`${key}=`));
    return line ? line.slice(line.indexOf('=') + 1).trim() : undefined;
  } catch {
    return undefined;
  }
}

const env = { ...process.env };
if (name === 'reverse-proxy') {
  env.PROXY_PORT = readEnv('PANEL_PORT') ?? env.PROXY_PORT ?? '7777';
  // The reverse proxy talks to the backend on localhost, not on 0.0.0.0:$PORT.
  env.BACKEND_URL = readEnv('BACKEND_URL') ?? 'http://localhost:3001';
  // The binary's own default is ../frontend/dist, which only resolves when the
  // cwd is apps/reverse-proxy. From the repo root the bundle is here:
  env.STATIC_DIR = env.STATIC_DIR ?? join(root, 'apps', 'frontend', 'dist');
}

const child = spawn(join(root, 'apps', 'bin', name + exe), [], {
  cwd: root,
  env,
  stdio: 'inherit',
});

child.on('exit', (code, signal) => {
  if (signal) process.kill(process.pid, signal);
  else process.exit(code ?? 1);
});
for (const signal of ['SIGINT', 'SIGTERM']) {
  process.on(signal, () => child.kill(signal));
}
