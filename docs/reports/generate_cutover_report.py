#!/usr/bin/env python3
"""Generate the Production Cutover & Platform Hardening PDF report.

Covers work between the prior report (matchup-share-observability-
and-feed.pdf, May 6, 2026 morning) and the production cutover later
the same day. Style mirrors the prior generators so the four reports
read as a series.
"""

from reportlab.lib.pagesizes import letter
from reportlab.lib.styles import getSampleStyleSheet, ParagraphStyle
from reportlab.lib.units import inch
from reportlab.lib.colors import HexColor
from reportlab.platypus import (
    SimpleDocTemplate, Paragraph, Spacer, Table, TableStyle,
    PageBreak, HRFlowable,
)
from reportlab.lib.enums import TA_LEFT, TA_CENTER
from datetime import date

OUTPUT = "/Users/cj/Documents/Projects/go-workspace/Matchup/docs/reports/matchup-production-cutover-and-platform-hardening.pdf"

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
styles.add(ParagraphStyle("RCode", parent=styles["Normal"], fontSize=8,
                          fontName="Courier", leading=10, spaceAfter=4,
                          leftIndent=20, textColor=HexColor("#333333")))
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

ACCENT = HexColor("#0f3460")
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

    # ── Title ────────────────────────────────────────────
    story.append(Paragraph("Matchup Platform", styles["RH1"]))
    story.append(Paragraph("Production Cutover & Platform Hardening — Report",
                           styles["RH2"]))
    story.append(Paragraph(f"Date: {date.today().strftime('%B %d, %Y')}",
                           styles["RSmall"]))
    story.append(Paragraph("Covers the production cutover from the legacy gin/gorm REST stack to the "
                           "Connect-RPC + chi + sqlx rewrite, plus the accumulated feature work that "
                           "shipped in the same push. Continues the series after the prior report on "
                           "home feed, observability, and sharing (May 6, 2026 morning).",
                           styles["RSmall"]))
    story.append(hr())

    # ── Section 1: Production Cutover ────────────────────
    story.append(Paragraph("1. Production Cutover to Render", styles["RH2"]))
    story.append(Paragraph(
        "The gravity event for this cycle. <font face='Courier'>origin/main</font> had been frozen on "
        "the old gin + gorm + REST architecture while every cycle of meaningful work happened on a "
        "long-lived <font face='Courier'>improvements</font> branch (chi + sqlx + Connect-RPC). The "
        "two were architecturally incompatible — not just a feature delta. Shipping the rewrite meant "
        "treating it as a hard cutover.", styles["RBody"]))

    story.append(Paragraph("Decision Tree", styles["RH3"]))
    story.append(bullet("<b>Stay on Render, defer AKS.</b> The <font face='Courier'>k8s/</font> "
                        "manifests + <font face='Courier'>deploy.yml</font> + "
                        "<font face='Courier'>deploy-staging.yml</font> exist as a planned destination, "
                        "but no AKS cluster, ACR registry, or cert-manager has been stood up. Render "
                        "is the lowest-risk path for getting accumulated work into prod."))
    story.append(bullet("<b>Wipe + start fresh on the prod DB.</b> The legacy schema (gorm AutoMigrate) "
                        "diverged from the goose migrations the new code expects. With only a few "
                        "real users on prod, accepting the loss was the cleanest path."))
    story.append(bullet("<b>Force-push <font face='Courier'>improvements</font> over "
                        "<font face='Courier'>main</font>.</b> One atomic diff, one commit on main, "
                        "one Render auto-deploy. Rollback path is Render’s one-click previous-deploy."))
    story.append(bullet("<b>Mute the AKS workflows.</b> "
                        "<font face='Courier'>deploy.yml</font> and "
                        "<font face='Courier'>deploy-staging.yml</font> swapped from "
                        "<font face='Courier'>push: branches: [main]</font> to "
                        "<font face='Courier'>workflow_dispatch:</font> only. Without "
                        "<font face='Courier'>AZURE_CLIENT_ID</font> et al. configured in GitHub Actions "
                        "secrets, every push to main would otherwise produce a red CI run."))

    story.append(Paragraph("Cutover Sequence", styles["RH3"]))
    cutover = [
        [tc("1"), tc("Updated Render env vars on matchup-api: <font face='Courier'>APP_BASE_URL</font>, "
                     "<font face='Courier'>PUBLIC_ORIGIN</font>, <font face='Courier'>SENDGRID_API_KEY</font>, "
                     "<font face='Courier'>SENDGRID_FROM</font>. Confirmed plain "
                     "<font face='Courier'>AWS_REGION</font> (the K8s "
                     "<font face='Courier'>*_ENV</font> suffix pattern doesn’t apply on Render).")],
        [tc("2"), tc("Opened Render psql terminal against matchup_postgres. Ran "
                     "<font face='Courier'>DROP SCHEMA public CASCADE; CREATE SCHEMA public; CREATE EXTENSION pgcrypto;</font>")],
        [tc("3"), tc("Force-pushed <font face='Courier'>improvements</font> to "
                     "<font face='Courier'>main</font> with "
                     "<font face='Courier'>git config http.postBuffer 524288000</font> "
                     "(229 files / 32k+ insertions exceeded the default 1MB postBuffer — first push failed).")],
        [tc("4"), tc("Render auto-deploy triggered on the new commit. Goose ran all 24 migrations "
                     "(001 through 024) on first pod boot from the embedded SQL files in "
                     "<font face='Courier'>api/migrations/</font>.")],
        [tc("5"), tc("Smoked prod from the deployed frontend. Discovered "
                     "<font face='Courier'>REACT_APP_API_BASE</font> still pointed at "
                     "<font face='Courier'>.../api/v1</font> (legacy gin prefix) and email-verification "
                     "gated content creation. Fixed both post-cutover (sections 9 and 10 of this report).")],
    ]
    story.append(make_table(["Step", "Detail"], cutover, col_widths=[0.4*inch, 6.1*inch]))

    story.append(Paragraph("Architectural Divergence Resolved", styles["RH3"]))
    div = [
        [tc("Layer"), tc("Before (origin/main)"), tc("After (this push)")],
        [tc("HTTP framework"), tc("gin"), tc("chi")],
        [tc("RPC style"), tc("REST"), tc("Connect-RPC (over HTTP/1.1 + JSON)")],
        [tc("DB driver"), tc("gorm + AutoMigrate"), tc("sqlx + goose migrations")],
        [tc("Codegen"), tc("Hand-written DTOs"), tc("Buf + protoc-gen-connect-go")],
        [tc("Scheduler"), tc("External Dagster service"), tc("Embedded Go scheduler (api/scheduler)")],
    ]
    story.append(make_table(div[0], div[1:], col_widths=[1.4*inch, 2.3*inch, 2.8*inch]))
    story.append(Spacer(1, 4))
    story.append(Paragraph(
        "Confirmed via grep that no <font face='Courier'>gin-gonic</font>, "
        "<font face='Courier'>gorm.io</font>, or <font face='Courier'>jinzhu/gorm</font> imports remain "
        "in <font face='Courier'>api/</font>. Neither appears in <font face='Courier'>go.mod</font>. "
        "The migration off them is now the rewrite itself.", styles["RBody"]))

    # ── Section 2: Refresh Tokens ──────────────────────────
    story.append(Paragraph("2. Refresh Tokens (migration 020)", styles["RH2"]))
    story.append(Paragraph(
        "Pre-cutover, every 401 forced a re-login. The new flow uses a 15-minute access token plus a "
        "rotating refresh token; any 401 triggers a silent <font face='Courier'>Refresh</font> RPC and "
        "the original request is retried once the new access token lands.", styles["RBody"]))
    story.append(bullet("<b>Rotation + theft detection.</b> Each refresh call invalidates the prior "
                        "token and issues a new one. A reused (already-rotated) refresh token revokes "
                        "the entire token family for that user — a stolen-cookie indicator."))
    story.append(bullet("<b>Concurrency-safe interceptor.</b> "
                        "<font face='Courier'>frontend/src/services/api.js</font> coalesces N parallel "
                        "401s onto a single in-flight refresh promise so a screen firing five reads at "
                        "once produces exactly one Refresh call, not five."))
    story.append(bullet("<b>Self-healing logout.</b> If Refresh itself 401s, "
                        "<font face='Courier'>clearAuthStorage()</font> wipes localStorage and "
                        "redirects to <font face='Courier'>/login</font>. No infinite-retry loop."))

    # ── Section 3: Account Lifecycle & Moderation ──────────
    story.append(Paragraph("3. Account Lifecycle & Moderation (migrations 022, 023)", styles["RH2"]))
    story.append(Paragraph(
        "Two migrations and one Connect service apiece. The shape is conventional but the column "
        "semantics matter: <font face='Courier'>banned_at</font> distinguishes an admin ban from a "
        "self-delete (<font face='Courier'>deleted_at</font>) so the cascade behavior diverges.",
        styles["RBody"]))

    story.append(Paragraph("Account Lifecycle (022)", styles["RH3"]))
    story.append(bullet("<b>Soft-delete columns.</b> <font face='Courier'>users.deleted_at</font>, "
                        "<font face='Courier'>users.banned_at</font>, plus a 30-day "
                        "<font face='Courier'>hardDeleteUser</font> cron that purges anything past the grace window."))
    story.append(bullet("<b>Block / mute tables.</b> Per-direction rows in "
                        "<font face='Courier'>blocks</font> and <font face='Courier'>mutes</font>; "
                        "feed queries join against them so blocked content disappears in both directions."))
    story.append(bullet("<b>Account-settings page.</b> Frontend "
                        "<font face='Courier'>AccountSettings.js</font> + a dedicated "
                        "<font face='Courier'>BlocksAndMutes.js</font> for managing the lists."))

    story.append(Paragraph("Moderation (023)", styles["RH3"]))
    story.append(bullet("<b><font face='Courier'>admin_actions</font>.</b> Append-only audit log. "
                        "Every moderation decision (ban, content takedown, dismiss-report) writes a row "
                        "with <font face='Courier'>actor_id</font>, <font face='Courier'>subject_type</font>, "
                        "<font face='Courier'>subject_id</font>, action, reason. No updates, no deletes."))
    story.append(bullet("<b><font face='Courier'>reports</font>.</b> User-submitted content reports. "
                        "Partial index on open rows powers the AdminDashboard Reports tab without "
                        "scanning resolved entries."))
    story.append(bullet("<b>AdminDashboard.</b> Frontend page guarded by "
                        "<font face='Courier'>httpctx.IsAdminRequest</font>. Tabs for open reports, "
                        "user search, recent admin actions."))

    # ── Section 4: Notifications & Push ──────────────────
    story.append(Paragraph("4. Notifications & Push (migrations 016, 017, 019, 021)", styles["RH2"]))
    story.append(Paragraph(
        "Multi-platform push pipeline plus an in-app notification feed. Four migrations, since each one "
        "added a single denormalized concern that needed its own index strategy.", styles["RBody"]))
    notif = [
        [tc("016 add_notifications"), tc("In-app feed table. One row per notification, denormalized actor + subject + read_at.")],
        [tc("017 notification_prefs"), tc("Per-user, per-channel toggles (likes, follows, comments, bracket-advance, etc.)")],
        [tc("019 push_subscriptions"), tc("Web Push endpoints (endpoint URL + p256dh + auth keys). One row per browser/device.")],
        [tc("021 push_platform"), tc("Platform discriminator on push_subscriptions so iOS Safari, Android Chrome, and desktop "
                                     "can be filtered for platform-specific quirks.")],
    ]
    story.append(make_table(["Migration", "Adds"], notif, col_widths=[1.6*inch, 4.9*inch]))
    story.append(Spacer(1, 4))
    story.append(bullet("<b>Push CTR via service worker.</b> "
                        "<font face='Courier'>frontend/public/sw.js</font> appends "
                        "<font face='Courier'>?utm_source=push&push_kind=tag</font> to the URL it opens "
                        "on click. On landing, <font face='Courier'>App.js</font> reads the params, fires "
                        "<font face='Courier'>track('push_clicked')</font>, and strips the params via "
                        "<font face='Courier'>history.replaceState</font> so they don’t pollute "
                        "subsequent share links."))
    story.append(bullet("<b>Email digest worker.</b> Migration 018 backed a daily digest job that "
                        "respects per-user opt-outs from the <font face='Courier'>notification_prefs</font> "
                        "table. Runs from the embedded scheduler, not Dagster."))

    # ── Section 5: Anonymous Browsing & Voting ──────────
    story.append(Paragraph("5. Anonymous Browsing & Voting", styles["RH2"]))
    story.append(Paragraph(
        "The biggest funnel-shape change of the cycle. Anonymous visitors can now browse, read, and "
        "cast up to 3 votes per browser before a soft upgrade prompt. Bracket creation/play stays "
        "member-gated. Anon votes cast pre-signup are merged into the new account on signup.",
        styles["RBody"]))

    story.append(Paragraph("Backend", styles["RH3"]))
    story.append(bullet("<b><font face='Courier'>AnonVoteCap = 3</font>.</b> Per-anon-id limit, enforced "
                        "in <font face='Courier'>vote_connect_handler.go</font>. Returns "
                        "<font face='Courier'>FailedPrecondition</font> with reason "
                        "<font face='Courier'>anon_vote_limit</font> so the frontend can show the "
                        "upgrade modal."))
    story.append(bullet("<b><font face='Courier'>anonVoteIPBudget = 10</font>.</b> Coarse IP-tier limiter "
                        "in <font face='Courier'>middlewares/rate_limit.go</font> (<font face='Courier'>CheckIPRateLimit</font>) "
                        "to throttle ballot-stuffing across many minted anon IDs. Keyed on hashed IP."))
    story.append(bullet("<b>Bracket gate.</b> <font face='Courier'>bracket_connect_handler.go</font> "
                        "rejects votes from unauthenticated callers; brackets remain a member-only mode."))
    story.append(bullet("<b>Anon profile viewing.</b> <font face='Courier'>GetUser</font> succeeds for "
                        "anonymous viewers when the target profile isn’t private; private profiles "
                        "404 to anon. Public discovery without forcing signup."))
    story.append(bullet("<b>Vote merge on signup.</b> "
                        "<font face='Courier'>CreateUser</font> reads the "
                        "<font face='Courier'>X-Anon-Id</font> header and runs "
                        "<font face='Courier'>mergeDeviceVotesToUser</font> in the same transaction so "
                        "votes don’t double-count."))

    story.append(Paragraph("Frontend", styles["RH3"]))
    story.append(bullet("<b>Per-browser UUID.</b> "
                        "<font face='Courier'>utils/anonId.js</font> exposes "
                        "<font face='Courier'>getOrCreateAnonId</font>, "
                        "<font face='Courier'>peekAnonId</font>, "
                        "<font face='Courier'>clearAnonId</font>. "
                        "<font face='Courier'>peek</font> never mints as a side effect, so passive page "
                        "reads don’t pollute localStorage; the UUID is minted only on first vote."))
    story.append(bullet("<b>Global upgrade modal.</b> "
                        "<font face='Courier'>contexts/AnonUpgradeContext.js</font> exposes "
                        "<font face='Courier'>promptUpgrade(reason)</font>; the modal renders once per "
                        "app via <font face='Courier'>AnonUpgradeProvider</font> in "
                        "<font face='Courier'>App.js</font>."))
    story.append(bullet("<b>Vote counter.</b> <font face='Courier'>AnonVoteCounter</font> shows the "
                        "remaining-votes pip strip; "
                        "<font face='Courier'>useAnonVoteStatus</font> hook polls "
                        "<font face='Courier'>GetAnonVoteStatus</font> for the live count."))
    story.append(bullet("<b>RequireAuth removed</b> from <font face='Courier'>/</font>, "
                        "<font face='Courier'>/home</font>, "
                        "<font face='Courier'>/users/:username</font>, and "
                        "<font face='Courier'>/users/:userId/profile</font>."))

    # ── Section 6: Analytics ────────────────────────────
    story.append(Paragraph("6. Analytics — PostHog & Sentry", styles["RH2"]))
    story.append(Paragraph(
        "Sentry was already shipped (frontend + backend middleware). This cycle added PostHog with the "
        "same no-op-on-empty-key pattern so prod doesn’t crash before the project token is "
        "populated on Render.", styles["RBody"]))
    story.append(bullet("<b>Bootstrap pattern.</b> "
                        "<font face='Courier'>frontend/src/posthog.js</font> mirrors "
                        "<font face='Courier'>sentry.js</font>: import for side effects, no-op when "
                        "<font face='Courier'>REACT_APP_POSTHOG_KEY</font> is empty. Safe to deploy "
                        "before the env var lands."))
    story.append(bullet("<b><font face='Courier'>track()</font> helper.</b> "
                        "<font face='Courier'>utils/analytics.js</font> auto-attaches "
                        "<font face='Courier'>is_anon</font>, <font face='Courier'>anon_id</font>, "
                        "<font face='Courier'>user_id</font> to every event so funnel queries don’t "
                        "have to join tables."))
    story.append(bullet("<b>Identify on signup/login.</b> "
                        "<font face='Courier'>posthog.identify(userId)</font> with the "
                        "<font face='Courier'>$alias</font> set to the prior "
                        "<font face='Courier'>$device_id</font> merges anon → user sessions in "
                        "PostHog’s person graph."))

    story.append(Paragraph("Events Wired (13 total)", styles["RH3"]))
    events = [
        [tc("anon_home_viewed"), tc("First view of /home as an anon. Top of funnel.")],
        [tc("vote_cast"), tc("Every vote (anon + auth). Property: vote_number_for_anon for cap-tracking.")],
        [tc("upgrade_prompt_shown"), tc("Anon upgrade modal opened. Property: reason (vote_cap | bracket_gated | profile_action).")],
        [tc("upgrade_prompt_dismissed"), tc("Modal closed without action.")],
        [tc("signup_started"), tc("/register page mounted from any source.")],
        [tc("signup_completed"), tc("CreateUser RPC succeeded. Fires identify in the same call.")],
        [tc("login_completed"), tc("Authenticated session established. Fires identify.")],
        [tc("matchup_created"), tc("CreateMatchup RPC succeeded.")],
        [tc("bracket_created"), tc("CreateBracket RPC succeeded.")],
        [tc("comment_created"), tc("Top-level or reply.")],
        [tc("share_clicked"), tc("Native share or copy-link triggered (existing share-feature integration).")],
        [tc("push_clicked"), tc("Service-worker-tagged URL landed. Read on first nav, params stripped.")],
        [tc("verify_email_clicked"), tc("Banner CTA tapped. Lets us measure the soft-nudge funnel separately.")],
    ]
    story.append(make_table(["Event", "When fired"], events, col_widths=[1.7*inch, 4.8*inch]))

    # ── Section 7: Email Verification ─────────────────────
    story.append(Paragraph("7. Email Verification (migration 024)", styles["RH2"]))
    story.append(Paragraph(
        "Soft-nudge gate on content creation, not a login gate. Unverified users can sign in, browse, "
        "and vote. The original gate refused matchup / bracket / comment creation until verified. "
        "<i>This gate is currently disabled in prod</i> while SendGrid configuration on Render is "
        "still in flight (commit <font face='Courier'>9979bdf</font>). Re-enabling is a one-line revert.",
        styles["RBody"]))
    story.append(bullet("<b>Schema.</b> "
                        "<font face='Courier'>users.email_verified_at</font> nullable timestamp, plus "
                        "an <font face='Courier'>email_verification_tokens</font> table holding hashed "
                        "tokens (sha256), <font face='Courier'>used_at</font> for one-shot semantics, "
                        "and a 24-hour expiry."))
    story.append(bullet("<b>RPCs.</b> "
                        "<font face='Courier'>RequestEmailVerification</font> mints a token, hashes it, "
                        "stores the hash, and emails the plaintext via SendGrid. "
                        "<font face='Courier'>ConfirmEmailVerification</font> looks up the hash, marks "
                        "<font face='Courier'>used_at</font>, and flips "
                        "<font face='Courier'>email_verified_at</font> in the same transaction."))
    story.append(bullet("<b>Frontend banner + verify page.</b> "
                        "<font face='Courier'>EmailVerificationBanner.js</font> shows on every page for "
                        "unverified users; <font face='Courier'>/verify-email</font> handles the link "
                        "from the email."))
    story.append(bullet("<b>Forgot/reset password pages.</b> Same token-mint-confirm pattern, separate "
                        "table. Shipped alongside the verify flow since they share the email-template "
                        "infrastructure."))

    # ── Section 8: Bug Fixes Shipped ─────────────────────
    story.append(Paragraph("8. Bug Fixes Shipped", styles["RH2"]))

    story.append(Paragraph("Followers list “Matchup Fan” Placeholder", styles["RH3"]))
    story.append(Paragraph(
        "Root cause: <font face='Courier'>fetchFollowRowsStandalone</font> in "
        "<font face='Courier'>user_connect_handler.go</font> used "
        "<font face='Courier'>SELECT users.*</font>, which returns columns "
        "(<font face='Courier'>password</font>, <font face='Courier'>deleted_at</font>, "
        "<font face='Courier'>email_verified_at</font>, etc.) that the "
        "<font face='Courier'>followRow</font> struct doesn’t declare. sqlx’s strict-scan mode "
        "treated this as a fatal error and returned an empty list — the UI fell back to the "
        "“Matchup Fan” placeholder for every row. Fixed with an explicit column list.",
        styles["RBody"]))

    story.append(Paragraph("Home Card Uniform Heights", styles["RH3"]))
    story.append(Paragraph(
        "Cards in the same grid row had different heights when titles wrapped to two lines or tag "
        "rows were missing. Fixed in <font face='Courier'>HomePage.css</font>: "
        "<font face='Courier'>grid-auto-rows: 1fr</font> on the grid forces every card on a row to "
        "match the tallest, and <font face='Courier'>flex: 1 1 auto</font> on "
        "<font face='Courier'>.home-card__caption</font> lets the caption absorb the leftover space so "
        "the action bar pins to the bottom of every card.", styles["RBody"]))

    story.append(Paragraph("Owner-Controls Override-Winner Panel", styles["RH3"]))
    story.append(Paragraph(
        "The Override-Winner <font face='Courier'>&lt;details&gt;</font> panel rendered its seed-option "
        "list even when collapsed, dragging the matchup card height past its neighbours. Fixed in "
        "<font face='Courier'>MatchupPage.css</font> by gating "
        "<font face='Courier'>.matchup-winner-panel { display: none; }</font> behind "
        "<font face='Courier'>.matchup-winner-menu[open] &gt; .matchup-winner-panel</font>.",
        styles["RBody"]))

    story.append(Paragraph("Tray-Variant Button Sizing", styles["RH3"]))
    story.append(Paragraph(
        "The tray summary toggle button inherited a 36×36 fixed box from the icon-button base "
        "class, which clipped its text label. Fixed by resetting "
        "<font face='Courier'>width: auto; height: auto</font> for the "
        "<font face='Courier'>--tray</font> variant.", styles["RBody"]))

    story.append(Paragraph("Bracket Card Compact Variant", styles["RH3"]))
    story.append(Paragraph(
        "Bracket previews shared the matchup card stylesheet but rendered taller because of an extra "
        "metadata row. Added a <font face='Courier'>--compact</font> modifier that drops the row when "
        "the card is rendered inside a tighter slot (sidebar, search results).", styles["RBody"]))

    # ── Section 9: Post-Cutover Triage ────────────────────
    story.append(Paragraph("9. Post-Cutover Triage", styles["RH2"]))
    story.append(Paragraph(
        "Two issues surfaced once the new code was actually serving prod traffic. Both were "
        "configuration drift from the legacy stack rather than rewrite bugs.", styles["RBody"]))

    story.append(Paragraph("Path-Prefix Mismatch (REACT_APP_API_BASE)", styles["RH3"]))
    story.append(Paragraph(
        "Signup, login, and home-feed reads all returned 404. The deployed frontend’s "
        "<font face='Courier'>console.log</font> showed "
        "<font face='Courier'>Using API base URL: https://matchup-vhl6.onrender.com/api/v1</font>, but "
        "the new Connect handlers mount at root (<font face='Courier'>/user.v1.UserService/CreateUser</font>) "
        "— the <font face='Courier'>/api/v1</font> prefix was a leftover from the gin REST stack "
        "where every route was nested under it. A direct <font face='Courier'>GET</font> to the "
        "un-prefixed path returned <font face='Courier'>405 Method Not Allowed</font>, which is "
        "Connect’s tell that the handler is registered there and rejecting the wrong verb.",
        styles["RBody"]))
    story.append(Paragraph(
        "<b>Resolution:</b> drop <font face='Courier'>/api/v1</font> from "
        "<font face='Courier'>REACT_APP_API_BASE</font> on the frontend Render service. CRA env vars "
        "are baked at build time, so a redeploy is required for the change to take effect.",
        styles["RBody"]))
    story.append(Paragraph(
        "<b>Why not mount the backend under <font face='Courier'>/api/v1</font>?</b> Connect-RPC "
        "encodes the service version into the URL already (<font face='Courier'>user.v1.UserService</font>). "
        "Adding an outer <font face='Courier'>/api/v1</font> would be double-versioning, and every test "
        "harness, chi group, and middleware in the new code assumes root mount. One env var change is "
        "smaller than rewriting all of that.", styles["RBody"]))

    story.append(Paragraph("Email-Verification Gate Disabled", styles["RH3"]))
    story.append(Paragraph(
        "Once signup worked, every new account hit the verification gate on the first attempt to "
        "create a matchup. SendGrid is not yet configured in prod, so users had no way to receive the "
        "token and unblock themselves.", styles["RBody"]))
    story.append(Paragraph(
        "<b>Resolution:</b> short-circuit "
        "<font face='Courier'>requireVerifiedEmail</font> in "
        "<font face='Courier'>controllers/email_verification.go</font> with an early "
        "<font face='Courier'>return nil</font>. The original implementation is preserved as a comment "
        "in the same function. Column, RPCs, frontend banner, and verify page are all left intact — "
        "re-enabling is a one-line revert when SendGrid lands. Commit "
        "<font face='Courier'>9979bdf</font>.", styles["RBody"]))

    story.append(Paragraph("Admin Auto-Elevation Reconfirmed", styles["RH3"]))
    story.append(Paragraph(
        "After the DB wipe, asked the natural question: does the admin account come back automatically? "
        "It does — but as auto-<i>elevation</i>, not auto-<i>creation</i>. Nothing creates the "
        "admin row at server start; "
        "<font face='Courier'>models/User.Prepare()</font> flips "
        "<font face='Courier'>IsAdmin = true</font> on insert when the email matches "
        "<font face='Courier'>SeedAdminEmail</font> "
        "(<font face='Courier'>cordelljenkins1914@gmail.com</font>). First signup with that email is "
        "the path to a fresh admin row.", styles["RBody"]))

    # ── Section 10: Dagster Decommission ──────────────────
    story.append(Paragraph("10. Dagster Decommission", styles["RH2"]))
    story.append(Paragraph(
        "Confirmed via grep that Dagster has no live code path in <font face='Courier'>api/</font>. "
        "Every reference is a historical comment explaining what the new "
        "<font face='Courier'>scheduler</font> package replaced. Zero "
        "<font face='Courier'>os.Getenv(\"DAGSTER_*\")</font> calls. The "
        "<font face='Courier'>orchestration/*.py</font> directory it used to live in didn’t ship "
        "in the rewrite.", styles["RBody"]))
    story.append(Paragraph(
        "What’s left is operational on Render, not in the repo:", styles["RBody"]))
    story.append(bullet("Stop the Dagster Render service — while it runs, it re-bootstraps its "
                        "schema (22 tables: <font face='Courier'>alembic_version</font>, "
                        "<font face='Courier'>runs</font>, <font face='Courier'>asset_*</font>, etc.) "
                        "in the matchup_postgres DB on every restart. That’s why the squatter "
                        "tables came back after the <font face='Courier'>DROP SCHEMA public CASCADE</font>."))
    story.append(bullet("Drop the 22 orphan tables once Dagster is stopped. None collide with Matchup’s "
                        "names, so they’re harmless until then — just noise."))
    story.append(bullet("Remove <font face='Courier'>DAGSTER_*</font> env vars from "
                        "<font face='Courier'>matchup-api</font>. Inert today (no Go code reads them) "
                        "but worth scrubbing."))
    story.append(bullet("Delete the Dagster Render service when confident nothing else points at it."))

    # ── Section 11: Current Project State ─────────────────
    story.append(Paragraph("11. Current Project State", styles["RH2"]))
    state_data = [
        [tc("Total goose migrations"), tc("24 (10 added this cycle: 015–024)")],
        [tc("Architecture (HTTP)"), tc("chi router + Connect-RPC handlers")],
        [tc("Architecture (DB)"), tc("sqlx + goose-embedded migrations (no gorm)")],
        [tc("Architecture (RPC)"), tc("Connect-RPC over HTTP/1.1 + JSON; protoc-gen-connect-go codegen")],
        [tc("Production host"), tc("Render (matchup-api + matchup-uud5 frontend + matchup_postgres)")],
        [tc("AKS state"), tc("Manifests in repo, workflows muted to workflow_dispatch, cluster not provisioned")],
        [tc("Auth"), tc("15-min access token + rotating refresh token; theft detection wired")],
        [tc("Anon model"), tc("Per-browser UUID, 3-vote cap, IP-tier rate limit, vote merge on signup")],
        [tc("Moderation"), tc("Block, mute, report, append-only admin actions, admin dashboard")],
        [tc("Email verification"), tc("Wired end-to-end; gate temporarily disabled pending SendGrid")],
        [tc("Analytics"), tc("PostHog (13 events, identify on signup) + Sentry (frontend + backend)")],
        [tc("Push"), tc("Web Push + service-worker CTR tagging via utm_source=push")],
        [tc("Build status"), tc("Clean (go build, go vet; frontend npm run build with pre-existing test warnings)")],
    ]
    story.append(make_table(["Metric", "Value"], state_data,
                            col_widths=[2.2*inch, 4.3*inch]))
    story.append(Spacer(1, 10))

    story.append(Paragraph("Deferred From This Cycle", styles["RH3"]))
    story.append(bullet("<b>Configure SendGrid in Render.</b> Then revert "
                        "<font face='Courier'>9979bdf</font> to re-arm the email-verification gate."))
    story.append(bullet("<b>Populate PostHog + Sentry env vars on Render.</b> "
                        "<font face='Courier'>REACT_APP_POSTHOG_KEY</font>, "
                        "<font face='Courier'>REACT_APP_POSTHOG_HOST</font>, "
                        "<font face='Courier'>REACT_APP_POSTHOG_ENVIRONMENT</font> on the frontend; "
                        "<font face='Courier'>SENTRY_DSN</font> on the api. Trigger a manual rebuild "
                        "(CRA env vars are baked at build time)."))
    story.append(bullet("<b>Stop Dagster + drop 22 orphan tables</b> in matchup_postgres. Operational "
                        "Render task, no code change."))
    story.append(bullet("<b>Build the PostHog funnel</b> "
                        "<font face='Courier'>anon_home_viewed → vote_cast → "
                        "upgrade_prompt_shown → signup_started → signup_completed</font> "
                        "once events start flowing."))
    story.append(bullet("<b>npm audit cycle.</b> GitHub Dependabot reported 88 vulnerabilities "
                        "(2 critical, 40 high) on the post-cutover repo. Likely transitive in the React "
                        "tree; not on fire today, worth a sweep next cycle."))
    story.append(bullet("<b>AKS migration</b> is its own future cycle. Provision cluster + ACR + "
                        "cert-manager, populate <font face='Courier'>AZURE_CLIENT_ID</font> et al. as "
                        "GH Actions secrets, then re-enable the <font face='Courier'>push: branches: [main]</font> "
                        "trigger on <font face='Courier'>deploy.yml</font>."))
    story.append(bullet("<b>Optional banner mute.</b> If the orange “Verify your email” banner "
                        "is annoying while the gate is off, the early-return in "
                        "<font face='Courier'>EmailVerificationBanner.js</font> is a 5-minute change."))

    story.append(Spacer(1, 10))
    story.append(hr())
    story.append(Paragraph(
        "<i>Generated by Claude Code. Cutover verified by force-push to "
        "<font face='Courier'>origin/main</font>, Render auto-deploy, and goose migrations 001–024 "
        "running cleanly on first pod boot against a fresh matchup_postgres schema.</i>",
        styles["RSmall"]))

    doc.build(story)
    print(f"PDF written to {OUTPUT}")


if __name__ == "__main__":
    build()
