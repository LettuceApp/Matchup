import React, { useEffect, useState } from 'react';
import { FiMoon, FiSun } from 'react-icons/fi';
import { THEMES, getTheme, toggleTheme } from '../utils/theme';

// ThemeToggleItem — drop-in <button> meant to live as one row inside
// an existing dropdown panel (the avatar menu in NavigationBar, the
// profile-pic menu on HomePage). Renders the *opposite* theme as its
// label since clicking it switches; the icon hints at the destination
// state. Inherits the host menu's spacing and color rules so we don't
// have to re-style it per surface.
//
// Visual chrome lives entirely in the host panel's CSS — pass the
// same className the menu uses for its other items (e.g.
// "navigation-bar__profile-item") and this row will match.
const ThemeToggleItem = ({ className, onAfterToggle }) => {
  // Track the current theme in component state so the label flips
  // immediately on click (toggleTheme writes the DOM attribute, but
  // React doesn't know to re-render this component without state).
  const [theme, setLocalTheme] = useState(() => getTheme());
  // If another surface (e.g. the other menu) toggles the theme,
  // refresh our copy when this component remounts. Cheap belt-and-
  // suspenders against the menus showing different labels.
  useEffect(() => {
    setLocalTheme(getTheme());
  }, []);

  const handleClick = () => {
    const next = toggleTheme();
    setLocalTheme(next);
    if (typeof onAfterToggle === 'function') onAfterToggle(next);
  };

  const isDark = theme === THEMES.DARK;
  const Icon = isDark ? FiSun : FiMoon;
  const label = isDark ? 'Light mode' : 'Dark mode';

  return (
    <button
      type="button"
      className={className}
      role="menuitem"
      onClick={handleClick}
      aria-label={`Switch to ${label.toLowerCase()}`}
    >
      <Icon aria-hidden="true" />
      <span>{label}</span>
    </button>
  );
};

export default ThemeToggleItem;
