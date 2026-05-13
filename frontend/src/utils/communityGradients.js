/*
 * Curated gradient palette for community theming. Owners pick one of
 * these in CommunitySettings → the slug is stored on the community
 * row (theme_gradient), and CommunityPage resolves it back through
 * gradientForSlug() to a real CSS background.
 *
 * The slug is the wire contract — adding a NEW gradient here is a
 * frontend-only change, but the slug also has to be added to the
 * server-side allow-list (api/controllers/community_connect_handler.go
 * — allowedThemeGradients). Renaming a slug is a breaking change
 * (existing rows would suddenly resolve to "no theme"); add a new
 * one + migrate if you ever need to retire an old slug.
 *
 * Default is 'stardust' — same warm 3-stop sweep used in the brand
 * wordmark and the rest of the app's accent system. The remaining 9
 * cover cool / warm / neutral so every owner finds something that
 * matches their community's vibe.
 */

export const DEFAULT_GRADIENT_SLUG = 'stardust';

export const COMMUNITY_GRADIENTS = [
  { id: 'stardust', name: 'Stardust',  css: 'linear-gradient(120deg, #ff9a6c 0%, #ff6aa6 60%, #a47bff 100%)' },
  { id: 'sunset',   name: 'Sunset',    css: 'linear-gradient(120deg, #ff8a3d 0%, #ff5a1f 100%)' },
  { id: 'ocean',    name: 'Ocean',     css: 'linear-gradient(120deg, #0ea5e9 0%, #2563eb 60%, #6366f1 100%)' },
  { id: 'mint',     name: 'Mint',      css: 'linear-gradient(120deg, #34d399 0%, #14b8a6 60%, #0ea5e9 100%)' },
  { id: 'amber',    name: 'Amber',     css: 'linear-gradient(120deg, #fbbf24 0%, #f59e0b 60%, #d97706 100%)' },
  { id: 'magenta',  name: 'Magenta',   css: 'linear-gradient(120deg, #ec4899 0%, #db2777 60%, #a21caf 100%)' },
  { id: 'forest',   name: 'Forest',    css: 'linear-gradient(120deg, #4ade80 0%, #22c55e 60%, #16a34a 100%)' },
  { id: 'plum',     name: 'Plum',      css: 'linear-gradient(120deg, #a855f7 0%, #7c3aed 60%, #4c1d95 100%)' },
  { id: 'rose',     name: 'Rose',      css: 'linear-gradient(120deg, #fb7185 0%, #f43f5e 60%, #be123c 100%)' },
  { id: 'graphite', name: 'Graphite',  css: 'linear-gradient(120deg, #475569 0%, #1e293b 60%, #0f172a 100%)' },
];

/*
 * gradientForSlug — resolve a community's stored theme_gradient slug
 * to a CSS background value. Empty / unknown slug falls back to the
 * stardust default rather than blowing up so old rows + unset rows
 * still render something. Centralising the fallback here means every
 * surface (banner, avatar, share-card hero, etc.) renders the same
 * thing for the same input.
 */
export const gradientForSlug = (slug) => {
  const match = COMMUNITY_GRADIENTS.find((g) => g.id === slug);
  if (match) return match.css;
  const fallback = COMMUNITY_GRADIENTS.find((g) => g.id === DEFAULT_GRADIENT_SLUG);
  return fallback ? fallback.css : COMMUNITY_GRADIENTS[0].css;
};
