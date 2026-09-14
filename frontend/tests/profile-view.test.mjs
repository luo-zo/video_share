import test from 'node:test';
import assert from 'node:assert/strict';

import { renderNode } from '../src/view-kit.js';
import {
  PROFILE_TABS, deleteConfirmation, normalizeTab, ownerEditForm, ownerPatch,
  profileGrid, profileRequest, tabList,
} from '../src/profile-view.js';
import { collectAttrs, collectText, fakeDocument } from './support/dom-stub.mjs';

const XSS = '"><img src=x onerror=alert(1)>';
const owned = (overrides = {}) => ({
  id: 5,
  title: '我的猫',
  description: '它的下午',
  status: 'ready',
  visibility: 'public',
  created_at: '2026-09-07T12:00:00Z',
  stats: { view_count: 1, like_count: 0, favorite_count: 0, comment_count: 0 },
  ...overrides,
});

test('the personal center offers four tabs and falls back to submissions', () => {
  assert.deepEqual(PROFILE_TABS.map((tab) => tab.id), ['videos', 'favorites', 'history', 'follows']);
  assert.deepEqual(PROFILE_TABS.map((tab) => tab.label), ['投稿', '收藏', '历史', '关注']);
  assert.equal(normalizeTab('history'), 'history');
  assert.equal(normalizeTab('follows'), 'follows');
  assert.equal(normalizeTab('bogus'), 'videos');
  assert.equal(normalizeTab(undefined), 'videos');
});

test('each tab maps to the loader that owns its data', () => {
  assert.deepEqual(profileRequest('videos', { page: 2, pageSize: 12 }), { loader: 'listMyVideos', page: 2, pageSize: 12 });
  assert.deepEqual(profileRequest('favorites', { page: 1, pageSize: 12 }), { loader: 'listFavorites', page: 1, pageSize: 12 });
  assert.deepEqual(profileRequest('history', { page: 3, pageSize: 12 }), { loader: 'listHistory', page: 3, pageSize: 12 });
  assert.deepEqual(profileRequest('follows', { page: 1, pageSize: 12 }), { loader: 'listFollows', page: 1, pageSize: 12 });
});

test('tab controls mark the selected tab for assistive technology', () => {
  const tabs = tabList('history');
  assert.equal(tabs.attrs.role, 'tablist');
  const buttons = tabs.children.filter((child) => child.tag === 'button');
  assert.equal(buttons.length, 4);
  const selected = buttons.find((button) => button.dataset.tab === 'history');
  assert.equal(selected.attrs['aria-selected'], 'true');
  assert.equal(selected.attrs['aria-current'], 'page');
  assert.equal(buttons.find((button) => button.dataset.tab === 'videos').attrs['aria-selected'], 'false');
});

test('owner patch payloads carry only the fields the author changed', () => {
  const patch = ownerPatch({ title: ' 新标题 ', description: '', visibility: 'private' });
  assert.equal(patch.valid, true);
  assert.deepEqual(patch.values, { title: '新标题', description: '', visibility: 'private' });
  const empty = ownerPatch({});
  assert.equal(empty.valid, false);
  assert.match(empty.errors.title, /至少/);
  const badVisibility = ownerPatch({ visibility: 'hidden' });
  assert.equal(badVisibility.valid, false);
  assert.match(badVisibility.errors.visibility, /public/);
});

test('the owner edit form is prefilled and offers an explicit delete', () => {
  const form = ownerEditForm(owned());
  const title = form.children.find((child) => child.attrs?.name === 'title');
  assert.equal(title.props.value, '我的猫');
  const description = form.children.find((child) => child.attrs?.name === 'description');
  assert.equal(description.props.value, '它的下午');
  const visibility = form.children.find((child) => child.attrs?.name === 'visibility');
  assert.equal(visibility.props.value, 'public');
  assert.deepEqual(visibility.children.map((option) => option.attrs.value), ['public', 'private']);
  const remove = form.children.find((child) => child.dataset?.action === 'delete-video');
  assert.equal(remove.dataset.videoId, '5');
});

test('the delete confirmation names the video and demands an explicit choice', () => {
  const dialog = deleteConfirmation(owned({ title: '一只猫的下午' }));
  assert.equal(dialog.attrs.role, 'alertdialog');
  assert.match(collectText(renderNode(dialog, fakeDocument())), /一只猫的下午/);
  for (const action of ['cancel-delete', 'confirm-delete']) {
    assert.ok(dialog.children.some((child) => child.dataset?.action === action), `missing ${action}`);
  }
});

test('followed accounts render as people, not videos', () => {
  const grid = profileGrid('follows', { items: [{ id: 4, username: 'milo', nickname: '小猫' }], total: 1 });
  const text = collectText(renderNode(grid, fakeDocument()));
  assert.match(text, /小猫/);
  assert.doesNotMatch(text, /投稿/);
});

test('empty personal lists explain what will appear there', () => {
  assert.match(collectText(renderNode(profileGrid('favorites', { items: [], total: 0 }), fakeDocument())), /收藏/);
  assert.match(collectText(renderNode(profileGrid('history', { items: [], total: 0 }), fakeDocument())), /历史/);
  assert.match(collectText(renderNode(profileGrid('follows', { items: [], total: 0 }), fakeDocument())), /关注/);
});

test('a hostile author title stays inert text in owner controls', () => {
  const document = fakeDocument();
  const node = renderNode(ownerEditForm(owned({ title: XSS })), document);
  assert.ok(collectText(node).includes(XSS));
  assert.ok(collectAttrs(node).every((value) => !value.includes(XSS)));
});
