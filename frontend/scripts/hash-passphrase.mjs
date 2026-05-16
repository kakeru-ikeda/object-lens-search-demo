import { pbkdf2Sync, randomBytes } from 'node:crypto';

const passphrase = process.argv[2];
const iterationsArg = process.argv[3];
const iterations = iterationsArg ? Number(iterationsArg) : 600_000;

if (!passphrase) {
  console.error('Usage: npm run auth:hash -- "your long passphrase" [iterations]');
  process.exit(1);
}

if (!Number.isInteger(iterations) || iterations <= 0) {
  console.error('Iterations must be a positive integer.');
  process.exit(1);
}

const salt = randomBytes(16);
const hash = pbkdf2Sync(passphrase, salt, iterations, 32, 'sha256');

console.log(`VITE_AUTH_PASSPHRASE_HASH=${hash.toString('hex')}`);
console.log(`VITE_AUTH_PASSPHRASE_SALT=${salt.toString('hex')}`);
console.log(`VITE_AUTH_PASSPHRASE_ITERATIONS=${iterations}`);
