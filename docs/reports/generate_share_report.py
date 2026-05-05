#!/usr/bin/env python3
"""Generate the Home-Feed / Observability / Share-Feature PDF report.

Covers work between April 10, 2026 (end of the prior report) and today.
Style matches generate_report.py exactly — same palette, same table
chrome, same typography hierarchy — so the three reports read as a
series.
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

OUTPUT = "/Users/cj/Documents/Projects/go-workspace/Matchup/docs/reports/matchup-share-observability-and-feed.pdf"

styles = getSampleStyleSheet()

# Custom styles — mirror the prior report so the three PDFs read as one series.
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

# Cell styles: used by helpers below so long tokens in tables can
# wrap rather than bleed past the column edge.
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
    return Paragraph(f"\u2022 {text}", styles["RBullet"])


def tc(text, code=False):
    """Table cell. Wraps text in a Paragraph so long tokens break at
    column edges instead of spilling past. Pass code=True for
    monospace (file paths, identifiers)."""
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
    story.append(Paragraph("Home Feed, Observability & Social Sharing — Report",
                           styles["RH2"]))
    story.append(Paragraph(f"Date: {date.today().strftime('%B %d, %Y')}",
                           styles["RSmall"]))
    story.append(Paragraph("Covers work since the prior report (April 10, 2026).",
                           styles["RSmall"]))
    story.append(hr())

    # ── Section 1: Home-Feed Improvements ─────────────────
    story.append(Paragraph("1. Home-Feed Improvements", styles["RH2"]))

    story.append(Paragraph(
        "Five migrations, a scheduler change, two new tabs, and a visual rework of "
        "the card component itself. The home feed is now the primary surface for "
        "discovery and needed to behave that way.", styles["RBody"]))

    story.append(Paragraph("HomeCard — Facebook-Post Restyling", styles["RH3"]))
    story.append(Paragraph(
        "Rebuilt <font face='Courier'>frontend/src/components/HomeCard.js</font> "
        "from a gaming-app layout (image top, stacked body below) to a Facebook-style "
        "post structure: post header (avatar + username + time), caption area (title + "
        "content + tags), full-bleed media, and a flat action bar. Kept the dark theme. "
        "Utility functions (<font face='Courier'>GRADIENTS</font>, "
        "<font face='Courier'>TAG_RULES</font>, <font face='Courier'>deriveTags</font>, "
        "<font face='Courier'>relativeTime</font>, "
        "<font face='Courier'>authorDisplay</font>) preserved.", styles["RBody"]))

    story.append(Paragraph("Trending Tab (migration 010)", styles["RH3"]))
    story.append(Paragraph(
        "Added <font face='Courier'>trending_matchups_snapshot</font> materialized view — "
        "top 9 matchups by hourly engagement (<font face='Courier'>votes\u00d72 + "
        "likes\u00d73 + comments\u00d70.5</font>, owner self-interactions excluded). "
        "Scheduler refreshes it hourly via <font face='Courier'>jobRefreshTrendingSnapshot</font>. "
        "Wired through as <font face='Courier'>trending_matchups</font> on the "
        "<font face='Courier'>HomeSummaryData</font> proto and a "
        "<font face='Courier'>sortMode === 'trending'</font> branch on "
        "<font face='Courier'>HomePage.js</font>.", styles["RBody"]))

    story.append(Paragraph("Most-Played Tab (migration 011)", styles["RH3"]))
    story.append(Paragraph(
        "Similar MV to trending but ranked purely by vote count in the past hour — not "
        "engagement-weighted. Added <font face='Courier'>most_played_snapshot</font> + "
        "<font face='Courier'>most_played_matchups</font> proto field. Frontend "
        "<font face='Courier'>sortMode === 'most-played'</font> now reads from the home "
        "summary response instead of calling <font face='Courier'>getPopularMatchups()</font>, "
        "which let us drop one network round-trip per home-page load.", styles["RBody"]))

    story.append(Paragraph("Status Filter Hardening (migration 012)", styles["RH3"]))
    story.append(Paragraph(
        "Two silent leaks: <font face='Courier'>ListMatchups</font> had no status filter "
        "(drafts leaked into the Latest tab), and "
        "<font face='Courier'>home_new_this_week_snapshot</font> was missing "
        "<font face='Courier'>WHERE status IN ('active','published')</font>. Fixed at both "
        "layers — SQL filter in the MV and the <font face='Courier'>ListMatchups</font> "
        "query, plus a runtime "
        "<font face='Courier'>matchup.Status != \"active\"</font> guard in the home handler "
        "for the window between MV refreshes. Regression tests lock all three paths.",
        styles["RBody"]))

    story.append(Paragraph("Bracket Engagement-Filter Fix (migration 013)", styles["RH3"]))
    story.append(Paragraph(
        "<font face='Courier'>popular_brackets_snapshot</font> had "
        "<font face='Courier'>WHERE engagement_score &gt; 0</font>, which hid any "
        "newly-created active bracket until it received at least one like/vote/comment. "
        "Removed the filter — ROW_NUMBER + LIMIT 12 still caps the list and still "
        "prioritizes popular brackets first, but new active ones now surface on the "
        "homepage immediately.", styles["RBody"]))

    story.append(Paragraph("Embedded Scheduler for Local Dev", styles["RH3"]))
    story.append(Paragraph(
        "<font face='Courier'>cmd/cron/main.go</font> runs the scheduler as its own k8s "
        "deployment in production, but local dev had to start a second terminal for "
        "anything involving MV refresh. Added <font face='Courier'>EMBED_SCHEDULER=true</font> "
        "env var: when set, <font face='Courier'>api.Run()</font> spawns "
        "<font face='Courier'>scheduler.New(&server).Run(ctx)</font> in a goroutine so a "
        "single <font face='Courier'>go run cmd/main.go</font> covers the API + all cron "
        "workloads. Off by default; production keeps the cron pod separate.",
        styles["RBody"]))

    story.append(Paragraph("Author + Created-At on Popular Data", styles["RH3"]))
    story.append(Paragraph(
        "<font face='Courier'>PopularMatchupData</font> and "
        "<font face='Courier'>PopularBracketData</font> now carry "
        "<font face='Courier'>author_username</font> and "
        "<font face='Courier'>created_at</font> so home-feed cards can show "
        "<i>@username · 2h ago</i> without an extra fetch. Threaded end-to-end: proto \u2192 "
        "DTO \u2192 mapper \u2192 handler (all four paths: popular/trending/most-played "
        "matchups plus popular brackets) \u2192 <font face='Courier'>authorDisplay()</font> "
        "fallback chain in HomeCard.", styles["RBody"]))

    story.append(Paragraph("Bracket Page — Owner Link", styles["RH3"]))
    story.append(Paragraph(
        "<font face='Courier'>BracketData</font> gained a nested "
        "<font face='Courier'>author</font> submessage "
        "(<font face='Courier'>common.v1.UserSummaryResponse</font>) mirroring "
        "<font face='Courier'>MatchupData</font>. <font face='Courier'>BracketPage.js</font> "
        "renders the bracket owner as a small linked handle in the overline slot above the "
        "title \u2014 <i>TOURNAMENT SNAPSHOT \u00b7 BY @cordell</i>.", styles["RBody"]))

    feed_data = [
        [tc("010_trending_matchups.sql", code=True), tc("Trending MV, hourly refresh")],
        [tc("011_most_played.sql", code=True), tc("Most-played MV, hourly refresh")],
        [tc("012_fix_new_this_week_status.sql", code=True), tc("New-this-week status filter")],
        [tc("013_bracket_snapshot_no_engagement_filter.sql", code=True), tc("Surface new active brackets")],
        [tc("014_add_short_id.sql", code=True), tc("Short IDs for share URLs (see \u00a74)")],
    ]
    story.append(Paragraph("Migrations Added", styles["RH3"]))
    story.append(make_table(
        ["Migration", "Purpose"],
        feed_data,
        col_widths=[3.6*inch, 2.9*inch]
    ))

    story.append(PageBreak())

    # ── Section 2: Observability ─────────────────────────
    story.append(Paragraph("2. Observability \u2014 Prometheus + Grafana", styles["RH2"]))

    story.append(Paragraph(
        "Production observability is a check-the-box requirement once user-facing latency "
        "matters. Chose Prometheus + Grafana over a paid SaaS because every part of the "
        "stack is self-hosted, free, and fits the existing k8s manifest style.",
        styles["RBody"]))

    story.append(Paragraph("Go API Instrumentation", styles["RH3"]))
    story.append(Paragraph(
        "Added <font face='Courier'>github.com/prometheus/client_golang</font>. "
        "<font face='Courier'>api/middlewares/metrics.go</font> provides a chi middleware "
        "that records two series per request \u2014 "
        "<font face='Courier'>http_requests_total{method,route,status}</font> counter and "
        "<font face='Courier'>http_request_duration_seconds{method,route}</font> histogram. "
        "Route labels come from chi\u2019s "
        "<font face='Courier'>chi.RouteContext(ctx).RoutePattern()</font> (the template, "
        "not the raw URL) to keep cardinality bounded; unmatched paths bucket as "
        "<font face='Courier'>\"unmatched\"</font>; the <font face='Courier'>/metrics</font> "
        "endpoint self-skips to prevent a feedback loop.", styles["RBody"]))

    story.append(Paragraph(
        "<font face='Courier'>/metrics</font> is mounted via "
        "<font face='Courier'>promhttp.Handler()</font> \u2014 exposes the HTTP metrics "
        "plus the default Go runtime metrics (goroutines, GC pauses, heap, file "
        "descriptors) for free.", styles["RBody"]))

    story.append(Paragraph("Local-Dev Compose", styles["RH3"]))
    story.append(Paragraph(
        "Added <font face='Courier'>prometheus</font> and <font face='Courier'>grafana</font> "
        "services to <font face='Courier'>docker-compose.yml</font>. Configs live in a new "
        "<font face='Courier'>observability/</font> directory at the repo root:",
        styles["RBody"]))
    story.append(bullet("<font face='Courier'>observability/prometheus.yml</font> \u2014 scrape config pointing at <font face='Courier'>host.docker.internal:8888</font> (Docker-for-Mac DNS for the host)"))
    story.append(bullet("<font face='Courier'>observability/grafana/provisioning/datasources/prometheus.yml</font> \u2014 auto-wires the Prometheus datasource on boot"))
    story.append(bullet("<font face='Courier'>observability/grafana/provisioning/dashboards/dashboards.yml</font> \u2014 points Grafana at the dashboards directory"))
    story.append(bullet("<font face='Courier'>observability/grafana/dashboards/matchup-api.json</font> \u2014 starter dashboard with request rate, p50/p95/p99 latency, HTTP status breakdown, goroutines + heap"))

    story.append(Paragraph("Kubernetes Manifests", styles["RH3"]))
    story.append(Paragraph(
        "Two new directories under <font face='Courier'>k8s/</font> mirror the compose "
        "setup but for in-cluster deployment:", styles["RBody"]))

    k8s_data = [
        [tc("k8s/prometheus/configmap.yaml", code=True), tc("Scrape config as ConfigMap (mounted at /etc/prometheus)")],
        [tc("k8s/prometheus/deployment.yaml", code=True), tc("Recreate-strategy, nonroot fsGroup, readiness on /-/ready")],
        [tc("k8s/prometheus/service.yaml", code=True), tc("ClusterIP on 9090")],
        [tc("k8s/prometheus/pvc.yaml", code=True), tc("20Gi TSDB storage, 15d retention")],
        [tc("k8s/grafana/configmap.yaml", code=True), tc("Provisioning (datasource + dashboard provider)")],
        [tc("k8s/grafana/dashboard-configmap.yaml", code=True), tc("Dashboard JSON inlined, deployable via kubectl apply")],
        [tc("k8s/grafana/deployment.yaml", code=True), tc("Grafana 11.2.0 with admin creds from matchup-secrets")],
        [tc("k8s/grafana/service.yaml", code=True), tc("ClusterIP on 3000")],
        [tc("k8s/grafana/pvc.yaml", code=True), tc("1Gi sqlite state")],
    ]
    story.append(make_table(
        ["File", "Role"],
        k8s_data,
        col_widths=[2.6*inch, 3.9*inch]
    ))

    story.append(Paragraph("Tests for the Metrics Middleware", styles["RH3"]))
    story.append(Paragraph(
        "<font face='Courier'>middlewares/metrics_test.go</font> adds 5 cases: route "
        "pattern collapsing (<font face='Courier'>/users/1</font> and "
        "<font face='Courier'>/users/2</font> collapse into "
        "<font face='Courier'>/users/{id}</font>), status-code recording, "
        "<font face='Courier'>/metrics</font>-endpoint self-skip (prevents feedback loop), "
        "unmatched-route bucketing (cardinality defense), and duration-histogram "
        "observation. <font face='Courier'>middlewares</font> coverage moved "
        "62.4% \u2192 67.5%.", styles["RBody"]))

    story.append(PageBreak())

    # ── Section 3: Test Coverage Expansion ────────────────
    story.append(Paragraph("3. Test Coverage Expansion", styles["RH2"]))

    story.append(Paragraph(
        "The prior report closed at 147 tests / 25.8% overall coverage. Since then the "
        "home-feed handler and the bracket handler \u2014 both of which had zero dedicated "
        "integration tests \u2014 gained full suites, plus the share package shipped with "
        "its own tests, plus regression coverage for every status-filter fix in \u00a71.",
        styles["RBody"]))

    test_add = [
        [tc("home_connect_handler_integration_test.go", code=True), tc("10"), tc("Empty DB; popular brackets (new active + status filter); creators-skip-viewer; new-this-week (status + bracket-children); private user hidden from stranger; admin bypass; runtime status guard; RFC3339")],
        [tc("bracket_connect_handler_integration_test.go", code=True), tc("12"), tc("Create (success/unauth); Get (success/not-found); Update + Delete (owner + forbidden); GetPopularBrackets (status filter + author+time); GetUserBrackets; GetBracketSummary")],
        [tc("matchup_connect_handler_integration_test.go", code=True), tc("+2"), tc("ListMatchups status filter + pagination total")],
        [tc("proto_mappers_test.go", code=True), tc("+2"), tc("PopularMatchupToProto / PopularBracketToProto with new author + created_at fields")],
        [tc("middlewares/metrics_test.go", code=True), tc("5"), tc("Metrics middleware coverage (\u00a72)")],
        [tc("db/db_test.go", code=True), tc("+3"), tc("GenerateShortID length+alphabet, no-collisions-over-50k, IsUniqueViolation")],
        [tc("controllers/share/*", code=True), tc("16"), tc("Bot UA matrix; validShortID; render shape/size; gradient stability; XSS hardening; rate-limit bucket isolation")],
    ]
    story.append(make_table(
        ["File", "New Tests", "Coverage"],
        test_add,
        col_widths=[2.6*inch, 0.8*inch, 3.1*inch]
    ))
    story.append(Spacer(1, 6))

    coverage_data = [
        [tc("Total tests"), tc("147"), tc("173"), tc("+26")],
        [tc("controllers (package)"), tc("24.3%"), tc("35.6%"), tc("+11.3 pp")],
        [tc("controllers/share (new)"), tc("\u2014"), tc("48.9%"), tc("new")],
        [tc("db"), tc("62.5%"), tc("68.9%"), tc("+6.4 pp")],
        [tc("middlewares"), tc("62.4%"), tc("67.5%"), tc("+5.1 pp")],
        [tc("security / utils (unchanged)"), tc("100%"), tc("100%"), tc("\u2014")],
    ]
    story.append(Paragraph("Coverage Deltas", styles["RH3"]))
    story.append(make_table(
        ["Metric", "Before", "After", "Change"],
        coverage_data,
        col_widths=[2.5*inch, 1*inch, 1*inch, 1*inch]
    ))
    story.append(Spacer(1, 6))

    story.append(Paragraph(
        "Every new test was written to lock down a specific change from this cycle rather "
        "than chase coverage numbers. Migration 013\u2019s engagement-filter removal, "
        "migration 012\u2019s status filter, the runtime <font face='Courier'>matchup.Status "
        "!= \"active\"</font> guard, XSS inside the JSON-LD <font face='Courier'>&lt;script&gt;</font> "
        "block, the short-ID unique-violation retry path \u2014 every one of those has a "
        "named test that will fail the moment someone unwinds the fix.", styles["RBody"]))

    story.append(PageBreak())

    # ── Section 4: Share Feature — Backend ───────────────
    story.append(Paragraph("4. Share Feature \u2014 Core Infrastructure", styles["RH2"]))

    story.append(Paragraph(
        "The centerpiece of this release. A shared Matchup link now renders as a rich "
        "preview card on every major platform \u2014 iMessage, X/Twitter, Discord, Slack, "
        "WhatsApp, Telegram, LinkedIn, Facebook, Reddit \u2014 driven by server-side "
        "Open Graph HTML plus a per-matchup generated PNG.", styles["RBody"]))

    story.append(Paragraph("Short IDs (migration 014)", styles["RH3"]))
    story.append(Paragraph(
        "Public IDs are 36-char UUIDs \u2014 ugly in share URLs. Added "
        "<font face='Courier'>short_id TEXT UNIQUE</font> on "
        "<font face='Courier'>matchups</font> and <font face='Courier'>brackets</font> "
        "with a partial index <font face='Courier'>WHERE short_id IS NOT NULL</font> so "
        "the backfill doesn\u2019t fight a NOT NULL constraint. "
        "<font face='Courier'>GenerateShortID()</font> in "
        "<font face='Courier'>api/db/db.go</font> produces 8-char base62 IDs from "
        "<font face='Courier'>crypto/rand</font> (\u2248 2.18\u00d710<sup>14</sup> space). "
        "<font face='Courier'>IsUniqueViolation(err)</font> wraps the Postgres SQLSTATE "
        "23505 check so the create handlers can retry cleanly \u2014 3 attempts per row, "
        "collision odds are astronomically low but we handle them anyway.",
        styles["RBody"]))

    story.append(Paragraph("Backfill Binary", styles["RH3"]))
    story.append(Paragraph(
        "<font face='Courier'>api/cmd/backfill_short_ids/main.go</font>: one-shot binary "
        "that scans matchups + brackets where <font face='Courier'>short_id IS NULL</font> "
        "in 1000-row batches, generates IDs, and <font face='Courier'>UPDATE \u2026 WHERE "
        "id = $1 AND short_id IS NULL</font>. Safe to re-run; handles collisions with the "
        "same retry path. Initial run populated 5 rows (4 matchups + 1 bracket).",
        styles["RBody"]))

    story.append(Paragraph("The share/ Package", styles["RH3"]))
    share_files = [
        [tc("share/bot_detect.go", code=True), tc("User-Agent substring match against 21 crawler patterns (Twitterbot, facebookexternalhit, Discordbot, Slackbot, WhatsApp, etc.) plus a <font face='Courier'>?_meta=1</font> escape hatch")],
        [tc("share/html.go", code=True), tc("text/template HTML with full og:*, twitter:*, JSON-LD schema.org. og:image URL versioned with updated_at for Facebook's 30-day cache bust")],
        [tc("share/image.go", code=True), tc("1200\u00d7630 PNG via fogleman/gg. See \u00a75 for the design.")],
        [tc("share/fonts.go", code=True), tc("Go's built-in Go Sans (goregular + gobold) embedded via x/image/font. Zero external font files to vendor")],
        [tc("share/cache.go", code=True), tc("Redis keys <font face='Courier'>og:{m|b}:{id}:{updated_at}</font> with 7-day TTL. Edits auto-invalidate. Negative cache <font face='Courier'>og:miss:{id}</font> (60s) absorbs 404 hammers")],
        [tc("share/ratelimit.go", code=True), tc("Token bucket: 60 rps global + 10 rps per short_id, burst 20/5")],
        [tc("share/handler.go", code=True), tc("chi handlers: <font face='Courier'>/m/{id}</font>, <font face='Courier'>/b/{id}</font> (bot\u2192OG HTML, human\u2192302), <font face='Courier'>/og/m/{id}.png</font>, <font face='Courier'>/og/b/{id}.png</font> (cached PNG)")],
    ]
    story.append(make_table(
        ["File", "Role"],
        share_files,
        col_widths=[1.5*inch, 5*inch]
    ))
    story.append(Spacer(1, 6))

    story.append(Paragraph("Request Flow", styles["RH3"]))
    story.append(Paragraph(
        "Chi routes are mounted <i>before</i> ConnectRPC so "
        "<font face='Courier'>/m/{id}</font> and friends aren\u2019t swallowed by any "
        "catch-all. The handler first validates the short ID (length 8, base62 alphabet "
        "only) \u2014 stops SQL injection attempts and garbage inputs before they hit "
        "the DB.", styles["RBody"]))
    story.append(bullet("Bot UA \u2192 OG HTML response (<font face='Courier'>text/html; charset=utf-8</font>, <font face='Courier'>Cache-Control: max-age=600</font>)"))
    story.append(bullet("Human UA \u2192 <font face='Courier'>302</font> to the SPA route, carrying any <font face='Courier'>?ref=\u2026</font> query through"))
    story.append(bullet("Private author \u2192 generic &ldquo;Private on Matchup&rdquo; card; no title leak"))
    story.append(bullet("Deleted or unknown short_id \u2192 fallback wordmark card (not 404 \u2014 crawlers cache 404s aggressively)"))

    story.append(Paragraph("Hardening", styles["RH3"]))
    story.append(bullet("<b>XSS defense in JSON-LD.</b> Inside <font face='Courier'>&lt;script type=\"application/ld+json\"&gt;</font>, user-supplied titles are escaped to <font face='Courier'>\\u003c</font> / <font face='Courier'>\\u003e</font> / <font face='Courier'>\\u0026</font> so <font face='Courier'>&lt;/script&gt;</font> in a title can\u2019t break out of the block. Regression test covers it"))
    story.append(bullet("<b>CORS on PNG.</b> <font face='Courier'>Access-Control-Allow-Origin: *</font> on <font face='Courier'>/og/*.png</font> (Twitter\u2019s inline image fetch needs it)"))
    story.append(bullet("<b>Facebook 30d cache bust.</b> <font face='Courier'>og:image</font> URL carries <font face='Courier'>?v={updated_at_unix}</font> so edits unfurl correctly"))
    story.append(bullet("<b>iMessage 1 MB cap.</b> Default PNG compression keeps files \u223c 50 KB, well under the limit"))

    story.append(PageBreak())

    # ── Section 5: Share Feature — Image Design ───────────
    story.append(Paragraph("5. Share Feature \u2014 Image Design", styles["RH2"]))

    story.append(Paragraph(
        "The first-pass design was visually busy \u2014 gradient background, wordmark, "
        "kind pill, auto-fit title, &ldquo;A vs B&rdquo; line, QR code, byline \u2014 "
        "and at iMessage\u2019s preview size the details blurred together. Pivoted to an "
        "X/Twitter-style dark card after comparing Reddit and X reference previews.",
        styles["RBody"]))

    design_diff = [
        [tc("Background"), tc("Full vertical gradient"), tc("Solid dark navy + 10px gradient accent stripe on top")],
        [tc("Brand"), tc("Large wordmark top-left"), tc("Small MATCHUP.APP in indigo, top-left")],
        [tc("Kind pill"), tc("Solid white 15% fill"), tc("Tinted bg + bordered indigo pill")],
        [tc("Title"), tc("Auto-fit 80\u219248px, centered"), tc("Auto-fit 76\u219244px, left-aligned, fixed baseline")],
        [tc("Contenders"), tc("\u201cA vs B\u201d inline + tiny votes"), tc("Two tinted tiles side-by-side with bold name + big vote count, leader bar below")],
        [tc("Bracket subtitle"), tc("\u201cSize N \u00b7 Round M\u201d"), tc("Description (2-line) + \u201cRound M\u201d accent line")],
        [tc("Author"), tc("Tiny byline text bottom-left"), tc("Colored avatar circle + @username + middle-dot engagement line: 42 votes \u00b7 5 likes \u00b7 3 comments")],
        [tc("QR code"), tc("Present (bottom-right)"), tc("Removed (X doesn\u2019t have one \u2014 cleaner)")],
    ]
    story.append(Paragraph("Layout Changes (v1 \u2192 v2 dark card)", styles["RH3"]))
    story.append(make_table(
        ["Element", "v1", "v2 (shipped)"],
        design_diff,
        col_widths=[1.2*inch, 2*inch, 3.3*inch]
    ))
    story.append(Spacer(1, 6))

    story.append(Paragraph("Implementation Notes", styles["RH3"]))
    story.append(bullet("<b>Library:</b> <font face='Courier'>fogleman/gg</font>. Pixel-native API, pure Go, native image encoding via <font face='Courier'>image/png</font>. Evaluated <font face='Courier'>tdewolff/canvas</font> (pulled in a TeX rendering stack \u2014 too heavy) and <font face='Courier'>kanrichan/resvg-go</font> (WASM + font-shaping gaps for complex scripts) before picking"))
    story.append(bullet("<b>Color identity:</b> each title hashes (via FNV-32a) to a fixed index into an 8-entry gradient palette matching the home-feed cards \u2014 same matchup gets the same accent colors across home card, share preview, and any future inline embed"))
    story.append(bullet("<b>Auto-fit title:</b> wraps to 2 lines max, shrinks from 76\u21e644px until it fits, hard-truncates with ellipsis as a last resort"))
    story.append(bullet("<b>Leader bar:</b> proportional fill using item A\u2019s share of total votes, drawn in item A\u2019s tint color. Absent when no votes yet"))
    story.append(bullet("<b>Avatar:</b> tinted circle with the author\u2019s uppercase initial \u2014 no external image fetch, consistent branding, works offline"))
    story.append(bullet("<b>Engagement line:</b> middle-dot separated, only includes pieces with non-zero counts. <font face='Courier'>formatShortCount(4200) \u2192 \"4.2K\"</font>, <font face='Courier'>pluralize</font> handles 1-vs-many"))
    story.append(bullet("<b>No extra DB work:</b> <font face='Courier'>Likes</font> and <font face='Courier'>Comments</font> come from the existing denormalized <font face='Courier'>likes_count</font> / <font face='Courier'>comments_count</font> columns (migration 006)"))

    story.append(PageBreak())

    # ── Section 6: Share Feature — Frontend & Attribution ─
    story.append(Paragraph("6. Share Feature \u2014 Frontend & Attribution", styles["RH2"]))

    story.append(Paragraph("ShareButton Component", styles["RH3"]))
    story.append(Paragraph(
        "<font face='Courier'>frontend/src/components/ShareButton.js</font> detects "
        "<font face='Courier'>navigator.share</font> at mount. On mobile it opens the OS "
        "share sheet (iMessage, Instagram DM, whatever\u2019s installed); on desktop it "
        "drops a menu with six targets: Copy Link, X/Twitter, Facebook, LinkedIn, Email "
        "(mailto:), SMS (sms:?&amp;body=). Every outgoing URL carries a "
        "<font face='Courier'>?ref=\u2026</font> parameter so the landing beacon knows "
        "the channel. Pre-filled text adapts to the viewer\u2019s current vote: "
        "<i>&ldquo;I\u2019m team Kendrick on this matchup \u2014 you?&rdquo;</i> if they\u2019ve "
        "voted, a generic prompt otherwise. Mounted on "
        "<font face='Courier'>MatchupPage</font> and "
        "<font face='Courier'>BracketPage</font>.", styles["RBody"]))

    story.append(Paragraph("useShareTracking Hook", styles["RH3"]))
    story.append(Paragraph(
        "<font face='Courier'>frontend/src/hooks/useShareTracking.js</font>: on mount, "
        "reads <font face='Courier'>?ref=</font> from the URL, validates against a fixed "
        "allow-list (<font face='Courier'>tw, fb, li, copy, native, email, sms, ig, dm, "
        "discord, slack</font>), fires a <font face='Courier'>navigator.sendBeacon</font> "
        "to <font face='Courier'>/share/landed</font>, strips the query via "
        "<font face='Courier'>history.replaceState</font>, and stores the ref in "
        "localStorage with a 30-day TTL. That stored ref is available to the signup flow "
        "later so conversions can attribute back to the first share channel.",
        styles["RBody"]))

    story.append(Paragraph("/share/landed Beacon", styles["RH3"]))
    story.append(Paragraph(
        "<font face='Courier'>api/controllers/share_tracking_handler.go</font>: plain "
        "chi <font face='Courier'>POST /share/landed</font>, intentionally non-Connect "
        "because <font face='Courier'>sendBeacon</font> fire-and-forget ignores response "
        "codes anyway. Increments a Prometheus counter "
        "<font face='Courier'>share_landed_total{source, content_type}</font>. Both labels "
        "are validated against fixed allow-lists before use \u2014 nobody can blow up "
        "cardinality by POSTing arbitrary strings. Unknown sources bucket as "
        "<font face='Courier'>\"other\"</font>.", styles["RBody"]))

    story.append(Paragraph("Fallback OG Tags + Deep-Link Scaffolding", styles["RH3"]))
    story.append(bullet("<font face='Courier'>frontend/public/index.html</font> now carries default <font face='Courier'>og:type</font>, <font face='Courier'>og:site_name</font>, <font face='Courier'>og:title</font>, <font face='Courier'>og:description</font>, and <font face='Courier'>og:image</font> tags for non-share pages (homepage, profile, etc.)"))
    story.append(bullet("<font face='Courier'>frontend/nginx.conf</font> serves empty-but-valid JSON from <font face='Courier'>/.well-known/apple-app-site-association</font> and <font face='Courier'>/.well-known/assetlinks.json</font>. Stages the Universal / App Links surface so a future native app silently upgrades web links without a coordinated web release"))

    # ── Section 7: Testing & Production Roadmap ───────────
    story.append(Paragraph("7. Local-Dev Testing & Production Roadmap", styles["RH2"]))

    story.append(Paragraph("Testing the Unfurl Locally", styles["RH3"]))
    story.append(Paragraph(
        "To test how a link unfurls in a real messaging app, the tunnel needs a publicly "
        "reachable hostname \u2014 social crawlers can\u2019t hit localhost. Running two "
        "concurrent Cloudflare <i>quick</i> tunnels from the same machine tripped "
        "Cloudflare\u2019s connect-control protection, so the architecture simplified to "
        "one tunnel + a reverse proxy in the Go API.", styles["RBody"]))
    story.append(bullet("<b>cloudflared</b> installed via Homebrew; run with <font face='Courier'>--protocol http2</font> to sidestep QUIC flakiness on some networks"))
    story.append(bullet("<b>FRONTEND_PROXY env</b> added to <font face='Courier'>api.Run()</font> \u2014 when set (e.g. <font face='Courier'>http://localhost:3000</font>), chi\u2019s <font face='Courier'>NotFound</font> handler forwards unmatched routes to the React dev server via <font face='Courier'>httputil.NewSingleHostReverseProxy</font>. Off in production"))
    story.append(bullet("<b>PUBLIC_ORIGIN env</b> sets the hostname bots see in <font face='Courier'>og:url</font>/<font face='Courier'>og:image</font>. Points at the tunnel URL while testing; points at the real domain in production"))
    story.append(bullet("<b>Pre-warm.</b> Hitting <font face='Courier'>/og/m/{id}.png</font> through the tunnel once caches the PNG in Redis so the first real crawler sees no rendering latency"))

    story.append(Paragraph("Production Roadmap", styles["RH3"]))
    prod_stages = [
        [tc("Now"), tc("*.trycloudflare.com (rotates on restart)"), tc("None"), tc("$0")],
        [tc("Stable dev/demo"), tc("yourdomain.com via named tunnel"), tc("<font face='Courier'>cloudflared tunnel create matchup</font> \u2014 hostname persists"), tc("~$15/yr<br/>(domain only)")],
        [tc("Production"), tc("yourdomain.com on real hosting"), tc("Managed k8s / Render / Fly + TLS ingress + DNS"), tc("$15/yr<br/>+ ~$7\u201315/mo")],
    ]
    story.append(make_table(
        ["Stage", "Hostname", "Change", "Cost"],
        prod_stages,
        col_widths=[1*inch, 1.9*inch, 2.4*inch, 1.2*inch]
    ))
    story.append(Spacer(1, 6))
    story.append(Paragraph(
        "The only line of configuration that changes across the three stages is "
        "<font face='Courier'>PUBLIC_ORIGIN</font> in the API env. The short-ID format, "
        "the crawler logic, the PNG renderer, and the frontend share UI are all domain-agnostic.",
        styles["RBody"]))

    story.append(PageBreak())

    # ── Section 8: Current Project State ─────────────────
    story.append(Paragraph("8. Current Project State", styles["RH2"]))

    state_data = [
        [tc("Total Go tests"), tc("173 (up from 147)")],
        [tc("controllers coverage"), tc("35.6% (up from 24.3%)")],
        [tc("controllers/share coverage"), tc("48.9% (new package)")],
        [tc("db coverage"), tc("68.9% (up from 62.5%)")],
        [tc("middlewares coverage"), tc("67.5% (up from 62.4%)")],
        [tc("Packages at 100%"), tc("security, utils/fileformat, utils/httpctx")],
        [tc("Migrations"), tc("14 total (5 added this cycle: 010\u2013014)")],
        [tc("New Go packages"), tc("controllers/share")],
        [tc("New Go dependencies"), tc("prometheus/client_golang, fogleman/gg")],
        [tc("New endpoints"), tc("<font face='Courier'>/metrics</font>, <font face='Courier'>/m/{id}</font>, <font face='Courier'>/b/{id}</font>, <font face='Courier'>/og/m/{id}.png</font>, <font face='Courier'>/og/b/{id}.png</font>, <font face='Courier'>/share/landed</font>")],
        [tc("New env vars"), tc("EMBED_SCHEDULER, PUBLIC_ORIGIN, FRONTEND_PROXY")],
        [tc("Build status"), tc("Clean (go build, go vet, go test -race, integration)")],
    ]
    story.append(make_table(
        ["Metric", "Value"],
        state_data,
        col_widths=[2.2*inch, 4.3*inch]
    ))
    story.append(Spacer(1, 10))

    story.append(Paragraph("Deferred From This Cycle (Documented, Not Urgent)", styles["RH3"]))
    story.append(bullet("<b>OG image pre-warm job.</b> Enqueue a render on matchup/bracket create so the very first share doesn\u2019t pay cold-render latency. Simple, but first-share latency isn\u2019t showing up as a real issue yet in Prometheus"))
    story.append(bullet("<b>OG image metrics.</b> Add a counter for cache hits vs. misses vs. negative hits. Useful for tuning TTLs once there\u2019s real traffic"))
    story.append(bullet("<b>Grafana dashboard for share metrics.</b> <font face='Courier'>share_landed_total</font> is there but no panel exists yet. One-liner to add"))
    story.append(bullet("<b>Short-ID backfill follow-up migration.</b> Flip <font face='Courier'>short_id</font> to <font face='Courier'>NOT NULL</font> after confirming <font face='Courier'>COUNT(*) WHERE short_id IS NULL = 0</font> across every environment"))
    story.append(bullet("<b>oEmbed + <font face='Courier'>/embed/m/{id}</font>.</b> Substack / Notion / Ghost auto-embed on paste when served. Scaffolded in the plan, not yet implemented"))
    story.append(bullet("<b>Named tunnel for stable dev demos.</b> <font face='Courier'>cloudflared tunnel create</font> once the primary domain is on Cloudflare DNS, so test URLs stop rotating on restart"))

    story.append(Spacer(1, 10))
    story.append(hr())
    story.append(Paragraph(
        "<i>Generated by Claude Code. All changes verified with full test suite and "
        "end-to-end tunnel tests against real social-platform crawlers.</i>",
        styles["RSmall"]))

    doc.build(story)
    print(f"PDF written to {OUTPUT}")


if __name__ == "__main__":
    build()
