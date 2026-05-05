import React from 'react';
import LegalDocumentPage from '../components/LegalDocumentPage';

/*
 * /terms — placeholder Terms of Service.
 *
 * Section list matches the common SaaS/UGC clause structure App Store
 * reviewers expect to see: accounts, acceptable use, user-content
 * ownership, termination, disclaimers, governing law, contact.
 * Final copy replaces per-section bodies when legal ships it.
 */

const PLACEHOLDER = 'This section is a placeholder. Final text is under review.';

const SECTIONS = [
  { heading: 'Accounts', body: PLACEHOLDER },
  { heading: 'Acceptable use', body: PLACEHOLDER },
  { heading: 'User content ownership', body: PLACEHOLDER },
  { heading: 'Termination', body: PLACEHOLDER },
  { heading: 'Disclaimers & limitation of liability', body: PLACEHOLDER },
  { heading: 'Governing law', body: PLACEHOLDER },
  { heading: 'Contact', body: PLACEHOLDER },
];

const TermsPage = () => (
  <LegalDocumentPage
    title="Terms of Service"
    lastUpdated="under review"
    sections={SECTIONS}
    otherDocPath="/privacy"
    otherDocLabel="Privacy Policy →"
  />
);

export default TermsPage;
