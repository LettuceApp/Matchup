import os
import pandas as pd
import sqlalchemy as sa
from dagster import asset, AssetExecutionContext

# ---------------------------------------------------------------------------
# DATABASE CONNECTION
# ---------------------------------------------------------------------------

DB_URL = os.getenv(
    "MATCHUP_DB_URL",
    "postgresql+psycopg2://cordelljenkins1914:Limitless12345@postgres:5432/matchup"
)

ENGINE = sa.create_engine(DB_URL)

# ---------------------------------------------------------------------------
# RAW TABLE ASSETS
# ---------------------------------------------------------------------------

@asset
def raw_matchups() -> pd.DataFrame:
    """Load all matchups from the database."""
    query = "SELECT * FROM matchups"
    with ENGINE.connect() as conn:
        return pd.read_sql(query, conn)


@asset
def raw_matchup_items() -> pd.DataFrame:
    """Load all matchup items (the two choices per matchup)."""
    query = "SELECT * FROM matchup_items"
    with ENGINE.connect() as conn:
        return pd.read_sql(query, conn)


@asset
def raw_likes() -> pd.DataFrame:
    """Load all likes for each matchup."""
    query = "SELECT * FROM likes"
    with ENGINE.connect() as conn:
        return pd.read_sql(query, conn)


@asset
def raw_comments() -> pd.DataFrame:
    """Load all comments for each matchup."""
    query = "SELECT * FROM comments"
    with ENGINE.connect() as conn:
        return pd.read_sql(query, conn)

# ---------------------------------------------------------------------------
# ENGAGEMENT SCORE ASSET
# ---------------------------------------------------------------------------

@asset
def matchup_engagement(
    raw_matchups: pd.DataFrame,
    raw_likes: pd.DataFrame,
    raw_comments: pd.DataFrame
) -> pd.DataFrame:
    """Compute engagement metrics for each matchup."""

    matchups = raw_matchups.copy()
    matchups.rename(columns={"id": "matchup_id"}, inplace=True)

    likes_count = raw_likes.groupby("matchup_id").size().reset_index(name="likes")
    comments_count = raw_comments.groupby("matchup_id").size().reset_index(name="comments")

    merged = (
        matchups
        .merge(likes_count, on="matchup_id", how="left")
        .merge(comments_count, on="matchup_id", how="left")
    )

    merged["likes"] = merged["likes"].fillna(0).astype(int)
    merged["comments"] = merged["comments"].fillna(0).astype(int)
    merged["total_votes"] = merged["likes"] + merged["comments"]
    merged["engagement_score"] = merged["total_votes"]

    return merged

# ---------------------------------------------------------------------------
# TRENDING MATCHUPS (TOP 20)
# ---------------------------------------------------------------------------

@asset
def trending_matchups(matchup_engagement: pd.DataFrame) -> pd.DataFrame:
    """Top 20 matchups sorted by engagement score."""
    df = matchup_engagement.sort_values("engagement_score", ascending=False)
    return df.head(20).reset_index(drop=True)

# ---------------------------------------------------------------------------
# TOP 5 POPULAR MATCHUPS — WRITES TO DB
# ---------------------------------------------------------------------------

@asset
def top_matchups(
    context: AssetExecutionContext,
    trending_matchups: pd.DataFrame
) -> pd.DataFrame:
    """
    Top 5 most-engaged matchups.
    Persisted to the `popular_matchups` database table
    for the API and frontend to consume.
    """
    df = trending_matchups.copy()

    df = df.sort_values("engagement_score", ascending=False).head(5).reset_index(drop=True)

    # Add ranking
    df["rank"] = range(1, len(df) + 1)

    # Persist to DB
    with ENGINE.begin() as conn:
        df.to_sql("popular_matchups", con=conn, if_exists="replace", index=False)

    context.log.info("Wrote top 5 popular matchups to table 'popular_matchups'")

    return df

@asset
def popular_matchups():
    """
    Creates and populates the 'popular_matchups' analytics table.
    """
    engine = sa.create_engine(DB_URL)

    # --- 1. LOAD RAW DATA FROM APP TABLES ---
    query = sa.text("""
        SELECT
            m.id AS matchup_id,
            m.title,
            m.author_id,

            -- Sum votes from matchup_items
            COALESCE(SUM(mi.votes), 0) AS total_votes,

            -- Count likes
            (
                SELECT COUNT(*)
                FROM likes l
                WHERE l.matchup_id = m.id
            ) AS likes,

            -- Count comments
            (
                SELECT COUNT(*)
                FROM comments c
                WHERE c.matchup_id = m.id
            ) AS comments

        FROM matchups m
        LEFT JOIN matchup_items mi ON mi.matchup_id = m.id
        GROUP BY m.id, m.title, m.author_id
    """)

    df = pd.read_sql(query, engine)

    # --- 2. COMPUTE ANALYTICS METRIC ---
    df["engagement_score"] = (
        df["total_votes"] * 5 +
        df["likes"] * 3 +
        df["comments"] * 2
    )

    # --- 3. RANK MATCHUPS ---
    df = df.sort_values("engagement_score", ascending=False)
    df["rank"] = range(1, len(df) + 1)

    # --- 4. MATERIALIZE INTO THE ANALYTICS TABLE ---
    df.to_sql(
        "popular_matchups",
        engine,
        if_exists="replace",
        index=False,
        dtype={
            "matchup_id": sa.Integer,
            "title": sa.Text,
            "author_id": sa.Integer,
            "total_votes": sa.Integer,
            "likes": sa.Integer,
            "comments": sa.Integer,
            "engagement_score": sa.Float,
            "rank": sa.Integer,
        }
    )

    return df
