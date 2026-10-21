import { error } from '@sveltejs/kit';
import { serverApi } from '$lib/server/api';
import { sectionBySlug } from '$lib/collectionSections';
import { loadCollectionSections } from '$lib/server/collectionSections';
import type { PageServerLoad } from './$types';

// Server-render one section of the /collections hub in full: every collection in
// it, ranked by open jobs. Only this section's counts are fetched. An unknown slug,
// or a section that turns out empty, is a 404.
export const load: PageServerLoad = async ({ params, fetch }) => {
  const def = sectionBySlug(params.group);
  if (!def) error(404, 'Collection group not found');
  const [section] = await loadCollectionSections(serverApi(fetch), [def]);
  if (!section) error(404, 'Collection group not found');
  return { section };
};
