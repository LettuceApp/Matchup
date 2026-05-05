import React from 'react';
import { Link } from 'react-router-dom';
import '../styles/LegalDocument.css';

/*
 * LegalDocumentPage — shared scaffold for /privacy + /terms.
 *
 * Today both pages are placeholders: the visible amber banner tells
 * reviewers + users exactly that. Real copy replaces the per-section
 * bodies (a one-line italic stub) when legal ships the final text.
 *
 * Kept deliberately dependency-light so a content update is a pure
 * data change — no framework / markdown wiring. Pass `sections` as
 * a list of { heading, body } and the component handles layout.
 */

const DEFAULT_CONTACT = 'hello@matchup.com';

const LegalDocumentPage = ({
  title,
  lastUpdated,
  contactEmail = DEFAULT_CONTACT,
  sections = [],
  otherDocPath,
  otherDocLabel,
}) => (
  <div className="legal-page">
    <article className="legal-page__content">
      <aside className="legal-page__banner" role="status">
        <strong>This document is under legal review.</strong>{' '}
        The text below is a placeholder. For current terms, contact{' '}
        <a href={`mailto:${contactEmail}`}>{contactEmail}</a>.
      </aside>

      <header className="legal-page__header">
        <h1>{title}</h1>
        {lastUpdated && (
          <p className="legal-page__meta">Last updated: {lastUpdated}</p>
        )}
      </header>

      <div className="legal-page__sections">
        {sections.map(({ heading, body }) => (
          <section key={heading} className="legal-page__section">
            <h2>{heading}</h2>
            <p className="legal-page__placeholder">
              <em>{body}</em>
            </p>
          </section>
        ))}
      </div>

      <footer className="legal-page__footer">
        {otherDocPath && (
          <Link to={otherDocPath}>{otherDocLabel}</Link>
        )}
        <Link to="/">Back to Matchup</Link>
      </footer>
    </article>
  </div>
);

export default LegalDocumentPage;
