import type { RequestOptions } from './client';

export interface Category {
  readonly id: string | number;
  readonly slug: string;
  readonly name: string;
  readonly enabled?: boolean;
  readonly display_order?: number;
}

export interface CategoryList {
  readonly items: readonly Category[];
}

export class TaxonomyError extends Error {
  readonly code: string;

  constructor(message: string, code = 'TAXONOMY_ERROR') {
    super(message);
    this.name = 'TaxonomyError';
    this.code = code;
  }
}

export interface TaxonomyRequester {
  requestPublic(path: string, options?: RequestOptions): Promise<unknown>;
}

function responseFrom(value: unknown): CategoryList {
  if (!value || typeof value !== 'object') throw new TaxonomyError('分类响应无效。', 'INVALID_RESPONSE');
  const source = value as Record<string, unknown>;
  if (!Array.isArray(source.items)) throw new TaxonomyError('分类响应无效。', 'INVALID_RESPONSE');
  const items = source.items.filter((item): item is Record<string, unknown> => Boolean(item && typeof item === 'object'));
  if (items.some((item) => !['string', 'number'].includes(typeof item.id) || typeof item.slug !== 'string' || typeof item.name !== 'string')) {
    throw new TaxonomyError('分类响应无效。', 'INVALID_RESPONSE');
  }
  return { items: items as unknown as readonly Category[] };
}

export function createTaxonomyClient(requester: TaxonomyRequester): { listCategories(options?: { signal?: AbortSignal }): Promise<CategoryList> } {
  return {
    async listCategories(options?: { signal?: AbortSignal }): Promise<CategoryList> {
      return responseFrom(await requester.requestPublic('/categories', options));
    },
  };
}
