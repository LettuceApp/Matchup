import React, { createContext, useCallback, useContext, useState } from 'react';
import AnonUpgradeModal from '../components/AnonUpgradeModal';
import { track } from '../utils/analytics';

/*
 * AnonUpgradeContext — global state for the "sign up to keep going"
 * modal. Mounting it at the App root + reading via context lets ANY
 * component (a vote button buried inside BracketView, a like icon on
 * a feed card) trigger the upgrade prompt without passing callbacks
 * down through six layers.
 *
 * Contract:
 *   const { promptUpgrade } = useAnonUpgradePrompt();
 *   promptUpgrade('cap');      // 3rd-vote prompt
 *   promptUpgrade('bracket');  // bracket vote attempt
 *   promptUpgrade('like');     // anon clicked the heart
 *   promptUpgrade('comment');  // anon focused the comment box
 *
 * Multiple calls collapse to a single open modal. Closing dismisses
 * for the rest of the page session — the user's seen it, badgering
 * them on every click would feel hostile.
 */

const AnonUpgradeContext = createContext({
  promptUpgrade: () => {},
});

export const AnonUpgradeProvider = ({ children }) => {
  const [reason, setReason] = useState(null);

  const promptUpgrade = useCallback((nextReason) => {
    const finalReason = nextReason || 'cap';
    setReason(finalReason);
    // Funnel marker — fires every time the modal opens, regardless
    // of which callsite triggered it. Lets us compare cap vs bracket
    // vs follow vs like as conversion drivers without instrumenting
    // each callsite individually.
    track('upgrade_prompt_shown', { reason: finalReason });
  }, []);

  const close = useCallback(() => setReason(null), []);

  return (
    <AnonUpgradeContext.Provider value={{ promptUpgrade }}>
      {children}
      {reason && <AnonUpgradeModal reason={reason} onClose={close} />}
    </AnonUpgradeContext.Provider>
  );
};

export const useAnonUpgradePrompt = () => useContext(AnonUpgradeContext);
