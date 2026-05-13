import React, { useEffect, useRef, useState } from 'react';
import './CreateMenu.css';

/*
 * CreateMenu — single primary CTA on /home that opens a dropdown
 * with Matchup / Bracket. Replaces the previous pair of `+ Create
 * Matchup` and `+ Create Bracket` buttons that competed for the same
 * primary-CTA slot (Hick's Law violation flagged in the home-topbar
 * cleanup brief).
 *
 * The actual navigation lives in the parent — both because the
 * existing handlers carry the anon-redirect-to-login gate and
 * because the URLs are owner-scoped (the matchup create route uses
 * the viewer's username slug). Parent passes in `onMatchup` and
 * `onBracket`; this component owns only the open/close + outside-
 * click + Escape pattern, matching the avatar/profile menus in
 * NavigationBar and HomePage.
 */
const CreateMenu = ({ onMatchup, onBracket }) => {
  const [open, setOpen] = useState(false);
  const containerRef = useRef(null);
  const triggerRef = useRef(null);

  useEffect(() => {
    if (!open) return undefined;
    const onPointerDown = (e) => {
      if (containerRef.current && !containerRef.current.contains(e.target)) {
        setOpen(false);
      }
    };
    const onKey = (e) => {
      if (e.key === 'Escape') {
        setOpen(false);
        // Return focus to the trigger so keyboard users land back on
        // the visible control rather than at the top of the page.
        triggerRef.current?.focus();
      }
    };
    document.addEventListener('mousedown', onPointerDown);
    document.addEventListener('keydown', onKey);
    return () => {
      document.removeEventListener('mousedown', onPointerDown);
      document.removeEventListener('keydown', onKey);
    };
  }, [open]);

  const choose = (handler) => () => {
    setOpen(false);
    if (typeof handler === 'function') handler();
  };

  return (
    <div className="create-menu" ref={containerRef}>
      <button
        type="button"
        ref={triggerRef}
        className="create-menu__trigger"
        aria-haspopup="menu"
        aria-expanded={open}
        onClick={() => setOpen((v) => !v)}
      >
        + Create <span aria-hidden="true" className="create-menu__caret">▾</span>
      </button>
      {open && (
        <div role="menu" className="create-menu__panel">
          <button
            type="button"
            role="menuitem"
            className="create-menu__item"
            onClick={choose(onMatchup)}
          >
            Matchup
          </button>
          <button
            type="button"
            role="menuitem"
            className="create-menu__item"
            onClick={choose(onBracket)}
          >
            Bracket
          </button>
        </div>
      )}
    </div>
  );
};

export default CreateMenu;
