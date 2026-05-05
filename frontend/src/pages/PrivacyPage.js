import React from 'react';
import LegalDocumentPage from '../components/LegalDocumentPage';

/*
 * /privacy — placeholder Privacy Policy.
 *
 * Section list mirrors the App Store / Play Store data-declaration
 * categories so final copy can be slotted into the existing structure
 * without re-shuffling headings. Each section body is a one-line
 * italic stub — visual + textual signal that this is v0.
 */

const PLACEHOLDER = 'This section is a placeholder. Final text is under review.';

// Analytics disclosure is a real (non-placeholder) section so the
// data-collection notice is honest from the moment PostHog turns on.
// Full legal pass on the rest of the policy is a separate concern.
const ANALYTICS_BODY =
  'We use PostHog to understand how the app is used — pages visited, ' +
  'votes cast, and conversion to signup. We do not sell your data and ' +
  'we do not share it with advertisers. PostHog respects the Do Not ' +
  'Track header; if your browser sends DNT, we do not track you. We ' +
  'store analytics state in localStorage rather than third-party ' +
  'cookies.';

const SECTIONS = [
  { heading: 'Data we collect', body: PLACEHOLDER },
  { heading: 'How we use your data', body: PLACEHOLDER },
  { heading: 'Analytics', body: ANALYTICS_BODY },
  { heading: 'Sharing & third parties', body: PLACEHOLDER },
  { heading: 'Retention', body: PLACEHOLDER },
  { heading: 'Your rights', body: PLACEHOLDER },
  { heading: "Children's privacy", body: PLACEHOLDER },
  { heading: 'Contact', body: PLACEHOLDER },
];

const PrivacyPage = () => (
  <LegalDocumentPage
    title="Privacy Policy"
    lastUpdated="under review"
    sections={SECTIONS}
    otherDocPath="/terms"
    otherDocLabel="Terms of Service →"
  />
);

export default PrivacyPage;
