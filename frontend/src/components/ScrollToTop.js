import { useEffect } from 'react';
import { useLocation, useNavigationType } from 'react-router-dom';

/*
 * ScrollToTop — hybrid scroll handler for SPA navigation.
 *
 * - Forward navigation (clicking a <Link>, programmatic navigate() in
 *   PUSH/REPLACE mode): scroll resets to (0, 0). New page = fresh top,
 *   which is what users expect.
 * - Back/forward via browser history (navigationType === "POP"): we
 *   leave scroll alone. The browser natively restores the previous
 *   position, and that's the convention every major web app follows
 *   (Twitter, YouTube, GitHub, etc). Forcing a top-reset on back nav
 *   would break the "tap a feed item, hit back, land on the same
 *   item" pattern users rely on.
 *
 * Renders nothing — it's a side-effect-only component. Mount once
 * inside the Router (so the location hook is available) but outside
 * any route-level <AnimatePresence> so it doesn't get torn down on
 * every route change.
 */
const ScrollToTop = () => {
  const { pathname } = useLocation();
  const navigationType = useNavigationType();

  useEffect(() => {
    if (navigationType !== 'POP') {
      window.scrollTo(0, 0);
    }
  }, [pathname, navigationType]);

  return null;
};

export default ScrollToTop;
