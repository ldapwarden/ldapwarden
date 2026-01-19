#!/usr/bin/env node
import { chromium } from 'playwright';

const BASE_URL = process.env.BASE_URL || 'http://localhost:5173';
const USERNAME = process.env.USERNAME || 'admin';
const PASSWORD = process.env.PASSWORD || 'admin123';
const OUTPUT_DIR = process.env.OUTPUT_DIR || 'docs/screenshots';

async function main() {
  const browser = await chromium.launch();
  const context = await browser.newContext({
    viewport: { width: 1280, height: 800 },
  });
  const page = await context.newPage();

  console.log('Taking screenshots...\n');

  // Login page
  console.log('1. Login page');
  await page.goto(`${BASE_URL}/login`);
  await page.waitForLoadState('networkidle');
  await page.screenshot({ path: `${OUTPUT_DIR}/01-login.png` });

  // Perform login
  console.log('2. Logging in...');
  await page.fill('input[id="username"]', USERNAME);
  await page.fill('input[id="password"]', PASSWORD);
  await page.click('button[type="submit"]');
  await page.waitForURL('**/users**', { timeout: 10000 });
  await page.waitForLoadState('networkidle');
  await page.waitForTimeout(1000); // Wait for data to load

  // Users list
  console.log('3. Users list');
  await page.screenshot({ path: `${OUTPUT_DIR}/02-users-list.png` });

  // Click on first user link (inside table cell)
  console.log('4. User details');
  const userLink = page.locator('table tbody tr td a').first();
  if (await userLink.count() > 0) {
    await userLink.click();
    await page.waitForLoadState('networkidle');
    await page.waitForTimeout(1000); // Wait for details to render
    await page.screenshot({ path: `${OUTPUT_DIR}/03-user-details.png` });
  }

  // Groups list
  console.log('5. Groups list');
  await page.click('a[href="/groups"]');
  await page.waitForLoadState('networkidle');
  await page.waitForTimeout(1000); // Wait for data to load
  await page.screenshot({ path: `${OUTPUT_DIR}/04-groups-list.png` });

  // Click on first group link
  console.log('6. Group details');
  const groupLink = page.locator('table tbody tr td a').first();
  if (await groupLink.count() > 0) {
    await groupLink.click();
    await page.waitForLoadState('networkidle');
    await page.waitForTimeout(1000); // Wait for details to render
    await page.screenshot({ path: `${OUTPUT_DIR}/05-group-details.png` });
  }

  // Sudo Roles
  console.log('7. Sudo roles');
  await page.click('a[href="/sudo-roles"]');
  await page.waitForLoadState('networkidle');
  await page.waitForTimeout(1000);
  await page.screenshot({ path: `${OUTPUT_DIR}/06-sudo-roles.png` });

  // Audit logs
  console.log('8. Audit logs');
  await page.click('a[href="/audit-logs"]');
  await page.waitForLoadState('networkidle');
  await page.waitForTimeout(1000);
  await page.screenshot({ path: `${OUTPUT_DIR}/07-audit-logs.png` });

  // Admin page - click on user dropdown first
  console.log('9. Admin page');
  // Find the dropdown trigger button with the user's name
  await page.locator('nav button').filter({ hasText: 'admin' }).click();
  await page.waitForTimeout(300);
  await page.click('a[href="/admin"]');
  await page.waitForLoadState('networkidle');
  await page.waitForTimeout(500);
  await page.screenshot({ path: `${OUTPUT_DIR}/08-admin.png` });

  await browser.close();
  console.log(`\nDone! Screenshots saved to ${OUTPUT_DIR}/`);
}

main().catch((err) => {
  console.error('Error:', err.message);
  process.exit(1);
});
