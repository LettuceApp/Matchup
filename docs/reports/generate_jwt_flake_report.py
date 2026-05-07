#!/usr/bin/env python3
"""Generate the JWT-Tamper Flake Fix PDF report.

Small, single-commit cycle that fixed a probabilistic test flake in
the auth package. Style mirrors the prior reports so the series stays
consistent.
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

OUTPUT = "/Users/cj/Documents/Projects/go-workspace/Matchup/docs/reports/matchup-jwt-tamper-flake-fix.pdf"

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
    story.append(Paragraph("JWT-Tamper Test Flake Fix — Report",
                           styles["RH2"]))
    story.append(Paragraph(f"Date: {date.today().strftime('%B %d, %Y')}",
                           styles["RSmall"]))
    story.append(Paragraph(
        "A small, single-commit cycle that eliminated a probabilistic flake "
        "in the auth-package unit tests. Lands the same day as the lint "
        "cleanup + pre-push hook. Now the local hook gives consistent "
        "results, so green-on-the-machine actually means green-on-CI.",
        styles["RSmall"]))
    story.append(hr())

    # ── Section 1: How it was found ──────────────────────
    story.append(Paragraph("1. Discovery", styles["RH2"]))
    story.append(Paragraph(
        "The previous cycle landed a pre-push hook that runs the same "
        "battery CI runs (<font face='Courier'>go vet</font>, "
        "<font face='Courier'>go test</font>, "
        "<font face='Courier'>eslint --max-warnings 0</font>, "
        "<font face='Courier'>npm test</font>) before letting a push leave "
        "the machine. While verifying it, the auth-package unit tests "
        "failed and passed alternately on identical code. Estimated rate "
        "from the user’s anecdotal sampling: about 1 in 4 runs.",
        styles["RBody"]))
    story.append(Paragraph(
        "Initial hypothesis was test-ordering or env-state leakage. "
        "Reading the code carefully ruled both out.", styles["RBody"]))

    story.append(Paragraph("What was ruled out", styles["RH3"]))
    story.append(bullet("<b>Test ordering.</b> Subtests in token_test.go are "
                        "sequential — none call <font face='Courier'>t.Parallel()</font>. "
                        "Within a package, <font face='Courier'>go test</font> runs subtests "
                        "in declaration order."))
    story.append(bullet("<b>Env-state leakage.</b> "
                        "<font face='Courier'>t.Setenv(\"API_SECRET\", …)</font> at the top of "
                        "<font face='Courier'>TestExtractTokenID</font> scopes the value to "
                        "the parent test and all its subtests. Go’s testing package guarantees "
                        "the env is restored after the test ends."))
    story.append(bullet("<b>Cross-package collisions.</b> "
                        "<font face='Courier'>go test ./...</font> runs different packages in "
                        "separate processes; env collisions between packages aren’t physically "
                        "possible."))
    story.append(bullet("<b>Cached secrets.</b> "
                        "<font face='Courier'>parseToken</font> reads "
                        "<font face='Courier'>os.Getenv(\"API_SECRET\")</font> fresh on every "
                        "call. No package-level cache."))
    story.append(bullet("<b>Refresh-token tests.</b> "
                        "<font face='Courier'>refresh_test.go</font> is gated on a "
                        "<font face='Courier'>//go:build integration</font> tag and doesn’t run "
                        "in the unit-test path."))

    # ── Section 2: Real root cause ────────────────────────
    story.append(Paragraph("2. The Actual Root Cause — Inside the Tamper", styles["RH2"]))
    story.append(Paragraph(
        "The bug lived in the test’s tamper logic, not anywhere else.", styles["RBody"]))

    story.append(Paragraph("What the test did", styles["RH3"]))
    story.append(Paragraph(
        "<font face='Courier'>token_test.go:123</font>: "
        "<font face='Courier'>tampered := token[:len(token)-1] + \"X\"</font>",
        styles["RBody"]))
    story.append(Paragraph(
        "Asserts <font face='Courier'>ExtractTokenID(req)</font> returns an error for the "
        "tampered token. Sometimes it does. Sometimes it doesn’t.", styles["RBody"]))

    story.append(Paragraph("Why the tamper sometimes does nothing", styles["RH3"]))
    story.append(bullet("JWT signatures from <font face='Courier'>HS256</font> are <b>32 bytes</b> "
                        "of HMAC-SHA256 output."))
    story.append(bullet("<font face='Courier'>RawURLEncoding</font> encodes 32 bytes (256 bits) into "
                        "<b>43 base64url characters</b>. The last char represents 4 bits of source "
                        "+ 2 padding-zero bits."))
    story.append(bullet("<b>“X”</b> in base64url has 6-bit value <font face='Courier'>010111</font>. "
                        "Top 4 bits = <font face='Courier'>0101</font> (5). Bottom 2 bits = "
                        "<font face='Courier'>11</font> — non-zero pad bits."))
    story.append(bullet("Go’s <font face='Courier'>base64.RawURLEncoding</font> is <b>lenient</b> by "
                        "default — it silently discards non-zero pad bits during decode rather than "
                        "erroring. So the tampered token decodes to:"))
    story.append(Paragraph(
        "&nbsp;&nbsp;&nbsp;&nbsp;<font face='Courier'>tampered_byte_31 = (original_byte_31 &amp; 0xF0) | 0x05</font>",
        styles["RBody"]))
    story.append(bullet("If the original signature’s byte 31 already has its bottom 4 bits equal to "
                        "5 (1 in 16 of random HMAC output, ~6.25%), the tampered byte equals the "
                        "original byte. The signatures are <b>bit-identical</b>; validation passes; "
                        "the test fails."))
    story.append(Paragraph(
        "Reported “1 in 4” is loosely consistent with 1/16 given how few times the user ran the "
        "suite. The geometry of the math (probabilistic, depends only on byte 31’s low nibble) "
        "explained every observed failure cleanly.", styles["RBody"]))

    story.append(Paragraph("Why this isn’t a security bug", styles["RH3"]))
    story.append(Paragraph(
        "<font face='Courier'>parseToken</font> correctly rejects tokens whose signatures "
        "<i>genuinely</i> differ. The lenient pad-bit behavior in "
        "<font face='Courier'>base64.RawURLEncoding</font> is a Go standard-library convention, "
        "not a vulnerability — an attacker who knows part of a signature can’t use the lenience "
        "to forge a different but accepted signature. The bug was the <i>test</i> assuming a "
        "single-char flip is always meaningful when it isn’t.",
        styles["RBody"]))

    # ── Section 3: The fix ───────────────────────────────
    story.append(Paragraph("3. The Fix", styles["RH2"]))
    story.append(Paragraph(
        "Replace the entire signature segment with a string that cannot ever be a valid "
        "HMAC-SHA256 signature — wrong length, wrong charset:", styles["RBody"]))
    story.append(Paragraph(
        "<font face='Courier'>parts := strings.Split(token, \".\")</font><br/>"
        "<font face='Courier'>parts[2] = \"this-is-not-a-real-signature\"</font><br/>"
        "<font face='Courier'>tampered := strings.Join(parts, \".\")</font>",
        styles["RBody"]))
    story.append(bullet("Unambiguous: this string isn’t a valid HMAC-SHA256 output under any key, "
                        "so verification must fail."))
    story.append(bullet("Robust regardless of base64 lenient-decode quirks. Even if the standard "
                        "library’s decode behavior changes in some future Go release, the test "
                        "still asserts what it should."))
    story.append(bullet("Same pattern the adjacent “rejects token signed with wrong secret” subtest "
                        "uses (string-split-and-replace), so the file stays consistent."))

    # ── Section 4: Verification ──────────────────────────
    story.append(Paragraph("4. Verification", styles["RH2"]))
    story.append(Paragraph(
        "The original failure rate was ~6% per run. Running the previously-flaky test 50 times "
        "would have a <font face='Courier'>(1 - 0.0625)^50 ≈ 4%</font> chance of all passing if the "
        "bug were still present. Running 50 in a row and seeing all pass is therefore strong "
        "evidence the fix works.", styles["RBody"]))
    story.append(bullet("<font face='Courier'>for i in 1..50; go test -run TestExtractTokenID ./auth/...</font> "
                        "→ <b>50 / 50 pass</b>."))
    story.append(bullet("Full auth package run 5 / 5 pass."))
    story.append(bullet("Pre-push hook fired on the commit and validated the full battery "
                        "(go vet, go test, eslint, npm test) before allowing the push."))

    # ── Section 5: Diff ──────────────────────────────────
    story.append(Paragraph("5. Diff Summary", styles["RH2"]))
    diff = [
        [tc("api/auth/token_test.go"), tc("Replace single-char tamper with whole-signature "
                                          "replacement in TestExtractTokenID/rejects_tampered_token. "
                                          "+17 / −2.")],
    ]
    story.append(make_table(["File", "Change"], diff, col_widths=[2.4*inch, 4.1*inch]))
    story.append(Spacer(1, 4))
    story.append(Paragraph(
        "Single commit <font face='Courier'>0fc8e87</font>. No source changes — test-only fix.",
        styles["RBody"]))

    # ── Section 6: Project state ─────────────────────────
    story.append(Paragraph("6. Project State After This Cycle", styles["RH2"]))
    state = [
        [tc("HEAD on main"), tc("0fc8e87 — Fix flaky TestExtractTokenID/rejects_tampered_token")],
        [tc("Reliability"), tc("Auth-package unit tests now deterministic. Pre-push hook gives "
                               "consistent green/red signal — no false-blocks expected.")],
        [tc("Belt + suspenders"), tc("Pre-push hook + GitHub Actions CI + Render \"After CI Checks "
                                     "Pass\" gate, all three layers active.")],
        [tc("Render gate"), tc("Both matchup-backend and matchup-frontend now wait for CI green "
                               "before auto-deploying.")],
        [tc("Tests"), tc("All Go packages green. 5 frontend test suites / 43 tests green. "
                         "ESLint clean (--max-warnings 0).")],
    ]
    story.append(make_table(["Metric", "Value"], state, col_widths=[2.2*inch, 4.3*inch]))

    story.append(Spacer(1, 10))

    story.append(Paragraph("Deferred (filed as follow-ups)", styles["RH3"]))
    story.append(bullet("<b>User.go html.EscapeString cleanup.</b> Same anti-pattern as "
                        "Bracket/Matchup but on username + email; integration tests will need "
                        "their assertions updated. Separate cycle."))
    story.append(bullet("<b>Optional: a JWT fuzz test.</b> Now that we know lenient pad-bit decoding "
                        "is the relevant edge case, a small fuzz target around "
                        "<font face='Courier'>parseToken</font> would catch any future regression "
                        "where a hand-crafted edge-case token sneaks past validation."))
    story.append(bullet("<b>npm audit cycle.</b> 88 Dependabot alerts on the post-cutover repo."))
    story.append(bullet("<b>Operational follow-up from the bracket cycle.</b> Delete + re-create "
                        "the <font face='Courier'>Greatest Sci-Fi Movie of All Time</font> test "
                        "bracket on prod so its title is stored unescaped under the new contract."))

    story.append(Spacer(1, 10))
    story.append(hr())
    story.append(Paragraph(
        "<i>Generated by Claude Code. The interesting find was that the original "
        "“flaky in auth” framing pointed away from the bug — the auth package itself was "
        "fine. The bug was a test assuming a single-character base64 flip is always meaningful "
        "when it isn’t. Worth keeping that pattern in mind: probabilistic flakes that pass "
        "in isolation often have a cause inside the test, not the system under test.</i>",
        styles["RSmall"]))

    doc.build(story)
    print(f"PDF written to {OUTPUT}")


if __name__ == "__main__":
    build()
