import { expect, test } from '@playwright/test';

test('deep discovery links keep search and pagination through browser history', async ({ page }) => {
  await page.route('**/api/v1/videos?**', async (route) => {
    const url = new URL(route.request().url());
    const current = Number(url.searchParams.get('page')) || 1;
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ data: {
        items: [{ id: current, title: `猫片第 ${current} 页`, status: 'ready', author: { id: 9, nickname: '小黑猫' }, stats: { view_count: 3, like_count: 2, comment_count: 1 } }],
        page: current, page_size: 12, total: 30,
      } }),
    });
  });

  await page.goto('/?q=%E7%8C%AB&sort=popular&page=2');
  await expect(page.getByRole('heading', { name: '发现片段' })).toBeVisible();
  await expect(page.getByRole('searchbox', { name: '搜索视频' })).toHaveValue('猫');
  await expect(page.getByText('猫片第 2 页')).toBeVisible();

  await page.getByRole('button', { name: '上一页' }).click();
  await expect(page).toHaveURL(/page=1/);
  await expect(page.getByText('猫片第 1 页')).toBeVisible();
  await page.goBack();
  await expect(page).toHaveURL(/page=2/);
  await expect(page.getByRole('searchbox', { name: '搜索视频' })).toHaveValue('猫');
});

test('a video detail deep link exposes a shareable page URL', async ({ page, context }) => {
  await context.grantPermissions(['clipboard-read', 'clipboard-write']);
  await page.route('**/api/v1/videos/42/comments?**', (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ data: { items: [], page: 1, page_size: 50, total: 0 } }),
  }));
  await page.route('**/api/v1/videos/42', (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ data: {
      id: 42, title: '月光下的小黑猫', description: '一个可分享的深链。', status: 'ready',
      play_url: '/media/42.mp4', play_type: 'mp4', author: { id: 9, nickname: '小黑猫' },
      stats: { view_count: 8, like_count: 2, favorite_count: 1, comment_count: 0 },
    } }),
  }));

  await page.goto('/video/42');
  await expect(page.getByRole('heading', { name: '月光下的小黑猫' })).toBeVisible();
  await page.getByRole('button', { name: '复制页面链接' }).click();
  await expect(page.getByText('页面链接已复制。')).toBeVisible();
  await expect.poll(() => page.evaluate(() => navigator.clipboard.readText())).toBe('http://127.0.0.1:5173/video/42');
});

test('the development server hides project files while serving routes and source assets', async ({ request }) => {
  for (const path of ['/server.mjs', '/package.json', '/.nvmrc']) {
    expect((await request.get(path)).status(), path).toBe(404);
  }
  expect((await request.get('/login')).status()).toBe(200);
  expect((await request.get('/src/main.ts')).status()).toBe(200);
  expect((await request.get('/api/v1/not-allowlisted')).status()).toBe(404);
});
