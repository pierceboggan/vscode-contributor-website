// Webapp Validation Template — Playwright
// Copy and adapt this script for each validation run.
// Usage: node validate.mjs

import { chromium } from 'playwright';
import { mkdirSync } from 'fs';

const BASE_URL = process.env.BASE_URL || 'http://localhost:8080';
const SCREENSHOT_DIR = new URL('./screenshots/', import.meta.url).pathname;
mkdirSync(SCREENSHOT_DIR, { recursive: true });

let exitCode = 0;
const results = [];

function pass(name) {
  results.push({ name, status: 'PASS' });
  console.log(`  ✓ ${name}`);
}

function fail(name, reason) {
  results.push({ name, status: 'FAIL', reason });
  console.error(`  ✗ ${name}: ${reason}`);
  exitCode = 1;
}

(async () => {
  const browser = await chromium.launch();
  const page = await browser.newPage();

  console.log(`\nValidating against ${BASE_URL}\n`);

  // --- Home page ---
  try {
    const res = await page.goto(`${BASE_URL}/`);
    if (res.ok()) {
      pass('Home page loads (200)');
    } else {
      fail('Home page loads', `status ${res.status()}`);
    }
    await page.screenshot({ path: `${SCREENSHOT_DIR}/home.png`, fullPage: true });
  } catch (e) {
    fail('Home page loads', e.message);
  }

  // --- About page ---
  try {
    const res = await page.goto(`${BASE_URL}/about`);
    if (res.ok()) {
      pass('About page loads (200)');
    } else {
      fail('About page loads', `status ${res.status()}`);
    }
    await page.screenshot({ path: `${SCREENSHOT_DIR}/about.png`, fullPage: true });
  } catch (e) {
    fail('About page loads', e.message);
  }

  // --- Contributors page ---
  try {
    const res = await page.goto(`${BASE_URL}/contributors`);
    if (res.ok()) {
      pass('Contributors page loads (200)');
    } else {
      fail('Contributors page loads', `status ${res.status()}`);
    }
    await page.screenshot({ path: `${SCREENSHOT_DIR}/contributors.png`, fullPage: true });
  } catch (e) {
    fail('Contributors page loads', e.message);
  }

  // --- Static CSS ---
  try {
    const res = await page.goto(`${BASE_URL}/static/style.css`);
    if (res.ok()) {
      pass('Static CSS loads (200)');
    } else {
      fail('Static CSS loads', `status ${res.status()}`);
    }
  } catch (e) {
    fail('Static CSS loads', e.message);
  }

  // ===================================================
  // ADD FEATURE-SPECIFIC CHECKS BELOW THIS LINE
  // Example:
  //
  // try {
  //   await page.goto(`${BASE_URL}/`);
  //   const heading = await page.textContent('h1');
  //   if (heading.includes('Expected Text')) {
  //     pass('Home heading contains expected text');
  //   } else {
  //     fail('Home heading', `got "${heading}"`);
  //   }
  // } catch (e) {
  //   fail('Home heading check', e.message);
  // }
  // ===================================================

  await browser.close();

  console.log(`\n${results.filter(r => r.status === 'PASS').length}/${results.length} checks passed\n`);
  process.exit(exitCode);
})();
