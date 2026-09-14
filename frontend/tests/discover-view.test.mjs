import test from 'node:test';
import assert from 'node:assert/strict';

import { renderNode } from '../src/view-kit.js';
import {
  DEFAULT_PAGE_SIZE, DISCOVER_SORTS, applyQuery, applySort, changePage,
  discoverGrid, discoverPagination, discoverRequest, initialDiscoverState,
  paginationBounds, resultSummary, searchForm,
} from '../src/discover-view.js';
import { collectAttrs, collectText, fakeDocument } from './support/dom-stub.mjs';

const XSS = '<img src=x onerror="alert(1)">';
const video = (overrides = {}) => ({
  id: 9,
  title: '一只猫的下午',
  description: '午后的光',
  cover_url: null,
  created_at: '2026-09-07T12:00:00Z',
  author: { id: 4, username: 'milo', nickname: '小猫' },
  stats: { view_count: 3, like_count: 2, favorite_count: 1, comment_count: 0 },
  ...overrides,
});

test('discovery starts on the latest page with default paging', () => {
  const state = initialDiscoverState();
  assert.deepEqual(state, { query: '', sort: 'latest', page: 1, pageSize: DEFAULT_PAGE_SIZE });
  assert.deepEqual(discoverRequest(state), { query: '', sort: 'latest', page: 1, pageSize: 12 });
});

test('submitting a search trims the keyword and resets to page one', () => {
  const searched = applyQuery({ ...initialDiscoverState(), page: 3, sort: 'popular' }, '  小猫  ');
  assert.equal(searched.query, '小猫');
  assert.equal(searched.page, 1);
  assert.equal(searched.sort, 'popular');
  assert.deepEqual(discoverRequest(searched), { query: '小猫', sort: 'popular', page: 1, pageSize: 12 });
});

test('sorting resets to page one and falls back to latest for unknown values', () => {
  const sorted = applySort({ ...initialDiscoverState(), page: 4, query: '猫' }, 'popular');
  assert.equal(sorted.sort, 'popular');
  assert.equal(sorted.page, 1);
  assert.equal(sorted.query, '猫');
  assert.equal(applySort(sorted, 'drop-table').sort, 'latest');
});

test('paging keeps the active query and sort and never goes below one', () => {
  const base = applySort(applyQuery(initialDiscoverState(), '猫'), 'popular');
  const second = changePage(base, 1);
  assert.equal(second.page, 2);
  assert.equal(second.query, '猫');
  assert.equal(second.sort, 'popular');
  assert.equal(changePage(second, -5).page, 1);
});

test('the search form exposes a labelled keyword field and sort control', () => {
  const form = searchForm(applyQuery(initialDiscoverState(), '猫'));
  assert.equal(form.tag, 'form');
  assert.equal(form.attrs.role, 'search');
  const input = form.children.find((child) => child.attrs?.type === 'search');
  assert.equal(input.props.value, '猫');
  assert.equal(input.attrs.name, 'q');
  assert.ok(form.children.some((child) => child.attrs?.for === input.id), 'keyword field needs a label');
  const select = form.children.find((child) => child.tag === 'select');
  assert.equal(select.props.value, 'latest');
  assert.deepEqual(
    select.children.map((option) => option.attrs.value),
    DISCOVER_SORTS.map((sort) => sort.value),
  );
  assert.ok(form.children.some((child) => child.attrs?.for === select.id), 'sort control needs a label');
});

test('a hostile title stays inert text and never becomes an attribute', () => {
  const document = fakeDocument();
  const node = renderNode(discoverGrid({ items: [video({ title: XSS })], total: 1 }), document);
  assert.ok(collectText(node).includes(XSS));
  assert.ok(collectAttrs(node).every((value) => !value.includes(XSS)));
});

test('empty results render a readable empty state', () => {
  const document = fakeDocument();
  const node = renderNode(discoverGrid({ items: [], total: 0 }), document);
  assert.match(collectText(node), /没有找到/);
});

test('the result summary reports the keyword and the total', () => {
  assert.match(resultSummary(applyQuery(initialDiscoverState(), '猫'), { total: 3 }), /猫/);
  assert.match(resultSummary(applyQuery(initialDiscoverState(), '猫'), { total: 3 }), /3/);
  assert.match(resultSummary(initialDiscoverState(), { total: 5 }), /5/);
});

test('pagination bounds clamp to the available pages', () => {
  const state = { ...initialDiscoverState(), page: 2 };
  assert.deepEqual(
    paginationBounds(state, { total: 30, page_size: 12 }),
    { page: 2, totalPages: 3, canPrev: true, canNext: true },
  );
  assert.deepEqual(
    paginationBounds(state, { total: 0, page_size: 12 }),
    { page: 1, totalPages: 1, canPrev: false, canNext: false },
  );
});

test('pagination controls reflect the available pages', () => {
  const document = fakeDocument();
  const first = discoverPagination({ ...initialDiscoverState(), page: 1 }, { total: 30, page_size: 12 });
  const node = renderNode(first, document);
  assert.match(collectText(node), /1 \/ 3/);
  const [prev, next] = first.children.filter((child) => child.tag === 'button');
  assert.equal(prev.props.disabled, true);
  assert.equal(next.props.disabled, false);
});
