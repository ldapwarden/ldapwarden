#!/usr/bin/env node
import { chromium } from 'playwright';

const BASE_URL = process.env.BASE_URL || 'http://localhost:5173';
const USERNAME = process.env.USERNAME || 'admin';
const PASSWORD = process.env.PASSWORD || 'admin123';
const OUTPUT_DIR = process.env.OUTPUT_DIR || 'docs/screenshots';
// Point at an already-installed browser when `npx playwright install` is not an
// option (e.g. a cached revision that does not match this playwright build).
const CHROME_PATH = process.env.CHROME_PATH;

async function main() {
  const browser = await chromium.launch(CHROME_PATH ? { executablePath: CHROME_PATH } : {});
  const context = await browser.newContext({
    viewport: { width: 1280, height: 800 },
  });
  const page = await context.newPage();

  // The router devtools are dev-only, and these screenshots are taken against a
  // dev server — so the floating badge lands in the corner of every image in the
  // README. Hide it: it is part of the toolchain, not of the product. Its open
  // state is remembered in localStorage, which is why the images used to differ
  // from one run to the next.
  await context.addInitScript(() => {
    const style = document.createElement('style');
    style.textContent =
      '.tsqd-parent-container, [class*="TanStackRouterDevtools"], ' +
      'button[aria-label*="TanStack"], .tsrd-open-btn-container { display: none !important; }';
    document.addEventListener('DOMContentLoaded', () => document.head.append(style));
  });

  const shoot = async (step, name) => {
    console.log(`${step} ${name}`);
    await page.waitForLoadState('networkidle').catch(() => {});
    await page.waitForTimeout(1000); // let queries settle
    await page.screenshot({ path: `${OUTPUT_DIR}/${name}.png` });
  };

  console.log('Taking screenshots...\n');

  await page.goto(`${BASE_URL}/login`);
  await shoot('1.', '01-login');

  console.log('2. Logging in...');
  await page.fill('input[id="username"]', USERNAME);
  await page.fill('input[id="password"]', PASSWORD);
  await page.click('button[type="submit"]');
  // Login lands on the dashboard; wait for anything but /login.
  await page.waitForURL((url) => !url.pathname.startsWith('/login'), { timeout: 10000 });
  await shoot('3.', '02-dashboard');

  await page.goto(`${BASE_URL}/users`);
  await shoot('4.', '03-users-list');

  // Click the first user link (inside a table cell).
  const userLink = page.locator('table tbody tr td a').first();
  if (await userLink.count() > 0) {
    await userLink.click();
    await shoot('5.', '04-user-details');
  }

  await page.click('a[href="/groups"]');
  await shoot('6.', '05-groups-list');

  const groupLink = page.locator('table tbody tr td a').first();
  if (await groupLink.count() > 0) {
    await groupLink.click();
    await shoot('7.', '06-group-details');
  }

  await page.click('a[href="/sudo-roles"]');
  await shoot('8.', '07-sudo-roles');

  // Filter to updates and expand the first one, so the shot shows the
  // before/after diff — the point of the page.
  await page.goto(`${BASE_URL}/audit-logs?action=user.update`);
  await page.waitForLoadState('networkidle').catch(() => {});
  await page.waitForTimeout(1000);
  const expand = page.locator('button[aria-label="Show details"]').first();
  if (await expand.count() > 0) {
    await expand.click();
    await page.waitForTimeout(300);
  }
  await shoot('9.', '08-audit-logs');

  // Administration lives in the account menu.
  await page.locator('nav button').filter({ hasText: USERNAME }).click();
  await page.waitForTimeout(300);
  await page.click('a[href="/admin"]');
  await shoot('10.', '09-admin');

  // The command palette, opened over the users list.
  await page.goto(`${BASE_URL}/users`);
  await page.waitForTimeout(800);
  await page.keyboard.press('Control+k');
  await page.waitForTimeout(300);
  await page.keyboard.type('a', { delay: 60 });
  await shoot('11.', '10-command-palette');

  await browser.close();
  console.log(`\nDone! Screenshots saved to ${OUTPUT_DIR}/`);
}

main().catch((err) => {
  console.error('Error:', err.message);
  process.exit(1);
});
