#!/usr/bin/env python3
"""Generate the User-Escape Cleanup + npm Audit PDF report.

Two small commits batched into one cycle: a74e3af (User.Prepare
escape cleanup, mirrors the prior Bracket/Matchup fix) and 46a80e0
(npm audit fix: 54 -> 26 vulnerabilities). Neither was a feature;
both close out long-running deferred items.
"""

from reportlab.lib.pagesizes import letter
from reportlab.lib.styles import getSampleStyleSheet, ParagraphStyle
from reportlab.lib.units import inch
from reportlab.lib.colors import HexColor
from reportlab.platypus import (
    SimpleDocTemplate, Paragraph, Spacer, Table, TableStyle,
    HRFlowable,
)
from datetime import date

OUTPUT = "/Users/cj/Documents/Projects/go-workspace/Matchup/docs/reports/matchup-user-escape-cleanup-and-npm-audit.pdf"

styles = getSampleStyleSheet()

styles.add(ParagraphStyle("RH1", parent=styles["Heading1"], fontSize=18,
                          spaceAfter=10, textColor=HexColor("#1a1a2e")))
styles.add(ParagraphStyle("RH2", parent=styles["Heading2"], fontSize=14,
                          spaceBefore=14, spaceAfter=6,
                          textColor=HexColor("#16213e")))
styles.add(ParagraphStyle("RH3", parent=styles["Heading3"], fontSize=11,
                          spaceBefore=10, spaceAfter=4,
                          textColor=HexColor("#0f3460")))
styles.add(ParagraphStyle("RBody", parent=styles["Normal"], fontSize=9.5,
                          leading=13, spaceAfter=6))
styles.add(ParagraphStyle("RSmall", parent=styles["Normal"], fontSize=8.5,
                          leading=11, spaceAfter=4, textColor=HexColor("#444444")))
styles.add(ParagraphStyle("RBullet", parent=styles["Normal"], fontSize=9.5,
                          leading=13, leftIndent=20, bulletIndent=10,
                          spaceAfter=3))
styles.add(ParagraphStyle("RCellBody", parent=styles["Normal"], fontSize=8.5,
                          leading=11, spaceAfter=0, wordWrap="CJK"))
styles.add(ParagraphStyle("RCellCode", parent=styles["Normal"], fontSize=7.5,
                          fontName="Courier", leading=10, spaceAfter=0,
                          wordWrap="CJK", textColor=HexColor("#333333")))

LIGHT_BG = HexColor("#f0f0f5")
WHITE = HexColor("#ffffff")
HEADER_BG = HexColor("#1a1a2e")
HEADER_FG = HexColor("#ffffff")


def bullet(text):
    return Paragraph(f"• {text}", styles["RBullet"])


def tc(text, code=False):
    style = styles["RCellCode"] if code else styles["RCellBody"]
    return Paragraph(text, style)


def hr():
    return HRFlowable(width="100%", thickness=0.5, color=HexColor("#cccccc"),
                      spaceBefore=6, spaceAfter=6)


def make_table(headers, rows, col_widths=None):
    data = [headers] + rows
    t = Table(data, colWidths=col_widths, hAlign="LEFT")
    t.setStyle(TableStyle([
        ("BACKGROUND", (0, 0), (-1, 0), HEADER_BG),
        ("TEXTCOLOR", (0, 0), (-1, 0), HEADER_FG),
        ("FONTNAME", (0, 0), (-1, 0), "Helvetica-Bold"),
        ("FONTSIZE", (0, 0), (-1, 0), 9),
        ("FONTSIZE", (0, 1), (-1, -1), 8.5),
        ("ALIGN", (0, 0), (-1, -1), "LEFT"),
        ("VALIGN", (0, 0), (-1, -1), "TOP"),
        ("ROWBACKGROUNDS", (0, 1), (-1, -1), [WHITE, LIGHT_BG]),
        ("GRID", (0, 0), (-1, -1), 0.4, HexColor("#cccccc")),
        ("TOPPADDING", (0, 0), (-1, -1), 4),
        ("BOTTOMPADDING", (0, 0), (-1, -1), 4),
        ("LEFTPADDING", (0, 0), (-1, -1), 6),
        ("RIGHTPADDING", (0, 0), (-1, -1), 6),
    ]))
    return t


def build():
    doc = SimpleDocTemplate(OUTPUT, pagesize=letter,
                            topMargin=0.6*inch, bottomMargin=0.6*inch,
                            leftMargin=0.75*inch, rightMargin=0.75*inch)
    story = []

    story.append(Paragraph("Matchup Platform", styles["RH1"]))
    story.append(Paragraph("Model Escape Cleanup & npm Audit Pass — Report",
                           styles["RH2"]))
    story.append(Paragraph(f"Date: {date.today().strftime('%B %d, %Y')}",
                           styles["RSmall"]))
    story.append(Paragraph(
        "Two small commits closing out long-running deferred items: the "
        "last instance of the html.EscapeString anti-pattern in the model "
        "layer (User.Prepare), and a sweep of the 54 Dependabot-flagged "
        "frontend vulnerabilities. Neither cycle introduced new behavior. "
        "Both push the codebase further toward green.",
        styles["RSmall"]))
    story.append(hr())

    # ── Section 1: User escape cleanup ───────────────────
    story.append(Paragraph("1. User.Prepare Escape Cleanup (commit a74e3af)", styles["RH2"]))
    story.append(Paragraph(
        "The bracket-render cycle (commit <font face='Courier'>506fad4</font>) caught the "
        "<font face='Courier'>html.EscapeString</font> double-encode bug in "
        "<font face='Courier'>Bracket.Prepare</font> and <font face='Courier'>Matchup.Prepare</font>. "
        "<font face='Courier'>User.Prepare</font> had the same anti-pattern on username + email but "
        "was deferred to its own cycle so its tests could be touched in lockstep. Closing it out now "
        "completes the model-layer fix.", styles["RBody"]))

    story.append(Paragraph("Why it’s worth doing even though no user has hit it yet", styles["RH3"]))
    story.append(bullet("React auto-escapes JSX text at render. Backend pre-escape was redundant + "
                        "double-encoding. Same root cause as the bracket / matchup variant."))
    story.append(bullet("Existing prod usernames don’t contain HTML metacharacters today — most users "
                        "are simple alphanumerics — so the bug was latent. But "
                        "<font face='Courier'>O'Brien</font>, "
                        "<font face='Courier'>Renée</font>, etc. would have surfaced as "
                        "<font face='Courier'>&amp;#39;</font>, "
                        "<font face='Courier'>&amp;eacute;</font> in the UI as soon as a user with "
                        "those characters signed up."))
    story.append(bullet("Email apostrophes (RFC 5321 allows them in the local-part of an address) would "
                        "have round-tripped broken the same way."))

    story.append(Paragraph("Diff", styles["RH3"]))
    user_diff = [
        [tc("api/models/User.go"), tc("Drop html.EscapeString from Prepare(); keep TrimSpace + ToLower. "
                                      "Drop unused html import.")],
        [tc("api/models/user_test.go"), tc("Rename SanitizesUsername / SanitizesEmail to TrimsAndLowercases* "
                                           "(old name overpromised — assertion was always just trim+lowercase). "
                                           "Add PreservesApostrophe{InUsername,InEmail} mirroring the "
                                           "bracket/matchup pattern.")],
    ]
    story.append(make_table(["File", "Change"], user_diff, col_widths=[2.0*inch, 4.5*inch]))

    # ── Section 2: npm audit ─────────────────────────────
    story.append(Paragraph("2. npm Audit Pass (commit 46a80e0)", styles["RH2"]))
    story.append(Paragraph(
        "The frontend was carrying <b>54 Dependabot-flagged vulnerabilities</b> (1 critical, 28 high, "
        "12 moderate, 13 low) accumulated over the life of the project. Most are transitive deps "
        "through Create React App. <font face='Courier'>npm audit fix</font> (non-breaking) cleared "
        "<b>28 of the 54</b>, including the one critical.", styles["RBody"]))

    story.append(Paragraph("Before / after by severity", styles["RH3"]))
    audit = [
        [tc("Severity"), tc("Before"), tc("After"), tc("Δ")],
        [tc("Critical"), tc("1"), tc("0"), tc("−1")],
        [tc("High"), tc("28"), tc("14"), tc("−14")],
        [tc("Moderate"), tc("12"), tc("3"), tc("−9")],
        [tc("Low"), tc("13"), tc("9"), tc("−4")],
        [tc("Total"), tc("54"), tc("26"), tc("−28")],
    ]
    story.append(make_table(audit[0], audit[1:],
                            col_widths=[1.5*inch, 1.0*inch, 1.0*inch, 1.0*inch]))
    story.append(Spacer(1, 6))

    story.append(Paragraph("The critical, fixed", styles["RH3"]))
    story.append(Paragraph(
        "<font face='Courier'>form-data</font> was using "
        "<font face='Courier'>Math.random()</font> to choose its multipart boundary instead of a "
        "cryptographically secure RNG. Predictable boundaries enable certain classes of multipart "
        "smuggling attack on parsers that split on the boundary string. "
        "(<font face='Courier'>GHSA-fjxv-7rqg-78g4</font>.) Fix was a transitive bump; no "
        "application code changes.", styles["RBody"]))

    story.append(Paragraph("What’s left and why we’re not chasing it now", styles["RH3"]))
    story.append(Paragraph(
        "All 26 remaining vulns are gated behind <font face='Courier'>npm audit fix --force</font>, "
        "which would install <font face='Courier'>react-scripts@0.0.0</font> — an empty stub package "
        "that signals CRA’s broken / unmaintained upgrade path. The remaining vulnerable packages all "
        "flow through CRA’s frozen dep tree:",
        styles["RBody"]))
    story.append(bullet("<font face='Courier'>webpack-dev-server</font> ≤ 5.2.0 — source-code-stealing "
                        "moderate"))
    story.append(bullet("<font face='Courier'>@svgr/webpack</font> + <font face='Courier'>svgo</font> + "
                        "<font face='Courier'>nth-check</font> + <font face='Courier'>postcss</font> — "
                        "ReDoS / parser quirks (high)"))
    story.append(bullet("<font face='Courier'>@remix-run/router</font> open-redirect XSS — high "
                        "(addressed by upgrading react-router-dom, but interlocked with CRA’s "
                        "ecosystem)"))
    story.append(bullet("<font face='Courier'>jsonpath</font> / <font face='Courier'>underscore</font> "
                        "ReDoS — low to moderate, dev-time only"))
    story.append(Paragraph(
        "Most are dev-time / build-time tools, not runtime-served code, so the actual blast radius for "
        "prod is small. The clean fix is migrating off CRA — Vite is the standard target — but that’s "
        "a real cycle (build config, test runner, env-var substitution differs, asset pipeline "
        "differs). Filed as a follow-up.", styles["RBody"]))

    story.append(Paragraph("Verification", styles["RH3"]))
    story.append(bullet("<font face='Courier'>npm test</font>: 5 suites, 43 tests, all pass."))
    story.append(bullet("<font face='Courier'>npm run build</font>: clean, bundle still ~339 KB main.js."))
    story.append(bullet("Pre-push hook fired and validated the full battery before allowing the push."))

    # ── Section 3: Combined diff ─────────────────────────
    story.append(Paragraph("3. Combined Diff Summary", styles["RH2"]))
    diff = [
        [tc("Commit"), tc("Files"), tc("Net change")],
        [tc("a74e3af"), tc("api/models/User.go, user_test.go"), tc("+41 / −5")],
        [tc("46a80e0"), tc("frontend/package-lock.json"), tc("+734 / −564")],
    ]
    story.append(make_table(diff[0], diff[1:], col_widths=[1.0*inch, 3.5*inch, 2.0*inch]))

    # ── Section 4: Project state ─────────────────────────
    story.append(Paragraph("4. Project State After This Batch", styles["RH2"]))
    state = [
        [tc("HEAD on main"), tc("46a80e0 — \"npm audit fix: 54 -> 26 vulnerabilities (no breaking changes)\"")],
        [tc("Model-layer escapes"), tc("All Prepare() methods (Bracket, Matchup, User) now trim/lowercase "
                                       "without escaping. React auto-escapes at render. Round-trip safe.")],
        [tc("Dependabot count"), tc("54 → 26 (no critical, 14 high, 3 moderate, 9 low). All remaining "
                                    "behind --force; clean fix is CRA-to-Vite migration.")],
        [tc("Tests"), tc("Go: all packages green. Frontend: 43/43 pass.")],
        [tc("Pre-push hook"), tc("Active. Caught nothing this batch (clean changes), but ran successfully "
                                 "on both pushes.")],
    ]
    story.append(make_table(["Metric", "Value"], state, col_widths=[2.2*inch, 4.3*inch]))

    story.append(Spacer(1, 10))

    story.append(Paragraph("Deferred (filed as follow-ups)", styles["RH3"]))
    story.append(bullet("<b>CRA → Vite migration.</b> The path to closing the remaining 26 vulns. "
                        "Real cycle: build config rewrite, test-runner swap, env-var substitution "
                        "differences, asset pipeline. Worth doing once the UX backlog has stabilized."))
    story.append(bullet("<b>Test bracket recreate on prod.</b> Operational — delete + re-create the "
                        "<font face='Courier'>Greatest Sci-Fi Movie of All Time</font> bracket so its "
                        "title round-trips correctly under the new contract. ~30 sec."))
    story.append(bullet("<b>Render env vars for analytics.</b> "
                        "<font face='Courier'>REACT_APP_POSTHOG_KEY</font>, "
                        "<font face='Courier'>REACT_APP_POSTHOG_HOST</font>, "
                        "<font face='Courier'>REACT_APP_SENTRY_DSN</font>, "
                        "<font face='Courier'>SENTRY_DSN</font>. User-action item; needs project tokens "
                        "+ Render dashboard."))
    story.append(bullet("<b>Decommission Dagster Render service</b> + drop the 22 orphan tables in "
                        "matchup_postgres. User stops the service in Render, then the SQL drops are a "
                        "one-liner."))
    story.append(bullet("<b>UX backlog from the bracket research</b> — skip / can’t-decide affordance, "
                        "match-X-of-Y progress chip, richer cards with thumbnails. Each is its own "
                        "real cycle (DB migration + RPC + frontend + tests). Scope to be agreed before "
                        "starting."))

    story.append(Spacer(1, 10))
    story.append(hr())
    story.append(Paragraph(
        "<i>Generated by Claude Code. Two cycles batched into one report because they’re "
        "low-risk hygiene rather than feature work — neither warranted its own deep-dive doc, "
        "but both are worth the paper trail.</i>",
        styles["RSmall"]))

    doc.build(story)
    print(f"PDF written to {OUTPUT}")


if __name__ == "__main__":
    build()
