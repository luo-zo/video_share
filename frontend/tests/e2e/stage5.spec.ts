import { expect, test } from '@playwright/test';

const username = process.env.STAGE5_E2E_USERNAME;
const password = process.env.STAGE5_E2E_PASSWORD;
const videoID = process.env.STAGE5_E2E_VIDEO_ID;
const videoTitle = process.env.STAGE5_E2E_VIDEO_TITLE;

test('real Stage 5 stack supports login, video interaction, notifications, settings and logout', async ({ page }) => {
  if (!username || !password || !videoID || !videoTitle) {
    throw new Error('Set STAGE5_E2E_USERNAME, STAGE5_E2E_PASSWORD, STAGE5_E2E_VIDEO_ID and STAGE5_E2E_VIDEO_TITLE by running backend/scripts/e2e-stage5.ps1.');
  }

  await page.goto(`/login?returnTo=${encodeURIComponent(`/video/${videoID}`)}`);
  await expect(page.getByRole('heading', { name: '欢迎回来' })).toBeVisible();
  await page.getByRole('textbox', { name: '用户名', exact: true }).fill(username);
  await page.getByLabel('密码', { exact: true }).fill(password);
  await page.getByRole('button', { name: '登录并继续' }).click();

  await expect(page).toHaveURL(new RegExp(`/video/${videoID}$`));
  await expect(page.getByRole('heading', { name: videoTitle })).toBeVisible();
  await expect(page.getByRole('button', { name: /喜欢/ })).toBeVisible();
  await expect(page.locator('#comment-input')).toBeVisible();

  const browserComment = `Browser Stage 5 ${Date.now()}`;
  await page.locator('#comment-input').fill(browserComment);
  await page.getByRole('button', { name: '发布评论' }).click();
  await expect(page.getByText(browserComment, { exact: true })).toBeVisible();

  await page.getByRole('link', { name: '设置' }).click();
  await expect(page.getByRole('heading', { name: '账号设置' })).toBeVisible();
  await page.getByRole('link', { name: '通知' }).click();
  await expect(page.getByRole('heading', { name: '站内通知' })).toBeVisible();
  await expect(page.locator('.notification-item').first()).toBeVisible();

  await page.getByRole('button', { name: '退出' }).click();
  await expect(page.getByRole('link', { name: '登录' })).toBeVisible();
});
