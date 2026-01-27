import os
import pandas as pd
import psycopg2
import sqlalchemy as sa
from dagster import asset, AssetExecutionContext

SERVING_TABLE = "popular_matchups_snapshot"
POPULAR_BRACKETS_TABLE = "popular_brackets_snapshot"
HOME_SUMMARY_TABLE = "home_summary_snapshot"
HOME_CREATORS_TABLE = "home_creators_snapshot"
HOME_NEW_THIS_WEEK_TABLE = "home_new_this_week_snapshot"

# ---------------------------------------------------------------------------
# DATABASE CONNECTION
# ---------------------------------------------------------------------------

DB_URL = os.getenv(
    "MATCHUP_DB_URL",
    "postgresql+psycopg2://cordelljenkins1914:Limitless12345@postgres:5432/matchup"
)

ENGINE = sa.create_engine(DB_URL)


def _pg_dsn() -> str:
    """
    psycopg2 expects a postgres DSN; normalize SQLAlchemy URLs safely.
    """
    url = os.getenv("MATCHUP_DB_URL") or os.getenv("DATABASE_URL") or DB_URL
    return url.replace("postgresql+psycopg2://", "postgresql://")

# ---------------------------------------------------------------------------
# RAW TABLE ASSETS (SOURCE OF TRUTH)
# ---------------------------------------------------------------------------

@asset
def raw_matchups() -> pd.DataFrame:
    with ENGINE.connect() as conn:
        return pd.read_sql("SELECT * FROM matchups", conn)


@asset
def raw_brackets() -> pd.DataFrame:
    with ENGINE.connect() as conn:
        return pd.read_sql("SELECT * FROM brackets", conn)


@asset
def raw_matchup_items() -> pd.DataFrame:
    with ENGINE.connect() as conn:
        return pd.read_sql("SELECT * FROM matchup_items", conn)


@asset
def raw_matchup_votes() -> pd.DataFrame:
    with ENGINE.connect() as conn:
        return pd.read_sql("SELECT * FROM matchup_votes", conn)

@asset
def raw_matchup_vote_rollups() -> pd.DataFrame:
    with ENGINE.connect() as conn:
        return pd.read_sql("SELECT * FROM matchup_vote_rollups", conn)


@asset
def raw_likes() -> pd.DataFrame:
    with ENGINE.connect() as conn:
        return pd.read_sql("SELECT * FROM likes", conn)


@asset
def raw_bracket_likes() -> pd.DataFrame:
    with ENGINE.connect() as conn:
        return pd.read_sql("SELECT * FROM bracket_likes", conn)


@asset
def raw_comments() -> pd.DataFrame:
    with ENGINE.connect() as conn:
        return pd.read_sql("SELECT * FROM comments", conn)


@asset
def raw_users() -> pd.DataFrame:
    with ENGINE.connect() as conn:
        return pd.read_sql("SELECT * FROM users", conn)


# ---------------------------------------------------------------------------
# HOME SNAPSHOTS
# ---------------------------------------------------------------------------

@asset
def home_summary_snapshot(
    context: AssetExecutionContext,
    raw_matchups: pd.DataFrame,
    raw_brackets: pd.DataFrame,
    raw_matchup_votes: pd.DataFrame,
) -> pd.DataFrame:
    now = pd.Timestamp.now(tz="UTC")
    votes_today = 0
    if not raw_matchup_votes.empty and "created_at" in raw_matchup_votes.columns:
        vote_times = pd.to_datetime(
            raw_matchup_votes["created_at"],
            errors="coerce",
            utc=True,
        )
        cutoff = now - pd.Timedelta(days=1)
        votes_today = int((vote_times >= cutoff).sum())

    active_matchups = 0
    if not raw_matchups.empty and "status" in raw_matchups.columns:
        active_matchups = int(
            raw_matchups["status"].isin(["active", "published"]).sum()
        )

    active_brackets = 0
    if not raw_brackets.empty and "status" in raw_brackets.columns:
        active_brackets = int((raw_brackets["status"] == "active").sum())

    df = pd.DataFrame(
        [
            {
                "votes_today": votes_today,
                "active_matchups": active_matchups,
                "active_brackets": active_brackets,
                "updated_at": now,
            }
        ]
    )

    with ENGINE.begin() as conn:
        df.to_sql(
            HOME_SUMMARY_TABLE,
            conn,
            if_exists="replace",
            index=False,
        )

    context.log.info("Home summary snapshot written.")

    return df


@asset
def home_creators_snapshot(
    context: AssetExecutionContext,
    raw_users: pd.DataFrame,
) -> pd.DataFrame:
    if raw_users.empty:
        df = pd.DataFrame(
            columns=[
                "user_id",
                "username",
                "avatar_path",
                "followers_count",
                "following_count",
                "is_private",
                "rank",
            ]
        )
    else:
        df = raw_users.copy()
        df = df[df["is_admin"] == False]
        df = df.sort_values(["followers_count", "id"], ascending=[False, True])
        df = df.head(10).reset_index(drop=True)
        df["rank"] = range(1, len(df) + 1)
        df = df.rename(columns={"id": "user_id"})
        df = df[
            [
                "user_id",
                "username",
                "avatar_path",
                "followers_count",
                "following_count",
                "is_private",
                "rank",
            ]
        ]

    with ENGINE.begin() as conn:
        df.to_sql(
            HOME_CREATORS_TABLE,
            conn,
            if_exists="replace",
            index=False,
        )

    context.log.info("Home creators snapshot written.")

    return df


@asset
def home_new_this_week_snapshot(
    context: AssetExecutionContext,
    raw_matchups: pd.DataFrame,
) -> pd.DataFrame:
    columns = [
        "matchup_id",
        "title",
        "author_id",
        "bracket_id",
        "created_at",
        "visibility",
        "rank",
    ]
    if raw_matchups.empty or "created_at" not in raw_matchups.columns:
        df = pd.DataFrame(columns=columns)
    else:
        df = raw_matchups.copy()
        df["created_at"] = pd.to_datetime(df["created_at"], errors="coerce", utc=True)
        cutoff = pd.Timestamp.now(tz="UTC") - pd.Timedelta(days=7)
        df = df[df["created_at"] >= cutoff]
        if "bracket_id" in df.columns:
            df = df[df["bracket_id"].isna()]
        if "visibility" not in df.columns:
            df["visibility"] = "public"
        df = df.sort_values("created_at", ascending=False).head(6)
        df = df.rename(columns={"id": "matchup_id"})
        df["rank"] = range(1, len(df) + 1)
        df = df[columns]

    with ENGINE.begin() as conn:
        df.to_sql(
            HOME_NEW_THIS_WEEK_TABLE,
            conn,
            if_exists="replace",
            index=False,
        )

    context.log.info("Home new-this-week snapshot written.")

    return df

# ---------------------------------------------------------------------------
# ENGAGEMENT SNAPSHOT
# ---------------------------------------------------------------------------

@asset
def matchup_engagement(
    raw_matchups: pd.DataFrame,
    raw_brackets: pd.DataFrame,
    raw_matchup_items: pd.DataFrame,
    raw_matchup_votes: pd.DataFrame,
    raw_likes: pd.DataFrame,
    raw_comments: pd.DataFrame,
) -> pd.DataFrame:
    """
    Computes engagement score.
    - Excludes self-likes
    - Excludes self-comments
    - Excludes interactions from bracket owners
    - Votes exclude matchup authors and bracket owners
    - Legacy votes fall back to matchup_items totals if no per-user votes exist
    """

    matchups = raw_matchups.rename(columns={"id": "matchup_id"})
    brackets = raw_brackets.rename(
        columns={
            "id": "bracket_id",
            "author_id": "bracket_author_id",
        }
    )
    matchups = matchups.merge(
        brackets[["bracket_id", "bracket_author_id", "current_round"]],
        on="bracket_id",
        how="left",
    )

    # ------------------------
    # Votes (exclude matchup and bracket owners)
    # ------------------------
    if raw_matchup_votes.empty:
        per_user_votes = pd.DataFrame(columns=["matchup_id", "votes"])
        per_user_votes_total = pd.DataFrame(columns=["matchup_id", "total_votes"])
    else:
        matchup_votes = raw_matchup_votes.copy()
        if "matchup_public_id" in matchup_votes.columns:
            matchup_votes = matchup_votes.merge(
                matchups[["matchup_id", "public_id", "author_id", "bracket_author_id"]],
                left_on="matchup_public_id",
                right_on="public_id",
                how="left",
            )
        else:
            matchup_votes = matchup_votes.merge(
                matchups[["matchup_id", "author_id", "bracket_author_id"]],
                on="matchup_id",
                how="left",
            )

        if "matchup_id" in matchup_votes.columns:
            matchup_votes = matchup_votes.dropna(subset=["matchup_id"])

        per_user_votes_total = (
            matchup_votes
            .groupby("matchup_id")
            .size()
            .reset_index(name="total_votes")
        )
        per_user_votes = (
            matchup_votes
            .query("user_id != author_id and user_id != bracket_author_id")
            .groupby("matchup_id")
            .size()
            .reset_index(name="votes")
        )

    if raw_matchup_items.empty:
        matchup_item_votes = pd.DataFrame(columns=["matchup_id", "item_votes"])
    else:
        matchup_item_votes = (
            raw_matchup_items
            .groupby("matchup_id")["votes"]
            .sum()
            .reset_index(name="item_votes")
        )

    votes = (
        per_user_votes
        .merge(per_user_votes_total, on="matchup_id", how="outer")
        .merge(matchup_item_votes, on="matchup_id", how="outer")
    )
    if votes.empty:
        votes = pd.DataFrame(columns=["matchup_id", "votes"])
    else:
        votes["votes"] = votes["votes"].fillna(0).astype(int)
        votes["total_votes"] = votes["total_votes"].fillna(0).astype(int)
        votes["item_votes"] = votes["item_votes"].fillna(0).astype(int)
        votes["legacy_votes"] = (
            (votes["item_votes"] - votes["total_votes"])
            .clip(lower=0)
            .astype(int)
        )
        votes["votes"] = votes["votes"] + votes["legacy_votes"]
        votes = votes[["matchup_id", "votes"]]

    # ------------------------
    # Likes (exclude author)
    # ------------------------
    likes = (
        raw_likes
        .merge(
            matchups[["matchup_id", "author_id", "bracket_author_id"]],
            on="matchup_id",
            how="left",
        )
        .query("user_id != author_id and user_id != bracket_author_id")
        .groupby("matchup_id")
        .size()
        .reset_index(name="likes")
    )

    # ------------------------
    # Comments (exclude author)
    # ------------------------
    comments = (
        raw_comments
        .merge(
            matchups[["matchup_id", "author_id", "bracket_author_id"]],
            on="matchup_id",
            how="left",
        )
        .query("user_id != author_id and user_id != bracket_author_id")
        .groupby("matchup_id")
        .size()
        .reset_index(name="comments")
    )

    # ------------------------
    # Merge all
    # ------------------------
    df = (
        matchups[
            [
                "matchup_id",
                "title",
                "author_id",
                "bracket_id",
                "bracket_author_id",
                "round",
                "current_round",
            ]
        ]
        .merge(votes, on="matchup_id", how="left")
        .merge(likes, on="matchup_id", how="left")
        .merge(comments, on="matchup_id", how="left")
    )

    df[["votes", "likes", "comments"]] = (
        df[["votes", "likes", "comments"]]
        .fillna(0)
        .astype(int)
    )

    df["engagement_score"] = (
        df["votes"] * 2 +
        df["likes"] * 3 +
        df["comments"] * .5
    )

    return df


# ---------------------------------------------------------------------------
# BRACKET ENGAGEMENT (CURRENT ROUND)
# ---------------------------------------------------------------------------

@asset
def bracket_engagement(
    matchup_engagement: pd.DataFrame,
    raw_brackets: pd.DataFrame,
    raw_bracket_likes: pd.DataFrame,
) -> pd.DataFrame:
    """
    Sum of matchup engagement scores for each bracket's current round.
    """
    if matchup_engagement.empty:
        return pd.DataFrame(
            columns=[
                "bracket_id",
                "bracket_author_id",
                "current_round",
                "matchup_count",
                "bracket_engagement_score",
            ]
        )

    df = (
        matchup_engagement[
            matchup_engagement["bracket_id"].notna()
            & matchup_engagement["round"].notna()
            & matchup_engagement["current_round"].notna()
        ]
        .copy()
    )

    if df.empty:
        return pd.DataFrame(
            columns=[
                "bracket_id",
                "bracket_author_id",
                "current_round",
                "matchup_count",
                "bracket_engagement_score",
            ]
        )

    df["round"] = df["round"].astype(int)
    df["current_round"] = df["current_round"].astype(int)

    current_round = df[df["round"] == df["current_round"]]

    bracket_scores = (
        current_round
        .groupby(["bracket_id", "bracket_author_id", "current_round"])
        .agg(
            matchup_count=("matchup_id", "count"),
            bracket_engagement_score=("engagement_score", "sum"),
        )
        .reset_index()
    )

    if raw_bracket_likes.empty:
        bracket_likes = pd.DataFrame(columns=["bracket_id", "bracket_likes"])
    else:
        brackets = raw_brackets.rename(
            columns={"id": "bracket_id", "author_id": "bracket_author_id"}
        )
        bracket_likes = (
            raw_bracket_likes
            .merge(
                brackets[["bracket_id", "bracket_author_id"]],
                on="bracket_id",
                how="left",
            )
            .query("user_id != bracket_author_id")
            .groupby("bracket_id")
            .size()
            .reset_index(name="bracket_likes")
        )

    bracket_scores = bracket_scores.merge(
        bracket_likes,
        on="bracket_id",
        how="left",
    )
    bracket_scores["bracket_likes"] = (
        bracket_scores["bracket_likes"]
        .fillna(0)
        .astype(int)
    )
    bracket_scores["bracket_engagement_score"] = (
        bracket_scores["bracket_engagement_score"] +
        bracket_scores["bracket_likes"] * 3
    )

    return bracket_scores[
        [
            "bracket_id",
            "bracket_author_id",
            "current_round",
            "matchup_count",
            "bracket_engagement_score",
        ]
    ]


# ---------------------------------------------------------------------------
# POPULAR BRACKETS SNAPSHOT (TOP 5)
# ---------------------------------------------------------------------------

@asset
def popular_brackets(
    context: AssetExecutionContext,
    bracket_engagement: pd.DataFrame,
    raw_brackets: pd.DataFrame,
) -> pd.DataFrame:
    """
    Materialized snapshot of the top 5 most-engaged brackets.
    """
    if bracket_engagement.empty:
        df = pd.DataFrame(
            columns=[
                "bracket_id",
                "title",
                "author_id",
                "current_round",
                "matchup_count",
                "bracket_engagement_score",
                "rank",
            ]
        )
    else:
        brackets = raw_brackets.rename(columns={"id": "bracket_id"})
        df = bracket_engagement.copy()
        df = df[df["bracket_engagement_score"] > 0]

        if df.empty:
            df = pd.DataFrame(
                columns=[
                    "bracket_id",
                    "title",
                    "author_id",
                    "current_round",
                    "matchup_count",
                    "bracket_engagement_score",
                    "rank",
                ]
            )
        else:
            df["author_id"] = df["bracket_author_id"]
            df = df.merge(
                brackets[["bracket_id", "title"]],
                on="bracket_id",
                how="left",
            )
            df = (
                df.sort_values("bracket_engagement_score", ascending=False)
                .head(5)
                .reset_index(drop=True)
            )
            df["rank"] = range(1, len(df) + 1)
            df = df[
                [
                    "bracket_id",
                    "title",
                    "author_id",
                    "current_round",
                    "matchup_count",
                    "bracket_engagement_score",
                    "rank",
                ]
            ]

            assert df["bracket_engagement_score"].min() >= 0

    with ENGINE.begin() as conn:
        df.to_sql(
            POPULAR_BRACKETS_TABLE,
            conn,
            if_exists="replace",
            index=False,
        )

    context.log.info("Popular brackets snapshot written.")

    return df


# ---------------------------------------------------------------------------
# POPULAR MATCHUPS SNAPSHOT (TOP 5)
# ---------------------------------------------------------------------------

@asset
def popular_matchups(
    context: AssetExecutionContext,
    matchup_engagement: pd.DataFrame,
) -> pd.DataFrame:

    if matchup_engagement.empty:
        df = pd.DataFrame(
            columns=[
                "matchup_id",
                "title",
                "author_id",
                "bracket_id",
                "bracket_author_id",
                "round",
                "current_round",
                "votes",
                "likes",
                "comments",
                "engagement_score",
                "rank",
            ]
        )
    else:
        df = matchup_engagement.copy()
        df = df[df["engagement_score"] > 0]

        if df.empty:
            df = pd.DataFrame(
                columns=[
                    "matchup_id",
                    "title",
                    "author_id",
                    "bracket_id",
                    "bracket_author_id",
                    "round",
                    "current_round",
                    "votes",
                    "likes",
                    "comments",
                    "engagement_score",
                    "rank",
                ]
            )
        else:
            df = (
                df.sort_values("engagement_score", ascending=False)
                .head(5)
                .reset_index(drop=True)
            )
            df["rank"] = range(1, len(df) + 1)

            assert df["engagement_score"].min() >= 0
            assert df["engagement_score"].is_monotonic_decreasing

    with ENGINE.begin() as conn:
        df.to_sql(
            SERVING_TABLE,
            conn,
            if_exists="replace",
            index=False,
        )

    context.log.info("Popular matchups snapshot written.")

    return df

@asset
def expired_brackets():
    conn = psycopg2.connect(_pg_dsn())
    cur = conn.cursor()

    cur.execute("""
        SELECT id
        FROM brackets
        WHERE
          status = 'active'
          AND advance_mode = 'timer'
          AND round_ends_at IS NOT NULL
          AND round_ends_at <= NOW()
    """)

    rows = cur.fetchall()
    cur.close()
    conn.close()

    return [row[0] for row in rows]
