import test from 'node:test';
import assert from 'node:assert/strict';

import { renderNode } from '../src/view-kit.js';
import {
  WATCH_REPORT_INTERVAL_MS, actionBar, commentComposer, commentDate, commentDraft, commentList,
  optimisticRelation, relationFromServer, shouldReportWatch, watchPayload,
} from '../src/detail-view.js';
import { collectAttrs, collectText, fakeDocument } from './support/dom-stub.mjs';

const XSS = '<script>alert(1)</script>';
const detail = (overrides = {}) => ({
  id: 9,
  title: '一只猫的下午',
  description: '午后的光',
  duration_ms: 60_000,
  author: { id: 4, username: 'milo', nickname: '小猫' },
  stats: { view_count: 3, like_count: 5, favorite_count: 1, comment_count: 2 },
  viewer_state: { liked: false, favorited: false, following_author: false },
  created_at: '2026-09-07T12:00:00Z',
  ...overrides,
});

test('an optimistic toggle flips the state and moves the count by one', () => {
  assert.deepEqual(optimisticRelation({ active: false, count: 4 }, true), { active: true, count: 5 });
  assert.deepEqual(optimisticRelation({ active: true, count: 5 }, false), { active: false, count: 4 });
  assert.deepEqual(optimisticRelation({ active: true, count: 5 }, true), { active: true, count: 5 });
  assert.deepEqual(optimisticRelation({ active: false, count: 0 }, false), { active: false, count: 0 });
});

test('a failed optimistic toggle rolls back to the untouched snapshot', () => {
  const snapshot = { active: true, count: 5 };
  const optimistic = optimisticRelation(snapshot, false);
  assert.deepEqual(optimistic, { active: false, count: 4 });
  assert.deepEqual(snapshot, { active: true, count: 5 });
});

test('the server response replaces the optimistic guess', () => {
  assert.deepEqual(relationFromServer({ active: true, count: 9 }, { active: false, count: 8 }), { active: true, count: 9 });
  assert.deepEqual(relationFromServer({ active: true }, { active: false, count: 8 }), { active: true, count: 8 });
  assert.deepEqual(relationFromServer(null, { active: true, count: 3 }), { active: true, count: 3 });
});

test('watch reports are throttled to the configured interval', () => {
  assert.equal(shouldReportWatch({ lastReportedMs: 0, positionMs: 1_000, durationMs: 60_000 }), false);
  assert.equal(shouldReportWatch({ lastReportedMs: 0, positionMs: WATCH_REPORT_INTERVAL_MS, durationMs: 60_000 }), true);
  assert.equal(shouldReportWatch({ lastReportedMs: 20_000, positionMs: 30_000, durationMs: 60_000 }), false);
  assert.equal(shouldReportWatch({ lastReportedMs: 20_000, positionMs: 35_000, durationMs: 60_000 }), true);
  assert.equal(shouldReportWatch({ lastReportedMs: 0, positionMs: 5_000, durationMs: 0 }), false);
});

test('a watch payload clamps progress to the reported duration', () => {
  assert.deepEqual(watchPayload(1_200, 1_000), { progress_ms: 1_000, duration_ms: 1_000 });
  assert.deepEqual(watchPayload(500.9, 60_000.9), { progress_ms: 500, duration_ms: 60_000 });
  assert.equal(watchPayload(-1, 60_000), null);
  assert.equal(watchPayload(10, 0), null);
});

test('a comment draft validates its content and locks only while pending', () => {
  const blank = commentDraft('   ');
  assert.equal(blank.valid, false);
  assert.match(blank.error, /评论内容/);
  assert.equal(blank.disabled, false);
  const typed = commentDraft(' 很棒的记录 ');
  assert.equal(typed.content, '很棒的记录');
  assert.equal(typed.valid, true);
  assert.equal(typed.error, '');
  assert.equal(commentDraft('很棒的记录', { submitting: true }).disabled, true);
});

test('comment dates render deterministically in UTC', () => {
  assert.equal(commentDate('2026-09-07T12:00:00Z'), '2026-09-07 12:00');
  assert.equal(commentDate('不是日期'), '');
  assert.equal(commentDate(undefined), '');
});

test('action buttons expose their pressed state and running counts', () => {
  const bar = actionBar(detail({ viewer_state: { liked: true, favorited: false, following_author: true } }));
  const like = bar.children.find((child) => child.dataset?.action === 'like');
  assert.equal(like.attrs['aria-pressed'], 'true');
  assert.equal(like.attrs.type, 'button');
  assert.match(collectText(renderNode(like, fakeDocument())), /5/);
  const favorite = bar.children.find((child) => child.dataset?.action === 'favorite');
  assert.equal(favorite.attrs['aria-pressed'], 'false');
  const follow = bar.children.find((child) => child.dataset?.action === 'follow');
  assert.equal(follow.attrs['aria-pressed'], 'true');
  assert.equal(follow.dataset.userId, '4');
});

test('anonymous viewers see unpressed actions', () => {
  const bar = actionBar(detail({ viewer_state: undefined }));
  for (const action of ['like', 'favorite', 'follow']) {
    assert.equal(bar.children.find((child) => child.dataset?.action === action).attrs['aria-pressed'], 'false');
  }
});

test('the comment composer is labelled and reports validation', () => {
  const composer = commentComposer(commentDraft('写点什么'));
  const field = composer.children.find((child) => child.tag === 'textarea');
  assert.equal(field.props.value, '写点什么');
  assert.ok(composer.children.some((child) => child.attrs?.for === field.id), 'textarea needs a label');
  assert.equal(commentComposer(commentDraft(''), { error: '请写点内容' }).children.at(-2).text, '请写点内容');
});

test('only the viewer own comments get a delete control', () => {
  const comments = [
    { id: 11, user_id: 7, content: '我的评论', author: { id: 7, username: 'me', nickname: '我' }, created_at: '2026-09-07T12:00:00Z' },
    { id: 12, user_id: 8, content: '别人的评论', author: { id: 8, username: 'other', nickname: '别人' }, created_at: '2026-09-07T12:00:00Z' },
  ];
  const list = commentList(comments, 7);
  const first = list.children[0];
  const second = list.children[1];
  assert.equal(first.dataset.commentId, '11');
  assert.equal(first.children.some((child) => child.dataset?.action === 'delete-comment'), true);
  assert.equal(second.children.some((child) => child.dataset?.action === 'delete-comment'), false);
});

test('a hostile comment body stays inert text and never becomes an attribute', () => {
  const document = fakeDocument();
  const list = commentList([{ id: 11, user_id: 8, content: XSS, author: { id: 8, username: 'x', nickname: XSS } }], 7);
  const node = renderNode(list, document);
  assert.ok(collectText(node).includes(XSS));
  assert.ok(collectAttrs(node).every((value) => !value.includes(XSS)));
});

test('an empty comment list renders a friendly hint', () => {
  const node = renderNode(commentList([], 7), fakeDocument());
  assert.match(collectText(node), /还没有评论/);
});
