import { readFileSync } from 'node:fs';
import { expect, test } from '@playwright/test';

const appVersion = readFileSync(new URL('../../VERSION', import.meta.url), 'utf8').trim();

test('shows grouped PRs, renders untrusted text safely, and keeps long fields inside the popup', async ({ page }) => {
  await page.goto('/');
  await expect(page.getByText('Connected as octocat')).toBeVisible();
  await expect(page.getByText('Not available').first()).toBeVisible();
  await expect(page.locator('.title img, .title script')).toHaveCount(0);
  await expect(page.getByText(/<img src=x onerror=alert\(1\)> sanitize untrusted title/)).toBeVisible();
  await expect(page.locator('.pr-row')).toHaveCount(5);
  const scrollWidths = await page.evaluate(() => {
    const popup = document.querySelector('.popup');
    if (!(popup instanceof HTMLElement)) throw new Error('Popup element is missing');
    return {
      body: document.body.scrollWidth,
      root: document.documentElement.scrollWidth,
      popup: popup.scrollWidth,
      popupClient: popup.clientWidth,
      viewport: window.innerWidth,
    };
  });
  expect(scrollWidths.body).toBeLessThanOrEqual(scrollWidths.viewport);
  expect(scrollWidths.root).toBeLessThanOrEqual(scrollWidths.viewport);
  expect(scrollWidths.popup).toBeLessThanOrEqual(scrollWidths.popupClient);
  await page.locator('.popup').screenshot({ path: 'screenshots/popup-light-all.png' });
  await page.emulateMedia({ colorScheme: 'dark' });
  await expect
    .poll(() => page.locator('.popup').evaluate((el) => getComputedStyle(el).backgroundColor))
    .toBe('rgb(18, 18, 18)');
  const darkWidths = await page.evaluate(() => {
    const popup = document.querySelector('.popup');
    if (!(popup instanceof HTMLElement)) throw new Error('Popup element is missing');
    return {
      body: document.body.scrollWidth,
      root: document.documentElement.scrollWidth,
      popup: popup.scrollWidth,
      popupClient: popup.clientWidth,
      viewport: window.innerWidth,
    };
  });
  expect(darkWidths.body).toBeLessThanOrEqual(darkWidths.viewport);
  expect(darkWidths.root).toBeLessThanOrEqual(darkWidths.viewport);
  expect(darkWidths.popup).toBeLessThanOrEqual(darkWidths.popupClient);
  await page.locator('.popup').screenshot({ path: 'screenshots/popup-dark-all.png' });
  await page.emulateMedia({ colorScheme: 'light' });
  await expect
    .poll(() => page.locator('.popup').evaluate((el) => getComputedStyle(el).backgroundColor))
    .toBe('rgb(255, 255, 255)');
  await expect(page.getByText('acme-inc/a-very-long-private-repository-name-for-layout')).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Recently closed' })).toBeVisible();
  await page.screenshot({ path: 'screenshots/popup.png', fullPage: false });
  await page.getByRole('button', { name: 'Tabs' }).click();
  await expect(page.getByRole('navigation', { name: 'Pull request categories' })).toBeVisible();
  await page.locator('.popup').screenshot({ path: 'screenshots/popup-light-tabs.png' });
  await page.emulateMedia({ colorScheme: 'dark' });
  await expect
    .poll(() => page.locator('.popup').evaluate((el) => getComputedStyle(el).backgroundColor))
    .toBe('rgb(18, 18, 18)');
  await page.locator('.popup').screenshot({ path: 'screenshots/popup-dark-tabs.png' });
  await page.emulateMedia({ colorScheme: 'light' });
  await page.getByRole('button', { name: /Review/ }).click();
  await expect(page.getByText('Not passed')).toBeVisible();
});

test('opens PR by row keyboard activation and bell toggles without opening it', async ({ page }) => {
  await page.goto('/');
  const row = page.getByRole('button', { name: /alexcatdad\/pr-nanny pull request #101/ });
  await row.focus();
  await page.keyboard.press('Enter');
  await expect.poll(() => page.evaluate(() => (window as Window & { __openedPR?: string }).__openedPR)).toBe('pr-101');
  const openedBeforeBell = await page.evaluate(() => (window as Window & { __openedPRCount?: number }).__openedPRCount);
  const bell = page.getByRole('button', { name: 'Turn notifications on' }).first();
  await bell.focus();
  await page.keyboard.press('Space');
  await expect(page.getByRole('button', { name: 'Turn notifications off' }).first()).toHaveAttribute(
    'aria-pressed',
    'true',
  );
  expect(await page.evaluate(() => (window as Window & { __openedPRCount?: number }).__openedPRCount)).toBe(
    openedBeforeBell,
  );
});

test('settings expose account, options, and clear the PAT field before the bridge resolves', async ({ page }) => {
  await page.goto('/?settings=1');
  await page.locator('.popup').screenshot({ path: 'screenshots/settings-light.png' });
  await page.emulateMedia({ colorScheme: 'dark' });
  await page.locator('.popup').screenshot({ path: 'screenshots/settings-dark.png' });
  await page.emulateMedia({ colorScheme: 'light' });
  await page.getByRole('button', { name: 'Personal access token' }).click();
  const input = page.getByLabel('Personal access token');
  await input.fill('ghp_synthetic_example');
  await page.getByRole('button', { name: 'Connect' }).click();
  await expect(input).toHaveValue('');
  expect(
    await page.evaluate(() => (window as Window & { __PR_NANNY_PAT_PENDING__?: boolean }).__PR_NANNY_PAT_PENDING__),
  ).toBe(true);
  expect(await page.evaluate(() => localStorage.length + sessionStorage.length)).toBe(0);
  expect(await page.locator('body').innerText()).not.toContain('ghp_synthetic_example');
  await expect(page.getByLabel('Update interval')).toBeVisible();
  await expect(page.getByText('CI status changes')).toBeVisible();
  await expect(page.getByLabel('Launch at login')).toBeVisible();
  await page.keyboard.press('Escape');
  await expect
    .poll(() => page.evaluate(() => (window as Window & { __PR_NANNY_HIDDEN__?: boolean }).__PR_NANNY_HIDDEN__))
    .toBe(true);
});

test('settings display the version provided by the production build', async ({ page }) => {
  await page.goto('/?settings=1');
  await expect(page.getByText(`PR Nanny ${appVersion}`)).toBeVisible();
});

test('shows a fresh refresh as just now and keeps elapsed age for older refreshes', async ({ page }) => {
  await page.clock.install({ time: new Date('2026-10-20T05:00:00Z') });
  await page.goto('/');

  const refreshTime = page.locator('.refresh-time');
  await expect(refreshTime).toHaveText('Updated just now');
  await page.clock.runFor(60_000);
  await expect(refreshTime).toHaveText('Updated 1m ago');
});

test('shows a setup action when auth is missing and keeps cached pull requests visible offline', async ({ page }) => {
  await page.goto('/?fixtureConnection=missing_auth');
  await expect(page.getByRole('heading', { name: 'Connect to GitHub' })).toBeVisible();
  await page.getByRole('button', { name: 'Open Settings', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Settings' })).toBeVisible();
  await page.goto('/?fixtureConnection=offline');
  await expect(
    page.getByText('You are offline. Showing the pull requests saved from your last refresh.'),
  ).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Authored by you' })).toBeVisible();
  await page.goto('/?fixtureConnection=rate_limited');
  await expect(page.getByText('GitHub is limiting requests. PR Nanny will try again automatically.')).toBeVisible();
  await expect(page.getByText('Improve request caching across repeated polling cycles')).toBeVisible();
});

test('failed PAT stays disconnected, clears the field, and shows an actionable notice', async ({ page }) => {
  await page.goto('/?fixtureConnection=missing_auth&fixturePat=fail&settings=1');
  await page.getByRole('button', { name: 'Personal access token' }).click();
  const input = page.getByLabel('Personal access token');
  await input.fill('ghp_synthetic_failure');
  await page.getByRole('button', { name: 'Connect' }).click();
  await expect(input).toHaveValue('');
  await expect(page.getByRole('status').filter({ hasText: 'Could not validate that token' })).toBeVisible();
  await expect(page.getByText('GitHub connected.')).toHaveCount(0);
  expect(await page.evaluate(() => localStorage.length + sessionStorage.length)).toBe(0);
  await page.getByRole('button', { name: 'Back to pull requests' }).click();
  await expect(page.getByRole('heading', { name: 'Connect to GitHub' })).toBeVisible();
});

test('failed settings save stays actionable and prevents a second save while pending', async ({ page }) => {
  await page.goto('/?fixtureSave=fail&settings=1');
  const notifications = page.locator('.toggle-row').first().getByRole('checkbox');
  await notifications.click();
  await expect(notifications).toBeDisabled();
  await expect(page.getByLabel('CI status changes')).toBeDisabled();
  expect(
    await page.evaluate(() => (window as Window & { __PR_NANNY_SAVE_PENDING__?: boolean }).__PR_NANNY_SAVE_PENDING__),
  ).toBe(true);
  await expect(page.getByRole('status').filter({ hasText: 'Could not save settings. Try again.' })).toBeVisible();
  await expect(notifications).toBeEnabled();
  await expect(notifications).not.toBeChecked();
});

test('an older poll cannot overwrite a newer saved layout', async ({ page }) => {
  await page.clock.install({ time: new Date('2026-01-01T00:00:00Z') });
  await page.clock.pauseAt(new Date('2026-01-01T00:01:00Z'));
  await page.goto('/?fixtureStalePoll=1');
  await expect(page.getByText('Connected as octocat')).toBeVisible();
  await page.clock.runFor(1000);
  await expect
    .poll(() =>
      page.evaluate(
        () => (window as Window & { __PR_NANNY_STALE_POLL_PENDING__?: boolean }).__PR_NANNY_STALE_POLL_PENDING__,
      ),
    )
    .toBe(true);
  await page.getByRole('button', { name: 'Tabs' }).click();
  await expect(page.getByRole('button', { name: 'Tabs' })).toHaveAttribute('aria-pressed', 'true');
  await page.evaluate(() => {
    const release = (window as Window & { __PR_NANNY_RELEASE_STALE_POLL__?: () => void })
      .__PR_NANNY_RELEASE_STALE_POLL__;
    if (!release) throw new Error('The delayed stale poll is not pending.');
    release();
  });
  await page.clock.runFor(20);
  await expect(page.getByRole('button', { name: 'Tabs' })).toHaveAttribute('aria-pressed', 'true');
});

test('recent closures expire based on the actual closure timestamp', async ({ page }) => {
  await page.goto('/?fixtureClosedAt=expired');
  await page.getByRole('button', { name: 'Tabs' }).click();
  await page.getByRole('button', { name: /Recent/ }).click();
  await expect(page.getByText('No recently closed pull requests.')).toBeVisible();
});

test('a bell preference stays in sync across duplicate category rows', async ({ page }) => {
  await page.goto('/');
  const copies = page.locator('.pr-row').filter({ hasText: 'Unknown checks stay informational' });
  await expect(copies).toHaveCount(2);
  const openedBefore = await page.evaluate(
    () => (window as Window & { __openedPRCount?: number }).__openedPRCount ?? 0,
  );
  await copies.first().getByRole('button', { name: 'Turn notifications off' }).click();
  await expect(copies.nth(1).getByRole('button', { name: 'Turn notifications on' })).toHaveAttribute(
    'aria-pressed',
    'false',
  );
  expect(await page.evaluate(() => (window as Window & { __openedPRCount?: number }).__openedPRCount ?? 0)).toBe(
    openedBefore,
  );
});
