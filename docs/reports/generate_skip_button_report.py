#!/usr/bin/env python3
"""Generate the Skip / Can't-Decide PDF report.

Single-commit cycle (bf3f536) — the second of three UX-backlog items
from the bracket-research pass. Adds a server-side skip vote and a
frontend Skip button. Real schema change (migration 025) plus the
matching backend + frontend wiring.
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

OUTPUT = "/Users/cj/Documents/Projects/go-workspace/Matchup/docs/reports/matchup-skip-cant-decide.pdf"

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
    story.append(Paragraph("Skip / Can't-Decide Affordance — Report",
                           styles["RH2"]))
    story.append(Paragraph(f"Date: {date.today().strftime('%B %d, %Y')}",
                           styles["RSmall"]))
    story.append(Paragraph(
        "Second of three cycles working through the UX backlog. Adds a "
        "real schema column for vote kinds, a SkipMatchup Connect-RPC, "
        "an anon-cap filter that excludes skips, and a frontend "
        "Skip button. Largest UX cycle of the three — the only one that "
        "required a migration + a new RPC.",
        styles["RSmall"]))
    story.append(hr())

    # ── Section 1: Why ───────────────────────────────────
    story.append(Paragraph("1. Why a Skip Affordance", styles["RH2"]))
    story.append(Paragraph(
        "Pairwise-comparison literature (OpinionX, User Research Strategist, the Mural / Blaston / "
        "PlayyOn case studies surfaced during the bracket-research pass) is consistent on one finding: "
        "<b>forcing a choice on close pairs makes users abandon the flow</b>. A user who genuinely "
        "can't decide between two options will close the tab rather than guess randomly — and worse, "
        "if they do guess, the resulting vote is noise that pollutes the bracket signal.",
        styles["RBody"]))
    story.append(Paragraph(
        "A Skip affordance solves both: the user keeps moving (they don't need to commit on every "
        "matchup), and the bracket owner gets a measurable signal — \"40% of voters skipped this "
        "round\" tells you the pairing is too close, which is itself useful product data.",
        styles["RBody"]))

    # ── Section 2: Schema decision ───────────────────────
    story.append(Paragraph("2. Schema Decision — Option A: kind Column on matchup_votes", styles["RH2"]))
    story.append(Paragraph(
        "Two candidate designs were considered:", styles["RBody"]))

    sd = [
        [tc("Aspect"), tc("A — kind column on matchup_votes (chosen)"), tc("B — separate matchup_skips table")],
        [tc("Migration"), tc("ADD COLUMN kind + relax NOT NULL on item ref + check constraint + partial index"),
                          tc("CREATE TABLE matchup_skips with FK + indexes")],
        [tc("Skip rows"), tc("Same table; matchup_item_public_id NULL for skips"), tc("Cleanly separated")],
        [tc("Existing queries"), tc("Need WHERE kind = 'pick' added in cap counter + GetUserVotes"),
                                  tc("Untouched")],
        [tc("RPC"), tc("New SkipMatchup; VoteItem extended for skip → pick switch"),
                    tc("New SkipMatchup + new RecordSkip; VoteItem untouched")],
        [tc("Identity / rate limit"), tc("Reuses existing voteIdentity + IP rate limiter"),
                                       tc("New code paths, possibly duplicate logic")],
        [tc("Future evolvability"), tc("Add a third enum value (e.g. 'tie') with one migration"),
                                     tc("Need yet another table for each new kind")],
    ]
    story.append(make_table(sd[0], sd[1:], col_widths=[1.5*inch, 2.6*inch, 2.4*inch]))
    story.append(Spacer(1, 4))
    story.append(Paragraph(
        "Option A reuses more existing infrastructure (identity resolution, rate limiting, the "
        "switch-vote logic, the Redis vote buffer, the per-matchup \"have they already voted?\" "
        "lookup) and makes future evolution cheaper. The cost — touching 5 existing query sites to "
        "add the kind filter — is paid once.",
        styles["RBody"]))

    # ── Section 3: Three calls confirmed ─────────────────
    story.append(Paragraph("3. Three Product Calls Settled Before Implementation", styles["RH2"]))
    calls = [
        [tc("Call"), tc("Decision"), tc("Why")],
        [tc("Schema shape"), tc("A — kind column on matchup_votes"),
                             tc("Reuses existing infrastructure; smallest schema change.")],
        [tc("Skip vs anon cap"), tc("Skip does NOT count toward the 3-vote anon cap"),
                                  tc("The cap nudges anon → signup. Burning votes on \"can't decide\" defeats the point.")],
        [tc("Enum scope"), tc("Just 'pick' and 'skip' for v1 (no 'tie')"),
                            tc("'tie' is a different intent (\"both equal\") and we have zero data on demand.")],
    ]
    story.append(make_table(calls[0], calls[1:], col_widths=[1.7*inch, 2.4*inch, 2.4*inch]))

    # ── Section 4: Backend implementation ────────────────
    story.append(Paragraph("4. Backend Implementation", styles["RH2"]))

    story.append(Paragraph("Migration 025_vote_kind.sql", styles["RH3"]))
    story.append(bullet("<font face='Courier'>ADD COLUMN kind varchar(8) NOT NULL DEFAULT 'pick'</font>. "
                        "Existing rows backfill to 'pick' via the default — non-breaking."))
    story.append(bullet("CHECK constraint <font face='Courier'>kind IN ('pick','skip')</font>."))
    story.append(bullet("<font face='Courier'>ALTER COLUMN matchup_item_public_id DROP NOT NULL</font> + "
                        "a paired check constraint enforcing the invariant: <i>pick rows always have a "
                        "non-null item, skip rows always have null</i>."))
    story.append(bullet("Partial index on <font face='Courier'>(matchup_public_id) WHERE kind='skip'</font> "
                        "for future tally queries (\"what % of votes on this matchup were skips?\"). "
                        "Tiny index since most rows will be picks."))

    story.append(Paragraph("Connect proto + RPC", styles["RH3"]))
    story.append(bullet("New <font face='Courier'>SkipMatchup(SkipMatchupRequest) returns "
                        "(SkipMatchupResponse)</font> on MatchupItemService."))
    story.append(bullet("Request: <font face='Courier'>matchup_id</font> (matchup public_id) + "
                        "optional <font face='Courier'>anon_id</font>."))
    story.append(bullet("Response: <font face='Courier'>already_skipped: bool</font> — lets the "
                        "frontend distinguish \"recorded just now\" from a duplicate request."))

    story.append(Paragraph("Handler logic (vote_connect_handler.go)", styles["RH3"]))
    story.append(Paragraph(
        "Three flows, mirroring the existing VoteItem shape:",
        styles["RBody"]))
    story.append(bullet("<b>No prior vote</b> → INSERT fresh row with kind='skip', null item ref. "
                        "Skips don't trigger the anon-cap check (the cap is for picks)."))
    story.append(bullet("<b>Prior pick</b> → UPDATE the row to kind='skip', null out the item ref, "
                        "decrement the previously-picked item's vote counter (Redis or direct DB)."))
    story.append(bullet("<b>Prior skip</b> → idempotent no-op; return "
                        "<font face='Courier'>already_skipped=true</font> without touching the row."))
    story.append(Paragraph(
        "Same anon gates as VoteItem: bracket matchups stay members-only (anon callers get "
        "PermissionDenied); IP rate limiter applies to anon skips; status / expiry / bracket-round "
        "checks all reuse the same helpers.", styles["RBody"]))

    story.append(Paragraph("Anon-cap filter", styles["RH3"]))
    story.append(bullet("VoteItem's per-anon-id cap counter: "
                        "<font face='Courier'>SELECT COUNT(*) ... WHERE anon_id = $1 AND kind = 'pick'</font>"))
    story.append(bullet("GetAnonVoteStatus query: same filter. The counter chip on the home feed and "
                        "the matchup page now correctly excludes skips from the \"X of 3 free votes "
                        "left\" total."))

    story.append(Paragraph("Reverse path: skip → pick", styles["RH3"]))
    story.append(Paragraph(
        "VoteItem's switch-vote branch was extended to handle the skip → pick case. When the existing "
        "row is a skip (item ref null), picks rewrite to <font face='Courier'>kind='pick'</font> + set "
        "the new item ref. No previous item to decrement. The user's intent flow — \"actually I'll "
        "vote\" — works seamlessly even after they've already skipped.",
        styles["RBody"]))

    story.append(Paragraph("Model + nullable item ref ripple", styles["RH3"]))
    story.append(Paragraph(
        "<font face='Courier'>MatchupVote.MatchupItemPublicID</font> changed from "
        "<font face='Courier'>string</font> to <font face='Courier'>*string</font> to handle the "
        "nullable column. Five call sites updated to nil-handle. New "
        "<font face='Courier'>PickedItemID()</font> helper returns \"\" for skip rows so call sites "
        "that just want a string for proto / JSON don't have to nil-check at every use. "
        "<font face='Courier'>mergeDeviceVotesToUser</font> updated for skip rows: anon skips "
        "reassign user_id without trying to load a non-existent item.",
        styles["RBody"]))

    # ── Section 5: Tests ─────────────────────────────────
    story.append(Paragraph("5. Tests", styles["RH2"]))
    story.append(Paragraph(
        "Six new integration tests in <font face='Courier'>skip_integration_test.go</font> "
        "(<font face='Courier'>//go:build integration</font>). Use the existing test harness "
        "(setupTestDB, seedTestUser, seedTestMatchup, seedTestBracket, connectReq).",
        styles["RBody"]))
    tests = [
        [tc("Test"), tc("Asserts")],
        [tc("AnonInsertsSkipRow"), tc("First skip lands a row with kind='skip' + null item ref. AlreadySkipped=false.")],
        [tc("AnonIdempotent"), tc("Second skip on same matchup → AlreadySkipped=true; still exactly 1 row.")],
        [tc("AnonDoesNotCountTowardCap"), tc("3 picks + 5 skips: all succeed; cap counter sees only the 3 picks.")],
        [tc("AnonRejectsBracketMatchup"), tc("Bracket child + anon → PermissionDenied + 0 rows inserted.")],
        [tc("PickThenSkip_DecrementsItemVotes"), tc("Pick → skip rewrites the row to skip + decrements the item counter to 0.")],
        [tc("SkipThenPick_IncrementsItemVotes"), tc("Skip → pick rewrites back to kind='pick' + sets item ref + increments to 1.")],
    ]
    story.append(make_table(tests[0], tests[1:], col_widths=[2.4*inch, 4.1*inch]))
    story.append(Spacer(1, 4))
    story.append(Paragraph(
        "<b>Pre-existing gap:</b> the integration tests run via "
        "<font face='Courier'>make test-integration</font> but are not currently fired by GitHub Actions "
        "CI (which runs <font face='Courier'>go test ./...</font> without the integration tag). The "
        "pre-push hook also skips them. Wiring a Postgres service into CI to run integration tests on "
        "every push is filed as a follow-up — not new to this cycle.",
        styles["RBody"]))

    # ── Section 6: Frontend ──────────────────────────────
    story.append(Paragraph("6. Frontend", styles["RH2"]))

    story.append(Paragraph("RPC wrapper", styles["RH3"]))
    story.append(bullet("<font face='Courier'>skipMatchup(matchupId)</font> in api.js. Auto-attaches "
                        "<font face='Courier'>anon_id</font> when unauthenticated, mirroring "
                        "<font face='Courier'>incrementMatchupItemVotes</font>."))

    story.append(Paragraph("Skip button", styles["RH3"]))
    story.append(bullet("Mounts inside <font face='Courier'>.matchup-vote-stage</font>, between the "
                        "contender cards and the action bar."))
    story.append(bullet("Default state: ghost pill with muted indigo border, text "
                        "\"<i>Can't decide? Skip this one</i>\"."))
    story.append(bullet("After skip: morphs to a filled amber pill with text \"<i>Skipped</i>\". "
                        "<font face='Courier'>aria-pressed=\"true\"</font> for screen readers."))
    story.append(bullet("Hidden when the matchup is resolved (winner declared) or when an anon viewer "
                        "is on a bracket child (backend would reject anyway)."))
    story.append(bullet("Disabled while a skip RPC is in flight or after a successful skip — "
                        "re-clicking would no-op since the server is already in skip state."))
    story.append(bullet("Skip is reversible client-side: clicking a contender afterwards calls "
                        "VoteItem, which the backend handles atomically as the skip → pick switch."))

    story.append(Paragraph("Analytics", styles["RH3"]))
    story.append(Paragraph(
        "<font face='Courier'>track('matchup_skipped', {...})</font> with properties:",
        styles["RBody"]))
    story.append(bullet("<font face='Courier'>matchup_id</font>"))
    story.append(bullet("<font face='Courier'>bracket: bool</font> — was this a bracket matchup?"))
    story.append(bullet("<font face='Courier'>already_skipped: bool</font> — server reported a "
                        "duplicate (helpful for noise filtering)"))
    story.append(bullet("<font face='Courier'>from_pick: bool</font> — true when the skip rewrites a "
                        "prior pick (signals \"changed mind\" funnel state)"))
    story.append(Paragraph(
        "<font face='Courier'>matchup_skipped</font> joins the existing 13 PostHog events; the funnel "
        "<font face='Courier'>matchup_viewed → vote_cast or matchup_skipped</font> now has both "
        "branches captured.", styles["RBody"]))

    # ── Section 7: Diff ──────────────────────────────────
    story.append(Paragraph("7. Diff Summary", styles["RH2"]))
    diff = [
        [tc("File"), tc("Change")],
        [tc("api/migrations/025_vote_kind.sql"), tc("New migration: kind column + nullable item ref + check + partial index")],
        [tc("api/proto/matchup/v1/matchup.proto"), tc("New SkipMatchup RPC + request/response messages")],
        [tc("api/gen/matchup/v1/...pb.go + connect.go"), tc("Regenerated via buf generate")],
        [tc("api/models/MatchupVote.go"), tc("Item ref → *string, new Kind field, new PickedItemID() helper")],
        [tc("api/controllers/vote_connect_handler.go"), tc("New SkipMatchup handler; existing VoteItem switch-vote path updated; cap queries filter by kind='pick'")],
        [tc("api/controllers/anonymous_vote_merge.go"), tc("Skip rows reassign without item lookup")],
        [tc("api/controllers/proto_mappers.go"), tc("Use PickedItemID() helper for proto mapping")],
        [tc("api/controllers/skip_integration_test.go"), tc("New: 6 integration tests covering anon/auth × pick/skip × bracket combinations")],
        [tc("frontend/src/services/api.js"), tc("New skipMatchup() wrapper")],
        [tc("frontend/src/pages/MatchupPage.js"), tc("Skip state, handleSkip, button render, skip → reset effect")],
        [tc("frontend/src/styles/MatchupPage.css"), tc("New .matchup-skip-row + .matchup-skip-btn rules + --done variant")],
    ]
    story.append(make_table(diff[0], diff[1:], col_widths=[2.6*inch, 3.9*inch]))
    story.append(Spacer(1, 4))
    story.append(Paragraph(
        "Single commit <font face='Courier'>bf3f536</font>. +1047 / −143 across 12 files (heavy on "
        "regenerated protobuf code).", styles["RBody"]))

    # ── Section 8: Verification ──────────────────────────
    story.append(Paragraph("8. Verification", styles["RH2"]))
    story.append(bullet("<font face='Courier'>go vet ./...</font>: clean."))
    story.append(bullet("<font face='Courier'>go test ./...</font>: all packages green (integration "
                        "tests skipped without the tag, as expected)."))
    story.append(bullet("<font face='Courier'>npx eslint src/ --max-warnings 0</font>: clean."))
    story.append(bullet("<font face='Courier'>npm test</font>: 5 suites, 43 tests, all pass."))
    story.append(bullet("<font face='Courier'>npm run build</font>: clean, bundle delta tiny."))
    story.append(bullet("Pre-push hook fired and validated the full battery before allowing the push."))

    # ── Section 9: Project state ─────────────────────────
    story.append(Paragraph("9. Project State After This Cycle", styles["RH2"]))
    state = [
        [tc("HEAD on main"), tc("bf3f536 — \"Add Skip / Can't-decide affordance on matchups (cycle 6b)\"")],
        [tc("Migrations"), tc("25 total (1 added this cycle: 025_vote_kind.sql)")],
        [tc("UX backlog progress"), tc("2 of 3 done. Richer cards (6c) — item thumbnails — still pending.")],
        [tc("New PostHog events"), tc("matchup_skipped (joins the existing 13)")],
        [tc("Tests"), tc("Go: all unit packages green. 6 new integration tests gated behind the integration tag.")],
        [tc("CI runs integration tests?"), tc("No (pre-existing gap; not introduced by this cycle).")],
    ]
    story.append(make_table(["Metric", "Value"], state, col_widths=[2.2*inch, 4.3*inch]))

    story.append(Spacer(1, 10))

    story.append(Paragraph("Coming up", styles["RH3"]))
    story.append(bullet("<b>6c — Richer cards (item thumbnails).</b> Last UX-backlog item. Bracket "
                        "items don't have an image_path column today; matchup items don't either. "
                        "Adding requires a migration plus extending the upload RPC to accept "
                        "item-scoped uploads (currently only matchup cover images flow through it). "
                        "~3-4 hours."))
    story.append(bullet("<b>Wire integration tests into CI.</b> Add a Postgres service container to "
                        "the GitHub Actions workflow + run "
                        "<font face='Courier'>go test -tags integration ./...</font>. Today the "
                        "integration suite (including this cycle's six new tests) only runs when "
                        "someone manually fires <font face='Courier'>make test-integration</font>."))
    story.append(bullet("<b>Skip-rate dashboard.</b> The partial index on "
                        "<font face='Courier'>(matchup_public_id) WHERE kind='skip'</font> is "
                        "ready for tally queries — surfacing \"X% of voters skipped this round\" on "
                        "the bracket page would close the loop on why we built skip in the first "
                        "place."))

    story.append(Spacer(1, 10))
    story.append(hr())
    story.append(Paragraph(
        "<i>Generated by Claude Code. Largest of the three UX-backlog cycles — the only one that "
        "needed a real migration, a new RPC, and new test coverage. The other two cycles (progress "
        "chip done, richer cards pending) are pure frontend or pure upload-flow work.</i>",
        styles["RSmall"]))

    doc.build(story)
    print(f"PDF written to {OUTPUT}")


if __name__ == "__main__":
    build()
