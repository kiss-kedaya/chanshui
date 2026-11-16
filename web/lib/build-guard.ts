// A page opened before a deploy keeps the chunk names it was built with. If
// the server now asks for a chunk the page never loaded, the bundle is old.
export function staleChunks(loaded: string[], fresh: string[]): boolean {
  const have = new Set(loaded);
  return fresh.some((src) => !have.has(src));
}