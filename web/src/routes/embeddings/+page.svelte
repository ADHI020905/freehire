<script lang="ts">
  import { page } from '$app/state';
  import { api } from '$lib/api';
  import Seo from '$lib/components/Seo.svelte';
  import EmbeddingProgress from '$lib/components/EmbeddingProgress.svelte';
  import { SectionLabel } from '$lib/ui';
  import type { PageData } from './$types';

  let { data }: { data: PageData } = $props();

  const canonical = $derived(`${page.url.origin}/embeddings`);
</script>

<Seo
  title="Embeddings — freehire"
  description="Live progress of semantic embeddings across the freehire catalogue — how many postings are covered, how fast the rest is going, and how to help speed it up."
  {canonical}
/>

<div class="mx-auto w-full max-w-4xl px-4 py-10 sm:py-14">
  <header class="mb-10">
    <SectionLabel text="semantic search" />
    <h1 class="mt-4 text-4xl font-semibold tracking-tighter sm:text-5xl">Embeddings.</h1>
    <p class="mt-4 max-w-2xl text-lg leading-relaxed text-muted-foreground">
      Every technical posting gets a semantic embedding, which is how “similar jobs” finds
      postings that mean the same thing in different words. This is how much of the catalogue
      has one right now — live.
    </p>
  </header>

  <EmbeddingProgress initial={data.progress} streamUrl={api.embedProgressStreamUrl()} />
</div>
