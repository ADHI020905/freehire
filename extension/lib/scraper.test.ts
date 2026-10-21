import { describe, it, expect, beforeEach } from 'vitest';
import { extractSnapshot, extractHeadline, hasJobPostingMarkup } from './scraper';

function reset() {
  document.title = '';
  document.head.replaceChildren();
  document.body.replaceChildren();
}

function addOgTitle(content: string) {
  const meta = document.createElement('meta');
  meta.setAttribute('property', 'og:title');
  meta.setAttribute('content', content);
  document.head.append(meta);
}

function addHeading(text: string) {
  const h1 = document.createElement('h1');
  h1.textContent = text;
  document.body.append(h1);
}

function addParagraph(text: string) {
  const p = document.createElement('p');
  p.textContent = text;
  document.body.append(p);
}

describe('extractHeadline', () => {
  beforeEach(reset);

  it('prefers og:title over the h1', () => {
    addOgTitle('Senior Go Engineer');
    addHeading('Some page heading');
    expect(extractHeadline(document)).toBe('Senior Go Engineer');
  });

  it('falls back to the first h1 when no og:title', () => {
    addHeading('  Backend Developer  ');
    addHeading('Ignored');
    expect(extractHeadline(document)).toBe('Backend Developer');
  });

  it('falls back to the document title when no og:title or h1', () => {
    document.title = 'Careers — Acme';
    addParagraph('no headings here');
    expect(extractHeadline(document)).toBe('Careers — Acme');
  });
});

describe('extractSnapshot', () => {
  beforeEach(reset);

  it('captures title, headline and collapsed visible text', () => {
    document.title = 'Job Page';
    addHeading('Platform Engineer');
    addParagraph('We   are\n\nhiring.');

    const snapshot = extractSnapshot(document);

    expect(snapshot.title).toBe('Job Page');
    expect(snapshot.headline).toBe('Platform Engineer');
    expect(snapshot.text).toContain('Platform Engineer');
    expect(snapshot.text).toContain('We are hiring.');
    // whitespace is collapsed, not preserved verbatim
    expect(snapshot.text).not.toContain('  ');
  });
});

function addLd(json: string) {
  const script = document.createElement('script');
  script.type = 'application/ld+json';
  script.textContent = json;
  document.head.append(script);
}

describe('hasJobPostingMarkup', () => {
  beforeEach(reset);

  it('finds a top-level JobPosting', () => {
    addLd('{"@context":"https://schema.org","@type":"JobPosting","title":"Go Engineer"}');
    expect(hasJobPostingMarkup(document)).toBe(true);
  });

  it('finds one under @graph, in a list, or as a type array', () => {
    addLd('{"@graph":[{"@type":"WebPage"},{"@type":"JobPosting"}]}');
    expect(hasJobPostingMarkup(document)).toBe(true);
    reset();
    addLd('[{"@type":"Organization"},{"@type":["Thing","JobPosting"]}]');
    expect(hasJobPostingMarkup(document)).toBe(true);
  });

  it('finds one nested inside another node, and in IRI form', () => {
    addLd('{"@type":"WebPage","mainEntity":{"@type":"schema:JobPosting"}}');
    expect(hasJobPostingMarkup(document)).toBe(true);
    reset();
    addLd('{"@type":"http://schema.org/JobPosting"}');
    expect(hasJobPostingMarkup(document)).toBe(true);
  });

  it('ignores other types, malformed blocks and non-JSON-LD scripts', () => {
    addLd('{"@type":"Organization","name":"Acme"}');
    addLd('{not json');
    const plain = document.createElement('script');
    plain.type = 'application/json';
    plain.textContent = '{"@type":"JobPosting"}';
    document.head.append(plain);
    expect(hasJobPostingMarkup(document)).toBe(false);
  });

  it('does not mistake a type that merely contains the word', () => {
    addLd('{"@type":"JobPostingCollection"}');
    expect(hasJobPostingMarkup(document)).toBe(false);
  });

  it('is false for a page with no JSON-LD', () => {
    addParagraph('Just a page');
    expect(hasJobPostingMarkup(document)).toBe(false);
  });
});
