#!/usr/bin/env python3
"""Generate the Item-Thumbnails PDF report.

Single-commit cycle (3f7c799) — last of three UX-backlog items
from the bracket-research pass. Adds per-contender thumbnails on
matchups + bracket rows. Also rolls in the small comment-typography
hierarchy fix (commit d286274) that landed between this cycle and
the previous skip-button cycle.
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

OUTPUT = "/Users/cj/Documents/Projects/go-workspace/Matchup/docs/reports/matchup-item-thumbnails-and-comment-hierarchy.pdf"

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
    story.append(Paragraph("Item Thumbnails & Comment Hierarchy — Report",
                           styles["RH2"]))
    story.append(Paragraph(f"Date: {date.today().strftime('%B %d, %Y')}",
                           styles["RSmall"]))
    story.append(Paragraph(
        "Closes the bracket-research UX backlog with the third and final "
        "cycle: per-contender thumbnails. Folds in a small comment-"
        "typography hierarchy fix that browser-Claude flagged on a prod "
        "review pass between cycles. Migration 026, full upload pipeline "
        "extension, three-way display rollout, no breaking changes.",
        styles["RSmall"]))
    story.append(hr())

    # ── Section 1: Why ───────────────────────────────────
    story.append(Paragraph("1. Why Thumbnails", styles["RH2"]))
    story.append(Paragraph(
        "The pairwise-comparison literature browser-Claude pulled in during the bracket-research "
        "pass calls out visual richness as a measurable engagement lever. A contender card with the "
        "actual Blade Runner poster reads instantly; a card that just says \"Blade Runner 2049\" "
        "forces a moment of cognitive translation — particularly for users skimming on mobile. "
        "Thumbnails close that gap.", styles["RBody"]))
    story.append(Paragraph(
        "Last and largest of the three UX-backlog cycles. Required a real schema migration, an "
        "extension of the existing upload pipeline, model + proto + handler updates on three RPCs, "
        "and a per-item file picker on the create-matchup form.", styles["RBody"]))

    # ── Section 2: Reuse vs new ──────────────────────────
    story.append(Paragraph("2. Reuse Decision — Extend Existing Upload Pipeline", styles["RH2"]))
    story.append(Paragraph(
        "The codebase already had a clean three-step S3 upload flow: "
        "<font face='Courier'>PresignUpload(kind, content_type)</font> returns a signed PUT URL; "
        "the client PUTs raw bytes to S3; the kind-specific commit handler (e.g. CreateMatchup) "
        "calls <font face='Courier'>CommitUploadedObject</font> to validate + fetch the bytes "
        "and run the resize pipeline. The kind registry is a typed map keyed by "
        "<font face='Courier'>UploadKind</font> with per-kind size + content-type rules.",
        styles["RBody"]))
    story.append(Paragraph(
        "Adding item thumbnails is one new <font face='Courier'>UploadKind</font> constant + one "
        "new entry in the kind registry + a parallel S3 prefix. No new RPCs, no new helpers — every "
        "moving part already exists. Score one for the prior architectural work.",
        styles["RBody"]))

    # ── Section 3: Schema ────────────────────────────────
    story.append(Paragraph("3. Schema (migration 026)", styles["RH2"]))
    story.append(Paragraph(
        "Single ALTER TABLE on <font face='Courier'>matchup_items</font>:",
        styles["RBody"]))
    story.append(bullet("<font face='Courier'>ADD COLUMN image_path text NOT NULL DEFAULT ''</font>. "
                        "Mirrors <font face='Courier'>matchups.image_path</font> on purpose: same "
                        "shape, same default, same upload pipeline, same path-to-URL transform."))
    story.append(bullet("Existing items backfill to <font face='Courier'>''</font> — non-breaking. "
                        "Empty paths stay empty; the proto mapper produces empty "
                        "<font face='Courier'>image_url</font>; the frontend's \"no thumbnail\" "
                        "fallback renders the existing text-only card unchanged."))
    story.append(bullet("New S3 prefix <font face='Courier'>MatchupItemImages/</font> separates item "
                        "thumbnails from matchup covers for distinct lifecycle / cache rules."))

    # ── Section 4: Backend ───────────────────────────────
    story.append(Paragraph("4. Backend Changes", styles["RH2"]))

    story.append(Paragraph("Upload kind", styles["RH3"]))
    bk = [
        [tc("Constant"), tc("Cap"), tc("Notes")],
        [tc("UploadKindMatchupItem"), tc("2 MB"), tc("Smaller than the 5 MB matchup-cover ceiling. Items are tile-sized; full-res isn't useful.")],
        [tc("(reused) Allowed types"), tc("image/jpeg, image/png, image/webp, image/gif"), tc("Same set as covers + avatars.")],
    ]
    story.append(make_table(bk[0], bk[1:], col_widths=[2.0*inch, 1.6*inch, 2.9*inch]))

    story.append(Paragraph("Path-to-URL helpers (db/db.go)", styles["RH3"]))
    story.append(bullet("New <font face='Courier'>ProcessMatchupItemImagePath</font> + "
                        "<font face='Courier'>*Sized</font> variants."))
    story.append(bullet("Both delegate to the existing "
                        "<font face='Courier'>processImagePathSized</font> helper with prefix "
                        "<font face='Courier'>\"MatchupItemImages/\"</font>. Consistent URL shape: "
                        "<font face='Courier'>https://{bucket}.s3.{region}.amazonaws.com/MatchupItemImages/{file}</font>."))

    story.append(Paragraph("Handler updates", styles["RH3"]))
    handlers = [
        [tc("RPC"), tc("Behaviour added")],
        [tc("CreateMatchup"), tc("Walks req.Msg.Items; for each item with upload_key, commits + resizes + stores image_path BEFORE SaveMatchup. Atomic — failed S3 commit fails CreateMatchup with no orphan rows.")],
        [tc("AddItem"), tc("Accepts optional upload_key. Same commit-before-INSERT pattern. INSERT now includes image_path column. MatchupItemHandler gains S3Client field.")],
        [tc("UpdateItem"), tc("Accepts optional upload_key for image replacement. Empty key preserves existing image (clearing requires a follow-up flag).")],
        [tc("matchupItemToProto"), tc("Lifts ImagePath to a full S3 URL via ProcessMatchupItemImagePath; empty stays empty so the frontend's no-thumb fallback works unchanged.")],
    ]
    story.append(make_table(handlers[0], handlers[1:], col_widths=[1.7*inch, 4.8*inch]))

    story.append(Paragraph("Atomicity story", styles["RH3"]))
    story.append(Paragraph(
        "Item uploads commit BEFORE the matchup INSERT/SaveMatchup write, in the same handler call. "
        "If any item's commit fails, the request returns Connect error without writing anything to "
        "the DB. No half-saved matchups. The temp upload key gets reaped on success via "
        "<font face='Courier'>DeleteUploadedObject</font>; failures leave the temp blob for the 24h "
        "S3 lifecycle rule to sweep up. Same pattern the matchup-cover upload already uses.",
        styles["RBody"]))

    # ── Section 5: Frontend ──────────────────────────────
    story.append(Paragraph("5. Frontend Changes", styles["RH2"]))

    story.append(Paragraph("Upload helper + create flow", styles["RH3"]))
    story.append(bullet("<font face='Courier'>uploadMatchupItemImage(file)</font> in api.js mirrors "
                        "<font face='Courier'>uploadMatchupCoverImage</font>. Returns the S3 key "
                        "the backend expects on commit."))
    story.append(bullet("<font face='Courier'>createMatchup(userId, data)</font> now parallel-uploads "
                        "the cover + per-item images via <font face='Courier'>Promise.all</font>. A "
                        "4-item matchup with thumbnails on every item pays one round-trip-set "
                        "instead of five sequential."))

    story.append(Paragraph("CreateMatchup form", styles["RH3"]))
    story.append(bullet("Per-item state: each entry now carries "
                        "<font face='Courier'>{ item, imageFile, imagePreview }</font>."))
    story.append(bullet("Per-item file picker: dashed-border drop label \"+ Add thumbnail "
                        "(optional)\" / \"Replace thumbnail\". 48 px square preview swatch with a "
                        "Remove affordance on hover."))
    story.append(bullet("<font face='Courier'>URL.createObjectURL</font> / "
                        "<font face='Courier'>revokeObjectURL</font> dance: revoked on replace, on "
                        "remove, and on unmount. No object-URL references leaked across re-picks "
                        "or navigation."))

    story.append(Paragraph("Display surfaces", styles["RH3"]))
    surfaces = [
        [tc("Surface"), tc("Thumb size"), tc("Layout strategy")],
        [tc("MatchupPage .matchup-item"), tc("56 × 56 px"),
                                          tc("Mounts before the text label inside the existing flex row. flex-shrink: 0 keeps the thumb from squashing on narrow viewports.")],
        [tc("BracketView .bracket-row"), tc("22 × 22 px"),
                                          tc("Wrapped with the name in a new .bracket-row-name-wrap inside the first grid cell so the row's 2-column grid (name | votes) stays unchanged for items without thumbnails.")],
    ]
    story.append(make_table(surfaces[0], surfaces[1:],
                            col_widths=[2.2*inch, 1.0*inch, 3.3*inch]))
    story.append(Spacer(1, 4))
    story.append(Paragraph(
        "Both surfaces use <font face='Courier'>loading=\"lazy\"</font> + "
        "<font face='Courier'>decoding=\"async\"</font> so off-screen rows don't block paint. "
        "Empty <font face='Courier'>image_url</font> simply skips the &lt;img&gt; element — the "
        "rest of the card renders the existing text-only state.", styles["RBody"]))

    # ── Section 6: Comment-hierarchy bonus ───────────────
    story.append(Paragraph("6. Bonus — Comment Typography Hierarchy (commit d286274)", styles["RH2"]))
    story.append(Paragraph(
        "Browser-Claude ran a typography pass on prod comments between this cycle and the previous "
        "skip-button cycle and caught a small but real hierarchy bug: "
        "<font face='Courier'>.comment-card__author</font> was at 1.05 rem (~16.8 px) while the body "
        "sat at 1 rem (16 px). Mainstream discussion sites (Reddit, YouTube, HN, Substack) all keep "
        "author ≤ body because the comment is the lede, not the username.",
        styles["RBody"]))
    story.append(bullet("<font face='Courier'>.comment-card__author</font>: 1.05 rem → 0.95 rem "
                        "(~15 px). Now sits below body in the visual hierarchy."))
    story.append(bullet("<font face='Courier'>.comment-card__body</font> line-height: 1.5 → 1.45. "
                        "Slightly more cohesive feel without shrinking the size — 16 px stays as "
                        "the web minimum for comfortable reading on a discussion surface."))
    story.append(Paragraph(
        "Folded into this report (rather than its own dedicated PDF) because it's a 2-value CSS "
        "change — too small for a full-cycle write-up but worth the paper trail.",
        styles["RBody"]))

    # ── Section 7: Tests ─────────────────────────────────
    story.append(Paragraph("7. Tests", styles["RH2"]))
    story.append(Paragraph(
        "Two unit tests in <font face='Courier'>proto_mappers_test.go</font> (no integration tag — "
        "the pre-push hook + CI both run them on every push):", styles["RBody"]))
    story.append(bullet("<b>TestMatchupItemToProto.</b> Empty <font face='Courier'>ImagePath</font> "
                        "produces empty <font face='Courier'>image_url</font>. Non-empty path lifts "
                        "to a full S3 URL containing bucket, region, "
                        "<font face='Courier'>MatchupItemImages/</font> prefix, and filename. "
                        "Sub-test confirms label / vote count / public id round-trip."))
    story.append(bullet("<b>TestUploadKindMatchupItemRegistered.</b> Registry has a non-zero "
                        "<font face='Courier'>MaxBytes</font> below the matchup-cover ceiling and "
                        "the expected content-type allowlist. Without this, "
                        "<font face='Courier'>PresignUpload</font> would 400 on every "
                        "<font face='Courier'>matchup_item</font> request."))
    story.append(Paragraph(
        "Full integration of the upload + commit path needs an S3 mock (same pre-existing gap that "
        "limited cycle 6b's tests). Filed as part of the existing \"wire integration tests into "
        "CI + add an S3 mock harness\" follow-up.", styles["RBody"]))

    # ── Section 8: Diff ──────────────────────────────────
    story.append(Paragraph("8. Diff Summary", styles["RH2"]))
    diff = [
        [tc("File"), tc("Change")],
        [tc("api/migrations/026_matchup_item_image.sql"), tc("New: ALTER TABLE matchup_items ADD COLUMN image_path")],
        [tc("api/proto/common/v1/common.proto"), tc("MatchupItemResponse: + image_url field")],
        [tc("api/proto/matchup/v1/matchup.proto"), tc("MatchupItemCreate / AddItemRequest / UpdateItemRequest: + upload_key field")],
        [tc("api/gen/...pb.go"), tc("Regenerated via buf generate")],
        [tc("api/models/Matchup.go"), tc("MatchupItem.ImagePath; SaveMatchup INSERT now includes image_path")],
        [tc("api/db/db.go"), tc("ProcessMatchupItemImagePath / *Sized helpers")],
        [tc("api/controllers/upload_presign.go"), tc("UploadKindMatchupItem constant + 2 MB spec entry")],
        [tc("api/controllers/proto_mappers.go"), tc("matchupItemToProto: lift ImagePath to full S3 URL")],
        [tc("api/controllers/matchup_connect_handler.go"), tc("CreateMatchup: per-item upload commit before SaveMatchup")],
        [tc("api/controllers/vote_connect_handler.go"), tc("AddItem + UpdateItem: optional upload_key path; MatchupItemHandler.S3Client")],
        [tc("api/controllers/connect_routes.go"), tc("Pass S3Client into MatchupItemHandler")],
        [tc("api/controllers/proto_mappers_test.go"), tc("New unit tests for matchupItemToProto + UploadKindMatchupItem registry")],
        [tc("frontend/src/services/api.js"), tc("uploadMatchupItemImage helper; createMatchup parallel-uploads cover + per-item images")],
        [tc("frontend/src/pages/CreateMatchup.js"), tc("Per-item file picker + 48px preview + remove + blob URL revocation")],
        [tc("frontend/src/components/MatchupItem.js + MatchupItem.css"), tc("56 px thumbnail before label when image_url is set")],
        [tc("frontend/src/components/BracketView.js + BracketView.css"), tc("22 px thumbnail in name-wrap span; outer grid unchanged")],
    ]
    story.append(make_table(diff[0], diff[1:], col_widths=[2.6*inch, 3.9*inch]))
    story.append(Spacer(1, 4))
    story.append(Paragraph(
        "Single commit <font face='Courier'>3f7c799</font>. +540 / −49 across 19 files. Plus the "
        "tiny comment-hierarchy fix in <font face='Courier'>d286274</font> (15 / −2).",
        styles["RBody"]))

    # ── Section 9: Project state ─────────────────────────
    story.append(Paragraph("9. Project State After This Cycle", styles["RH2"]))
    state = [
        [tc("HEAD on main"), tc("3f7c799 — \"Add per-contender thumbnails on matchups + bracket rows\"")],
        [tc("Migrations"), tc("26 total (1 added this cycle: 026_matchup_item_image.sql)")],
        [tc("UX backlog progress"), tc("3 of 3 done. The bracket-research backlog is fully shipped.")],
        [tc("Upload kinds"), tc("3: avatar, matchup_cover, matchup_item.")],
        [tc("Unit tests added"), tc("2 (matchupItemToProto + uploadKinds registry)")],
        [tc("Pre-push hook"), tc("Validated cleanly on this push (full battery passes).")],
    ]
    story.append(make_table(["Metric", "Value"], state, col_widths=[2.2*inch, 4.3*inch]))

    story.append(Spacer(1, 10))

    story.append(Paragraph("UX backlog — final tally", styles["RH3"]))
    final = [
        [tc("Cycle"), tc("Item"), tc("Status")],
        [tc("6a"), tc("Bracket-match progress chip (\"Match X of Y · Round N\")"), tc("✓ shipped (548f3ba)")],
        [tc("6b"), tc("Skip / can't-decide affordance"), tc("✓ shipped (bf3f536)")],
        [tc("6c"), tc("Per-contender thumbnails"), tc("✓ shipped (3f7c799)")],
    ]
    story.append(make_table(final[0], final[1:], col_widths=[0.7*inch, 4.3*inch, 1.5*inch]))

    story.append(Spacer(1, 6))

    story.append(Paragraph("Deferred (filed as follow-ups)", styles["RH3"]))
    story.append(bullet("<b>CreateBracketPage entry-image upload.</b> Backend already supports it "
                        "(AddItem accepts upload_key). Just needs the bracket builder UI to expose "
                        "a file picker per entry. Smaller cycle now that the pipeline is in place."))
    story.append(bullet("<b>Update-existing-item image change.</b> UpdateItem accepts upload_key but "
                        "no UI exposes it. \"Edit a contender's thumbnail\" — separate cycle."))
    story.append(bullet("<b>Skip-rate dashboard from cycle 6b.</b> Surface "
                        "\"X% of voters skipped this round\" on the bracket page — closes the loop "
                        "on why the skip button was built."))
    story.append(bullet("<b>Wire integration tests into CI</b> + add an S3 mock harness so the "
                        "AddItem upload path can be tested end-to-end."))
    story.append(bullet("<b>CRA → Vite migration.</b> The path to closing the remaining 26 "
                        "Dependabot vulns. Real cycle's worth of build config, test runner swap, "
                        "env-var substitution differences."))

    story.append(Spacer(1, 10))
    story.append(hr())
    story.append(Paragraph(
        "<i>Generated by Claude Code. Closes the bracket-research backlog: progress chip → skip "
        "button → thumbnails. The first cycle was a pure frontend tweak, the second was a real "
        "schema change with a new RPC, and the third was an upload-pipeline extension touching "
        "13 backend files + 6 frontend files. Each was its own cycle on purpose — small, "
        "shippable units that compose without stepping on each other.</i>",
        styles["RSmall"]))

    doc.build(story)
    print(f"PDF written to {OUTPUT}")


if __name__ == "__main__":
    build()
