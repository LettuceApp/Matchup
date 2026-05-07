#!/usr/bin/env python3
"""Generate the Progress-Chip PDF report.

Single-commit cycle (548f3ba) that adds a "Match X of Y · Round N"
chip to bracket matchup pages. Pure frontend, no backend changes.
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

OUTPUT = "/Users/cj/Documents/Projects/go-workspace/Matchup/docs/reports/matchup-bracket-progress-chip.pdf"

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
    story.append(Paragraph("Bracket-Match Progress Chip — Report",
                           styles["RH2"]))
    story.append(Paragraph(f"Date: {date.today().strftime('%B %d, %Y')}",
                           styles["RSmall"]))
    story.append(Paragraph(
        "First of three cycles working through the UX backlog from the "
        "bracket-research pass. Adds a \"Match X of Y · Round N\" chip "
        "above the contender cards on every bracket matchup so users can "
        "see where they are in the sequence. Pure frontend — no backend, "
        "no proto, no DB.",
        styles["RSmall"]))
    story.append(hr())

    # ── Section 1: Why ───────────────────────────────────
    story.append(Paragraph("1. Why", styles["RH2"]))
    story.append(Paragraph(
        "The pairwise-comparison literature browser-Claude pulled in is consistent on this: surfacing "
        "progress reduces abandonment on multi-vote flows. \"Match 1 of 2\" tells the user this is the "
        "first of two votes in this round — implicit promise that the flow is short, plus a sense of "
        "completion when they get to \"Match 2 of 2\". Without it, every bracket matchup looks like it "
        "could be the only one, which is disorienting on entry from a deep link or push notification.",
        styles["RBody"]))
    story.append(Paragraph(
        "Smallest cycle of the three UX-backlog items because no schema decision is required — the "
        "matchups list for a bracket is already exposed via the existing "
        "<font face='Courier'>GetBracketMatchups</font> RPC. The chip is purely a derived value.",
        styles["RBody"]))

    # ── Section 2: Implementation ────────────────────────
    story.append(Paragraph("2. Implementation", styles["RH2"]))

    story.append(Paragraph("Data load", styles["RH3"]))
    story.append(Paragraph(
        "<font face='Courier'>MatchupPage.refreshMatchup</font> already fires "
        "<font face='Courier'>getBracket(bracket_id)</font> when the matchup is part of a bracket. "
        "Added a parallel <font face='Courier'>getBracketMatchups(bracket_id)</font> via "
        "<font face='Courier'>Promise.all</font> — same round-trip, no waterfall. The matchups list "
        "lands in a new <font face='Courier'>bracketMatchups</font> piece of state.",
        styles["RBody"]))

    story.append(Paragraph("Computation", styles["RH3"]))
    story.append(Paragraph(
        "Memoized in a <font face='Courier'>useMemo</font> keyed on the bracketMatchups list, the "
        "isBracketMatchup flag, the matchupRound, and the matchup id. Filter to matchups in the same "
        "round; sort by seed (then id as tiebreak) — same ordering BracketView uses, so the chip's "
        "position number lines up with the visual position in the bracket. Find the current matchup's "
        "index in the sorted list. Return null if the bracket data hasn't loaded yet, or if the "
        "matchup isn't in a bracket.", styles["RBody"]))

    story.append(Paragraph("\"Final\" detection", styles["RH3"]))
    story.append(Paragraph(
        "When the current round has exactly one match AND that round equals the maximum round across "
        "all loaded matchups, the chip says \"Final\" instead of \"Round N\". For a 4-entry bracket: "
        "Round 1 is \"Round 1\", Round 2 is \"Final\". For 8-entry: \"Final\" only on Round 3. Reads "
        "more naturally than \"Round 2\" for the championship match.", styles["RBody"]))

    story.append(Paragraph("Render", styles["RH3"]))
    story.append(Paragraph(
        "Inserted at the top of <font face='Courier'>.matchup-vote-stage</font>, above the anon "
        "counter, above the contender cards. Three spans: position number, separator dot, round label. "
        "<font face='Courier'>aria-live=\"polite\"</font> + a flat aria-label so screen readers get a "
        "clean read of the whole status. Hidden via the <font face='Courier'>matchProgress &amp;&amp; …</font> "
        "guard for non-bracket matchups.", styles["RBody"]))

    story.append(Paragraph("Styling", styles["RH3"]))
    story.append(Paragraph(
        "<font face='Courier'>display: flex</font> with <font face='Courier'>width: fit-content</font> "
        "+ <font face='Courier'>margin: 0 auto</font> centers the chip without forcing parent "
        "text-align hacks. Same pill shape as the existing "
        "<font face='Courier'>.matchup-status-pill</font> family but tinted muted indigo / amber to "
        "read as informational rather than CTA. Round label gets a subtle amber accent so it stands "
        "out from the position number.", styles["RBody"]))

    # ── Section 3: Diff ──────────────────────────────────
    story.append(Paragraph("3. Diff Summary", styles["RH2"]))
    diff = [
        [tc("File"), tc("Change")],
        [tc("frontend/src/pages/MatchupPage.js"),
         tc("Import getBracketMatchups + useMemo. New bracketMatchups state. "
            "Promise.all in refreshMatchup. matchProgress useMemo. Render chip above anon counter.")],
        [tc("frontend/src/styles/MatchupPage.css"),
         tc("New .matchup-progress-chip rule + __sep + __round modifiers.")],
    ]
    story.append(make_table(diff[0], diff[1:], col_widths=[2.5*inch, 4.0*inch]))
    story.append(Spacer(1, 4))
    story.append(Paragraph(
        "Single commit <font face='Courier'>548f3ba</font>. +113 / −2. No new dependencies, no "
        "migrations, no env vars.", styles["RBody"]))

    # ── Section 4: Verification ──────────────────────────
    story.append(Paragraph("4. Verification", styles["RH2"]))
    story.append(bullet("ESLint clean (--max-warnings 0)."))
    story.append(bullet("All 43 frontend tests pass."))
    story.append(bullet("npm run build succeeds, bundle delta tiny."))
    story.append(bullet("Pre-push hook fired and validated the full battery before allowing the push."))

    # ── Section 5: Project state ─────────────────────────
    story.append(Paragraph("5. Project State After This Cycle", styles["RH2"]))
    state = [
        [tc("HEAD on main"), tc("548f3ba — \"Add bracket-match progress chip on MatchupPage\"")],
        [tc("UX backlog progress"), tc("1 of 3 done. Skip / can't-decide button (6b) and richer cards "
                                       "with thumbnails (6c) still pending.")],
        [tc("Schema state"), tc("Unchanged — 24 migrations. Chip is purely a derived value from "
                                "existing data.")],
        [tc("Tests"), tc("All Go packages green. 43/43 frontend tests pass. ESLint clean.")],
    ]
    story.append(make_table(["Metric", "Value"], state, col_widths=[2.2*inch, 4.3*inch]))

    story.append(Spacer(1, 10))

    story.append(Paragraph("Coming up", styles["RH3"]))
    story.append(bullet("<b>6b — Skip / can't-decide button.</b> The biggest schema decision left in "
                        "the UX backlog. Two viable paths: extend votes table with a vote_kind enum "
                        "(<font face='Courier'>'pick' / 'skip' / 'tie'</font>), or add a separate skips "
                        "table. Either choice cascades through the Vote RPC, the rate limiter, the "
                        "anon-vote-cap counter, the bracket-tally aggregation. Real cycle. ~2-3 hours."))
    story.append(bullet("<b>6c — Richer cards (item thumbnails).</b> Bracket items don't have an "
                        "<font face='Courier'>image_path</font> column today; matchup items don't either. "
                        "Adding requires a migration plus extending the upload RPC to accept item-scoped "
                        "uploads (currently only matchup cover images flow through it). Bigger scope. "
                        "~3-4 hours."))

    story.append(Spacer(1, 10))
    story.append(hr())
    story.append(Paragraph(
        "<i>Generated by Claude Code. Smallest of the three UX cycles by design — picked first to "
        "verify the cadence (read code, ship, write report) is sustainable as a per-cycle pattern. "
        "It is.</i>",
        styles["RSmall"]))

    doc.build(story)
    print(f"PDF written to {OUTPUT}")


if __name__ == "__main__":
    build()
