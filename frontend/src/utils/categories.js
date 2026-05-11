// Canonical category vocabulary for the home feed and matchup creation.
//
// Three places consume this list and they MUST stay in lockstep —
// otherwise a chip rendered on a card won't match a sidebar filter,
// or a tag the creator selects won't show up under any category:
//
//   • HomeSidebar.js — the left rail's filter list.
//   • HomeCard.js → deriveTags() — the keyword-rule output (and the
//     "Other" catch-all when no rule fires).
//   • CreateMatchup.js — the preset chips offered to a creator.
//
// Order mirrors the sidebar's display order. "All Categories" is the
// cleared-filter sentinel and is intentionally excluded from the
// SELECTABLE list so it never appears as a creation chip.

export const CATEGORIES = [
  'All Categories',
  'Anime',
  'Manga',
  'Gaming',
  'Music',
  'Movies',
  'TV Shows',
  'K-Pop',
  'Sports',
  'Food',
  'Pokémon',
  'Cartoons',
  'Animals',
  'Celebrities',
  'Random',
  'Other',
];

// SELECTABLE_CATEGORIES is what creators pick from. "All Categories"
// is a filter mode, not a real tag — surfacing it as a chip would let
// users save matchups whose tag is the literal string "All Categories",
// which would then never match any sidebar filter.
export const SELECTABLE_CATEGORIES = CATEGORIES.filter(
  (c) => c !== 'All Categories',
);
