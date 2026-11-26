#!/usr/bin/env bun
// Cross-platform build for the panel. `go build -o ../bin/backend` produces a
// file without an extension on Windows too, so the .exe suffix is added here
// instead of being hard-coded in package.json.
import { spawnSync } from 'node:child_process';
import { mkdirSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';

const root = resolve(import.meta.dir, '..');
const binDir = join(root, 'apps', 'bin');
const exe = process.platform === 'win32' ? '.exe' : '';

function run(command, args, cwd) {
  const result = spawnSync(command, args, { cwd, stdio: 'inherit', shell: process.platform === 'win32' });
  if (result.status !== 0) process.exit(result.status ?? 1);
}

function frontend() {
  run('bun', ['run', 'build'], join(root, 'apps', 'frontend'));
}

function goBinary(name, dir) {
  mkdirSync(binDir, { recursive: true });
  run('go', ['build', '-o', join(binDir, name + exe), '.'], join(root, dir));
}

const target = process.argv[2] ?? 'all';
const steps = {
  frontend,
  backend: () => goBinary('backend', 'apps/backend'),
  proxy: () => goBinary('reverse-proxy', 'apps/reverse-proxy'),
};

if (target === 'all') {
  frontend();
  steps.backend();
  steps.proxy();
} else if (steps[target]) {
  steps[target]();
} else {
  console.error(`unknown target ${target}; use frontend, backend, proxy or all`);
  process.exit(2);
}
