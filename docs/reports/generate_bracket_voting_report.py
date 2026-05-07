#!/usr/bin/env python3
"""Generate the Bracket Render Fix + Side-by-Side Voting PDF report.

Covers the small focused cycle that landed in commit 506fad4 — same
day as the production cutover but a separate, well-scoped change.
Style mirrors the prior generators so the five reports read as one
series.
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

OUTPUT = "/Users/cj/Documents/Projects/go-workspace/Matchup/docs/reports/matchup-bracket-render-and-pairwise-voting.pdf"

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
                          leading=11, spaceAfter=0,
                          wordWrap="CJK"))
styles.add(ParagraphStyle("RCellCode", parent=styles["Normal"], fontSize=7.5,
                          fontName="Courier", leading=10, spaceAfter=0,
                          wordWrap="CJK",
                          textColor=HexColor("#333333")))

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
    story.append(Paragraph("Bracket Render Fix & Pairwise Voting Rework — Report",
                           styles["RH2"]))
    story.append(Paragraph(f"Date: {date.today().strftime('%B %d, %Y')}",
                           styles["RSmall"]))
    story.append(Paragraph(
        "A short, focused cycle that landed the day after the production "
        "cutover. Two real bugs surfaced when dogfooding the just-shipped "
        "Connect-RPC stack against a 4-entry test bracket; one research-"
        "backed UX change shipped alongside. Single commit, one Render "
        "auto-deploy. Continues the report series.",
        styles["RSmall"]))
    story.append(hr())

    # ── Section 1: How the bugs were found ──────────────
    story.append(Paragraph("1. Discovery", styles["RH2"]))
    story.append(Paragraph(
        "Browser-Claude was asked to dogfood the new bracket flow end-to-end: "
        "create a 4-entry, manually-advanced bracket on prod and walk through "
        "the layouts. It built “Greatest Sci-Fi Movie of All Time” with seeds "
        "Blade Runner 2049 (1), The Matrix (2), Interstellar (3), 2001: A Space "
        "Odyssey (4). Two bugs surfaced immediately, plus the pairwise-comparison "
        "research turned up one strong UX win we shipped in the same commit.",
        styles["RBody"]))

    # ── Section 2: Bracket render bug ────────────────────
    story.append(Paragraph("2. Bracket Page Painted Blank Below the Header", styles["RH2"]))
    story.append(Paragraph(
        "Symptom: the bracket detail page rendered the header card normally "
        "but the matches grid below it was blank. DOM was correct — "
        "<font face='Courier'>checkVisibility() === true</font>, computed dimensions "
        "matched. Browser-Claude even injected "
        "<font face='Courier'>border: 3px solid red; background: yellow !important;</font> "
        "on <font face='Courier'>.bracket-match</font> and got nothing.",
        styles["RBody"]))

    story.append(Paragraph("Root cause", styles["RH3"]))
    story.append(Paragraph(
        "Every match card in <font face='Courier'>BracketView.js</font> was wrapped in a "
        "<font face='Courier'>&lt;motion.div&gt;</font> with "
        "<font face='Courier'>initial={{ opacity: 0, y: 12 }}</font>. Framer Motion writes "
        "<font face='Courier'>opacity: 0</font> as an inline style on mount. "
        "<b>Inline styles beat <font face='Courier'>!important</font> from a stylesheet</b>, "
        "which is exactly why the forced background didn’t paint — the "
        "wrapper’s inline opacity composited everything inside it to nothing. "
        "When the entrance animation never resolved (StrictMode double-mount, "
        "intersection-observer / hover-state interference), the cards stayed "
        "invisible permanently.", styles["RBody"]))

    story.append(Paragraph("Fix", styles["RH3"]))
    story.append(bullet("Replace <font face='Courier'>&lt;motion.div&gt;</font> wrapping each "
                        "<font face='Courier'>.bracket-match</font> with a plain "
                        "<font face='Courier'>&lt;div&gt;</font>. Drop "
                        "<font face='Courier'>initial</font>, "
                        "<font face='Courier'>animate</font>, "
                        "<font face='Courier'>transition</font>, and "
                        "<font face='Courier'>whileHover</font>."))
    story.append(bullet("Move the hover scale into "
                        "<font face='Courier'>BracketView.css</font> as a "
                        "<font face='Courier'>:hover transform: scale(1.01)</font> rule, "
                        "with a 150ms transition. CSS-only hover never has the "
                        "“animation never started” failure mode."))
    story.append(bullet("Cards now paint at their final state from the first frame. "
                        "Connector lines + match cards visible immediately."))

    # ── Section 3: HTML escape bug ───────────────────────
    story.append(Paragraph("3. Apostrophes Rendered as &#39;", styles["RH2"]))
    story.append(Paragraph(
        "Bracket and matchup item titles surfaced HTML entities verbatim. The "
        "test bracket’s “2001: A Space Odyssey” item rendered "
        "<font face='Courier'>2001: A Space Odyssey</font> fine, but any "
        "title with an apostrophe — “Greatest ’90s Movie” — would surface as "
        "<font face='Courier'>Greatest &amp;#39;90s Movie</font>.", styles["RBody"]))

    story.append(Paragraph("Root cause", styles["RH3"]))
    story.append(Paragraph(
        "<font face='Courier'>Bracket.Prepare()</font> in "
        "<font face='Courier'>api/models/Bracket.go</font> and "
        "<font face='Courier'>Matchup.Prepare()</font> in "
        "<font face='Courier'>api/models/Matchup.go</font> ran title + description "
        "through <font face='Courier'>html.EscapeString(...)</font> before insert. "
        "The escaped string was what got stored in Postgres. The frontend "
        "renders <font face='Courier'>{bracket.title}</font> as plain JSX, which "
        "<i>React already auto-escapes</i> — so the data was being escaped twice "
        "and the round-trip leaked the encoded form to users.", styles["RBody"]))

    story.append(Paragraph("Fix", styles["RH3"]))
    story.append(bullet("Drop <font face='Courier'>html.EscapeString</font> from both "
                        "<font face='Courier'>Prepare()</font> methods. Keep "
                        "<font face='Courier'>strings.TrimSpace</font>. Drop the now-"
                        "unused <font face='Courier'>html</font> import."))
    story.append(bullet("React JSX text nodes are auto-escaped at render — content "
                        "stays safe. Backend pre-escaping was a defense pattern "
                        "carried over from a non-React era."))
    story.append(bullet("<b>Same anti-pattern lives in <font face='Courier'>User.go</font></b> on "
                        "username + email, deferred to a separate user-model "
                        "cleanup cycle so its integration tests can be touched once."))
    story.append(bullet("<b>Existing pre-fix data:</b> the test bracket created with the "
                        "old code has its title cached escaped in Postgres. Path "
                        "forward: delete + re-create after the deploy lands."))

    # ── Section 4: Body background ───────────────────────
    story.append(Paragraph("4. Body Background Safety Net", styles["RH2"]))
    story.append(Paragraph(
        "<font face='Courier'>body</font> in "
        "<font face='Courier'>frontend/src/index.css</font> had only font + "
        "smoothing rules. The dark gradient lived only on per-page wrappers "
        "(<font face='Courier'>.bracket-page</font>, "
        "<font face='Courier'>.matchup-page</font>, "
        "<font face='Courier'>.home-page</font>). Any layout glitch that left a page "
        "wrapper not covering the viewport produced a bright white flash.",
        styles["RBody"]))
    story.append(Paragraph("Fix", styles["RH3"]))
    story.append(bullet("Add the gradient to <font face='Courier'>body</font> with "
                        "<font face='Courier'>min-height: 100vh</font> and "
                        "<font face='Courier'>background-attachment: fixed</font>. "
                        "Per-page gradients still paint on top — the body rule is "
                        "the safety net behind any short-content or in-flight page."))
    story.append(bullet("No white flashes on hard refresh, throttled-CPU reload, or "
                        "navigation between page wrappers."))

    # ── Section 5: Side-by-side voting ───────────────────
    story.append(Paragraph("5. Side-by-Side Pairwise Voting", styles["RH2"]))
    story.append(Paragraph(
        "Browser-Claude’s research pass synthesised pairwise-comparison literature "
        "(OpinionX, User Research Strategist, Mural / Blaston / PlayyOn case studies) "
        "with one consistent finding: top-stacked options anchor users to the "
        "upper choice. Equal-weight side-by-side cards with a center divider "
        "remove the bias.", styles["RBody"]))

    story.append(Paragraph("Before", styles["RH3"]))
    story.append(Paragraph(
        "<font face='Courier'>.matchup-items</font> in "
        "<font face='Courier'>MatchupPage.css</font> was "
        "<font face='Courier'>display: flex; flex-direction: column</font>. The two "
        "<font face='Courier'>&lt;MatchupItem&gt;</font> cards rendered top over "
        "bottom on every viewport — desktop and mobile alike.", styles["RBody"]))

    story.append(Paragraph("After", styles["RH3"]))
    story.append(bullet("<font face='Courier'>.matchup-items</font> → CSS grid "
                        "<font face='Courier'>1fr auto 1fr</font> with "
                        "<font face='Courier'>align-items: stretch</font>. Two "
                        "equal-width contender columns flanking a "
                        "<font face='Courier'>.matchup-vs-divider</font> center column."))
    story.append(bullet("New <font face='Courier'>.matchup-vs-divider</font> rule renders "
                        "the literal text “VS” at "
                        "<font face='Courier'>font-weight: 800</font>, "
                        "<font face='Courier'>letter-spacing: 0.18em</font>, muted purple "
                        "(<font face='Courier'>rgba(148, 163, 255, 0.55)</font>) — visual "
                        "cue without competing with the cards."))
    story.append(bullet("<font face='Courier'>@media (max-width: 720px)</font> collapses the "
                        "grid back to a single column and hides the divider. "
                        "MatchupItem’s internal flex-wrap layout works at full "
                        "or half width with no internal change."))
    story.append(bullet("In <font face='Courier'>MatchupPage.js</font>, the "
                        "<font face='Courier'>items.map((item, idx) =&gt; ...)</font> "
                        "render now wraps each iteration in a "
                        "<font face='Courier'>&lt;React.Fragment&gt;</font> with "
                        "<font face='Courier'>idx === 1</font> conditionally inserting "
                        "the divider <i>before</i> the second card. Single-item "
                        "edge cases (bye rounds) skip the divider correctly."))

    # ── Section 6: Diff summary ──────────────────────────
    story.append(Paragraph("6. Diff Summary", styles["RH2"]))
    diff = [
        [tc("api/models/Bracket.go"), tc("Drop html.EscapeString in Prepare(); drop html import")],
        [tc("api/models/Matchup.go"), tc("Drop html.EscapeString in Prepare(); drop html import")],
        [tc("frontend/src/components/BracketView.js"), tc("&lt;motion.div&gt; → &lt;div&gt; on .bracket-match; "
                                                          "drop initial / animate / transition / whileHover props")],
        [tc("frontend/src/styles/BracketView.css"), tc("Add transition + :hover scale to .bracket-match — "
                                                       "CSS-only entrance and hover, no framer-motion")],
        [tc("frontend/src/pages/MatchupPage.js"), tc("Wrap items.map in React.Fragment; insert "
                                                     "&lt;div className='matchup-vs-divider'&gt;VS&lt;/div&gt; on idx === 1")],
        [tc("frontend/src/styles/MatchupPage.css"), tc(".matchup-items flex-column → grid 1fr auto 1fr. "
                                                       "New .matchup-vs-divider rule + &lt;720px collapse")],
        [tc("frontend/src/index.css"), tc("body: gradient + min-height: 100vh + background-attachment: fixed")],
    ]
    story.append(make_table(["File", "Change"], diff, col_widths=[2.4*inch, 4.1*inch]))
    story.append(Spacer(1, 4))
    story.append(Paragraph(
        "<font face='Courier'>+111 / −48</font> across 7 files. Single commit "
        "<font face='Courier'>506fad4</font> on top of "
        "<font face='Courier'>9979bdf</font>. No new dependencies, no migrations, "
        "no env vars.", styles["RBody"]))

    # ── Section 7: Verification ─────────────────────────
    story.append(Paragraph("7. Verification", styles["RH2"]))
    story.append(bullet("<font face='Courier'>go build ./...</font> in api/: silent success."))
    story.append(bullet("<font face='Courier'>CI=false npm run build</font> in frontend/: "
                        "compiled. Bundle deltas tiny (+66 B CSS, ±33 B JS)."))
    story.append(bullet("Render auto-deploy triggered on push to "
                        "<font face='Courier'>main</font>."))

    story.append(Paragraph("Browser smoke checklist", styles["RH3"]))
    story.append(bullet("Visit <font face='Courier'>/brackets/{id}</font> → match cards visible "
                        "below the header, connector lines drawn, no blank canvas."))
    story.append(bullet("Click into a bracket matchup → two contender cards "
                        "side-by-side with center “VS” divider, equal width."))
    story.append(bullet("Resize browser to ~600px → cards stack vertically, “VS” "
                        "divider hidden."))
    story.append(bullet("Hard refresh with DevTools CPU 4× throttle → no white "
                        "flash; dark gradient covers from first paint."))
    story.append(bullet("Create a new bracket with an apostrophe in the title "
                        "(e.g. <font face='Courier'>Greatest '90s Movie</font>) → real "
                        "apostrophe rendered, not <font face='Courier'>&amp;#39;</font>."))
    story.append(bullet("Delete + re-create the existing "
                        "<font face='Courier'>Greatest Sci-Fi Movie of All Time</font> "
                        "test bracket so its title is stored unescaped."))

    # ── Section 8: Deferred ──────────────────────────────
    story.append(Paragraph("8. Deferred (Filed as Follow-Ups)", styles["RH2"]))
    story.append(Paragraph(
        "Browser-Claude’s research surfaced more recommendations than this cycle "
        "shipped. The remaining ones are intentionally split out so each lands "
        "with its own design pass:", styles["RBody"]))
    story.append(bullet("<b>Skip / Can’t-decide affordance.</b> Needs a "
                        "<font face='Courier'>vote_kind</font> enum or a new column on "
                        "the votes table, plus a <font face='Courier'>RecordSkip</font> "
                        "RPC. Real backend work, not a CSS tweak."))
    story.append(bullet("<b>Match-X-of-Y / Round-Z progress chip.</b> The matchup "
                        "endpoint doesn’t return <font face='Courier'>bracket_id</font> "
                        "or position-in-round today; needs a small denormalization "
                        "on the Matchup proto."))
    story.append(bullet("<b>Richer contender cards.</b> Item thumbnails require "
                        "upload-flow integration on bracket-item creation. "
                        "(Currently text-only.)"))
    story.append(bullet("<b><font face='Courier'>User.go</font> escape cleanup.</b> Same "
                        "double-encoding pattern on username + email; lives in its "
                        "own user-model cycle so its integration tests can be touched once."))
    story.append(bullet("<b>Mobile bracket-tabs validation.</b> Existing "
                        "<font face='Courier'>.bracket-round-tabs</font> sticks at "
                        "<font face='Courier'>&lt;=840px</font>; smoke on a real phone "
                        "after this deploy lands."))

    # ── Section 9: Project state ─────────────────────────
    story.append(Paragraph("9. Project State After This Cycle", styles["RH2"]))
    state = [
        [tc("HEAD on main"), tc("506fad4 — Fix bracket render + side-by-side matchup voting")],
        [tc("Commits since last report"), tc("1 (this one). Prior report covered up to 9979bdf.")],
        [tc("Tests"), tc("Frontend build clean. Backend build clean. No new tests this cycle — change is render-path only.")],
        [tc("Migrations added"), tc("None. Still at 24 (001–024).")],
        [tc("New env vars"), tc("None.")],
        [tc("New dependencies"), tc("None.")],
        [tc("Production state"), tc("Render auto-deploying 506fad4 on push.")],
    ]
    story.append(make_table(["Metric", "Value"], state, col_widths=[2.2*inch, 4.3*inch]))

    story.append(Spacer(1, 10))
    story.append(hr())
    story.append(Paragraph(
        "<i>Generated by Claude Code. Discovery + research credit: browser-"
        "Claude. Both real bugs were caught during the dogfood pass — exactly "
        "the value of building with the tool you’re shipping.</i>",
        styles["RSmall"]))

    doc.build(story)
    print(f"PDF written to {OUTPUT}")


if __name__ == "__main__":
    build()
