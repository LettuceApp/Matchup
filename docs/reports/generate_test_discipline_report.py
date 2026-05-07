#!/usr/bin/env python3
"""Generate the Test-Discipline & Pre-Push Hook PDF report.

Backfilled report covering two commits that landed back-to-back as
part of the same investigation thread (e9f90d8 + e2fa7ae): the
Prepare test fix that was supposed to ship with 506fad4 but didn't,
the lint backlog cleanup that brought CI from red to green for the
first time in weeks, and the pre-push hook + Render gate that closes
the "broken code reaches prod before CI flags it" loop.
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

OUTPUT = "/Users/cj/Documents/Projects/go-workspace/Matchup/docs/reports/matchup-test-discipline-and-pre-push-hook.pdf"

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
    story.append(Paragraph("Test Discipline & Pre-Push Hook — Report",
                           styles["RH2"]))
    story.append(Paragraph(f"Date: {date.today().strftime('%B %d, %Y')}",
                           styles["RSmall"]))
    story.append(Paragraph(
        "Two commits, one investigation thread. Discovered mid-cycle that "
        "tests were never running locally before push, that CI had been red "
        "for weeks because of pre-existing lint warnings, and that Render's "
        "auto-deploy didn't wait for CI. Fixed the immediate test breakage, "
        "cleared the lint backlog to green, added a pre-push hook, and (with "
        "the user's help) configured Render to gate on CI. Three layers of "
        "guardrail where there were zero. Backfilled report for completeness "
        "of the series — written after the next cycle (the JWT flake fix) "
        "had already shipped.",
        styles["RSmall"]))
    story.append(hr())

    # ── Section 1: Discovery ─────────────────────────────
    story.append(Paragraph("1. Discovery — \"Are tests being run before we push?\"", styles["RH2"]))
    story.append(Paragraph(
        "After the bracket render fix shipped (commit "
        "<font face='Courier'>506fad4</font>), the user asked the meta "
        "question: <i>\"Are our unit tests being tested before we commit and "
        "push anything?\"</i> The answer was uncomfortable.", styles["RBody"]))

    story.append(Paragraph("What was actually happening", styles["RH3"]))
    state = [
        [tc("Stage"), tc("Before this cycle")],
        [tc("Local pre-commit / pre-push"), tc("Nothing automatic. Claude was running `go build` + `npm run build` "
                                               "(compile-only) before each push — no tests, no lint.")],
        [tc("CI on push to main"), tc("ci.yml runs go vet, golangci-lint, go test -race -coverprofile, "
                                      "ESLint with --max-warnings 0, and npm test. <b>Runs after the push.</b>")],
        [tc("Render auto-deploy"), tc("Triggered on push to main. <b>Did not wait for CI</b> — broken commits "
                                      "could ship to prod before CI flagged them.")],
        [tc("CI status"), tc("Had been <b>failing for weeks</b>. Three pre-existing ESLint errors in "
                             "ActivityFeed.test.js + 12 warnings violated the --max-warnings 0 rule.")],
    ]
    story.append(make_table(state[0], state[1:], col_widths=[1.8*inch, 4.7*inch]))
    story.append(Spacer(1, 4))

    story.append(Paragraph(
        "Running the Go tests for the first time also surfaced two unit-test "
        "regressions <b>shipped in the previous push</b> "
        "(<font face='Courier'>506fad4</font>): the bracket / matchup "
        "<font face='Courier'>Prepare()</font> tests still asserted "
        "<font face='Courier'>html.EscapeString</font> behavior even though "
        "the production code had stopped escaping. CI would have caught "
        "both. The hook would have caught both. Neither was running.",
        styles["RBody"]))

    # ── Section 2: Prepare test regression ────────────────
    story.append(Paragraph("2. Prepare-Test Regression Fix (commit e9f90d8)", styles["RH2"]))
    story.append(Paragraph(
        "Commit <font face='Courier'>506fad4</font> dropped "
        "<font face='Courier'>html.EscapeString</font> from "
        "<font face='Courier'>Bracket.Prepare()</font> and "
        "<font face='Courier'>Matchup.Prepare()</font>. The corresponding "
        "tests (<font face='Courier'>TestBracketPrepare_SanitizesTitle</font>, "
        "<font face='Courier'>TestMatchupPrepare_SanitizesTitle</font>) still "
        "asserted that <font face='Courier'>&lt;b&gt;Title&lt;/b&gt;</font> "
        "should round-trip to <font face='Courier'>&amp;lt;b&amp;gt;Title&amp;lt;/b&amp;gt;</font>. "
        "Both tests started failing immediately when the test suite ran for "
        "the first time.", styles["RBody"]))
    story.append(Paragraph("Fix", styles["RH3"]))
    story.append(bullet("Renamed both tests to "
                        "<font face='Courier'>TestBracketPrepare_TrimsTitle</font> / "
                        "<font face='Courier'>TestMatchupPrepare_TrimsTitle</font> — accurate description "
                        "of what <font face='Courier'>Prepare()</font> now does."))
    story.append(bullet("Updated the expected output: "
                        "<font face='Courier'>\"  &lt;b&gt;Title&lt;/b&gt;  \"</font> → "
                        "<font face='Courier'>\"&lt;b&gt;Title&lt;/b&gt;\"</font> (just trim, no escape)."))
    story.append(bullet("Added <font face='Courier'>TestBracketPrepare_PreservesApostrophe</font> + "
                        "the matchup mirror — locks in the round-trip behavior for the case that "
                        "surfaced the bug in the first place "
                        "(\"<font face='Courier'>Greatest '90s Movie</font>\" → "
                        "<font face='Courier'>'</font> not <font face='Courier'>&amp;#39;</font>)."))

    # ── Section 3: Lint backlog ──────────────────────────
    story.append(Paragraph("3. Lint Backlog Cleared (commit e2fa7ae)", styles["RH2"]))
    story.append(Paragraph(
        "<font face='Courier'>npx eslint src/ --max-warnings 0</font> reported <b>3 errors + 12 "
        "warnings</b> across 7 files. None blocked runtime functionality, but the "
        "<font face='Courier'>--max-warnings 0</font> rule meant the CI lint job had been red on "
        "every recent push. Cleared all 15 to zero in one pass.", styles["RBody"]))

    story.append(Paragraph("3 errors", styles["RH3"]))
    story.append(bullet("<font face='Courier'>__tests__/ActivityFeed.test.js:151</font> — three "
                        "<font face='Courier'>testing-library/no-container</font> + "
                        "<font face='Courier'>no-node-access</font> errors on a "
                        "<font face='Courier'>container.querySelectorAll('.activity-item.is-unread')</font> "
                        "assertion. The “unread” state is a CSS-class-driven visual cue with no accessible "
                        "role / label yet, so a class-selector query is the most direct assertion. Resolved "
                        "with a scoped <font face='Courier'>eslint-disable</font> + a one-line comment "
                        "explaining why."))

    story.append(Paragraph("12 warnings — including a real UX bug", styles["RH3"]))
    story.append(bullet("<b><font face='Courier'>LoginPage.js</font> "
                        "<font face='Courier'>'from' is assigned a value but never used</font>.</b> The "
                        "redirect-after-login flow defined a "
                        "<font face='Courier'>from = location.state?.from?.pathname || '/home'</font> but "
                        "the <font face='Courier'>navigate(...)</font> call passed "
                        "<font face='Courier'>'/home'</font> verbatim. <b>RequireAuth</b> stuffs the original "
                        "URL into <font face='Courier'>location.state.from</font> for users bounced to "
                        "login from a gated page — and we were ignoring it. Wired up. ESLint caught a real "
                        "UX regression."))
    story.append(bullet("<font face='Courier'>useCountdown.js</font> — wrapped "
                        "<font face='Courier'>getRemaining</font> in "
                        "<font face='Courier'>useCallback([targetTime])</font> so the effect can list it as "
                        "a dep without re-firing every render."))
    story.append(bullet("<font face='Courier'>MatchupCard.js</font> — wrapped "
                        "<font face='Courier'>items</font> in "
                        "<font face='Courier'>useMemo</font> so "
                        "<font face='Courier'>totalVotes</font>’ <font face='Courier'>useMemo</font> deps "
                        "are stable when <font face='Courier'>matchup.items</font> hasn’t changed."))
    story.append(bullet("<font face='Courier'>RegisterPage.js</font> — dropped unused "
                        "<font face='Courier'>from</font> + <font face='Courier'>useLocation</font> import. "
                        "(Signup redirects to <font face='Courier'>dest</font> = the new user’s profile, "
                        "not a <font face='Courier'>from</font> location.)"))
    story.append(bullet("<font face='Courier'>MatchupPage.js</font> — dropped unused "
                        "<font face='Courier'>FiShare2</font>, "
                        "<font face='Courier'>updateBracket</font>, "
                        "<font face='Courier'>showBracketActivate</font>, "
                        "<font face='Courier'>canManageBracket</font>, "
                        "<font face='Courier'>interactionLocked</font>. Mostly dead code from earlier "
                        "iterations."))
    story.append(bullet("<font face='Courier'>UserProfile.js</font> — dropped unused "
                        "<font face='Courier'>FiActivity</font> import + "
                        "<font face='Courier'>followersLoaded</font> state (only ever set, never read). "
                        "Added scoped <font face='Courier'>eslint-disable</font> on the two follow-list "
                        "useEffects with comments explaining why "
                        "<font face='Courier'>loadFollowers</font> / "
                        "<font face='Courier'>loadFollowing</font> aren’t deps (they read pagination state "
                        "they themselves write — listing them would create a feedback loop)."))

    story.append(Paragraph(
        "Final state: <font face='Courier'>npx eslint src/ --max-warnings 0</font> exits 0. CI lint job "
        "green for the first time in weeks.", styles["RBody"]))

    # ── Section 4: Pre-push hook ─────────────────────────
    story.append(Paragraph("4. The Pre-Push Hook (commit e2fa7ae)", styles["RH2"]))
    story.append(Paragraph(
        "Local seatbelt that runs the same battery CI runs before letting "
        "the push leave the machine. Stops the regression class where Claude "
        "or a human pushes a commit that breaks tests / lint and only finds "
        "out after Render has already auto-deployed it.", styles["RBody"]))

    story.append(Paragraph("Design", styles["RH3"]))
    story.append(bullet("<b><font face='Courier'>.githooks/pre-push</font></b> — tracked shell script "
                        "checked into the repo. Runs "
                        "<font face='Courier'>go vet</font>, "
                        "<font face='Courier'>go test -count=1</font>, "
                        "<font face='Courier'>npx eslint --max-warnings 0</font>, "
                        "<font face='Courier'>CI=true npm test --watchAll=false</font> in sequence. "
                        "Exits non-zero on any failure → push is blocked."))
    story.append(bullet("Uses <font face='Courier'>set -e</font> + per-step echo so the failure point "
                        "is obvious. Falls back to <font face='Courier'>/usr/local/go/bin/go</font> if "
                        "<font face='Courier'>go</font> isn’t in PATH (relevant when GUI git tools "
                        "invoke the hook from a non-interactive shell)."))
    story.append(bullet("Skips race detector + coverage for speed — CI runs the slower full battery. "
                        "Hook adds about 30 seconds per push."))
    story.append(bullet("<b>Bypass</b> with <font face='Courier'>git push --no-verify</font> — "
                        "intentional escape hatch for emergency rollbacks where you know the "
                        "diagnostic is unrelated."))

    story.append(Paragraph("Setup", styles["RH3"]))
    story.append(bullet("New <font face='Courier'>make setup-hooks</font> target runs "
                        "<font face='Courier'>git config core.hooksPath .githooks</font>. The config is "
                        "per-clone (not in the repo), so a fresh clone needs to run it once."))
    story.append(bullet("README updated under <b>Testing</b> with a “Pre-push hook (recommended)” "
                        "subsection documenting the setup + bypass syntax."))

    story.append(Paragraph("Validation on first push", styles["RH3"]))
    story.append(Paragraph(
        "The hook fired on its very first push (the same commit that introduced it). It ran the full "
        "battery, returned <font face='Courier'>✓ pre-push checks passed</font>, and let the push "
        "proceed. End-to-end smoke test of the seatbelt happened automatically.",
        styles["RBody"]))

    # ── Section 5: Render gating ─────────────────────────
    story.append(Paragraph("5. Render Auto-Deploy Gated on CI (operational)", styles["RH2"]))
    story.append(Paragraph(
        "User followed up by configuring both Render services "
        "(<font face='Courier'>matchup-backend</font>, "
        "<font face='Courier'>matchup-frontend</font>) to wait for CI status checks before "
        "auto-deploying. Render UI offers three levels: <font face='Courier'>On Commit</font>, "
        "<font face='Courier'>After CI Checks Pass</font>, "
        "<font face='Courier'>Off</font>. Both flipped to <font face='Courier'>After CI Checks Pass</font>.",
        styles["RBody"]))
    story.append(Paragraph(
        "All-checks-must-pass is the right default. If we ever need per-check granularity, that lives "
        "in GitHub branch protection (Settings → Branches → Add rule → Require status checks), not "
        "in Render. Composes cleanly if we add it later.", styles["RBody"]))

    # ── Section 6: Three-layer guardrail ─────────────────
    story.append(Paragraph("6. Three Layers of Guardrail Now Active", styles["RH2"]))
    layers = [
        [tc("Layer"), tc("What it does"), tc("When it fires")],
        [tc("Pre-push hook"), tc("go vet, go test, eslint, npm test on the developer's machine"), tc("Before push leaves machine")],
        [tc("GitHub Actions CI"), tc("Same battery + go test -race + golangci-lint + coverage"), tc("After push reaches origin")],
        [tc("Render gate"), tc("Wait for all GitHub status checks before auto-deploying"), tc("Before deploy starts")],
    ]
    story.append(make_table(layers[0], layers[1:],
                            col_widths=[1.5*inch, 3.5*inch, 1.6*inch]))
    story.append(Spacer(1, 4))
    story.append(Paragraph(
        "Each layer catches a different class of mistake. The hook stops local boo-boos cheaply. CI "
        "stops anything that slips past the hook (or past <font face='Courier'>--no-verify</font>). "
        "The Render gate stops red CI from ever reaching prod even if both prior layers somehow pass.",
        styles["RBody"]))

    # ── Section 7: Diff summary ──────────────────────────
    story.append(Paragraph("7. Diff Summary", styles["RH2"]))
    diff = [
        [tc("Commit"), tc("Files"), tc("Purpose")],
        [tc("e9f90d8"), tc("api/models/bracket_test.go, matchup_test.go"), tc("Update Prepare tests for new no-escape contract")],
        [tc("e2fa7ae"), tc(".githooks/pre-push (new), Makefile, README.md, 7 frontend files"),
                       tc("Pre-push hook + lint backlog cleared (15 issues → 0)")],
    ]
    story.append(make_table(diff[0], diff[1:], col_widths=[1.0*inch, 3.0*inch, 2.5*inch]))

    # ── Section 8: Project state ─────────────────────────
    story.append(Paragraph("8. Project State After This Cycle", styles["RH2"]))
    state = [
        [tc("HEAD on main"), tc("e2fa7ae — \"Add pre-push hook + clean up the lint backlog so CI is green\"")],
        [tc("Local checks"), tc("Pre-push hook active. Runs go vet + go test + eslint + npm test on every push.")],
        [tc("CI status"), tc("ESLint clean (0 errors, 0 warnings). All Go packages green. 5 frontend test suites / 43 tests pass.")],
        [tc("Render auto-deploy"), tc("Both services gated on \"After CI Checks Pass\".")],
        [tc("Tests run by default"), tc("Yes — locally via the hook, in CI via the workflow.")],
    ]
    story.append(make_table(["Metric", "Value"], state, col_widths=[2.2*inch, 4.3*inch]))

    story.append(Spacer(1, 10))

    story.append(Paragraph("Deferred (filed as follow-ups)", styles["RH3"]))
    story.append(bullet("<b>The flaky <font face='Courier'>TestExtractTokenID/rejects_tampered_token</font></b> "
                        "in the auth package — failed about 1 in 4 runs after this cycle landed. <i>(Already "
                        "fixed in the next cycle, commit <font face='Courier'>0fc8e87</font>; see the "
                        "JWT-tamper flake-fix report.)</i>"))
    story.append(bullet("<b><font face='Courier'>User.go</font> escape cleanup.</b> Same anti-pattern "
                        "as Bracket / Matchup but on username + email; the integration tests pin the "
                        "old escape behavior and need updating in lockstep. Separate cycle."))
    story.append(bullet("<b>npm audit pass.</b> 88 Dependabot alerts on the post-cutover repo "
                        "(2 critical, 40 high, 37 moderate, 9 low). Likely transitive in the React "
                        "tree; not on fire today, worth a hygiene cycle."))
    story.append(bullet("<b>Operational follow-up from the bracket cycle.</b> Delete + re-create the "
                        "<font face='Courier'>Greatest Sci-Fi Movie of All Time</font> test bracket on "
                        "prod so its title is stored unescaped under the new contract."))
    story.append(bullet("<b>Optional belt-tightening.</b> Branch protection on GitHub to require "
                        "specific named status checks (currently the gate is all-or-nothing). Useful if "
                        "the repo ever grows non-blocking checks (perf benchmarks, link checkers, etc.)."))

    story.append(Spacer(1, 10))
    story.append(hr())
    story.append(Paragraph(
        "<i>Generated by Claude Code. The user's question — \"are tests being run before we push?\" "
        "— uncovered a gap that had been silently widening for weeks. Three layers of guardrail "
        "stood up the same day. The seatbelt is the cheapest investment with the best return: it "
        "literally caught its first regression on the very next push.</i>",
        styles["RSmall"]))

    doc.build(story)
    print(f"PDF written to {OUTPUT}")


if __name__ == "__main__":
    build()
