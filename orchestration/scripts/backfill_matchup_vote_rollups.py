import argparse
import os

import pandas as pd
import sqlalchemy as sa

DEFAULT_DB_URL = (
    "postgresql+psycopg2://cordelljenkins1914:Limitless12345@postgres:5432/matchup"
)


def _db_url() -> str:
    return os.getenv("MATCHUP_DB_URL", DEFAULT_DB_URL)


def _ensure_table(conn) -> None:
    conn.exec_driver_sql(
        """
        CREATE TABLE IF NOT EXISTS matchup_vote_rollups (
            matchup_id BIGINT PRIMARY KEY,
            legacy_votes INTEGER NOT NULL DEFAULT 0,
            created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
            updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
        )
        """
    )


def main() -> None:
    parser = argparse.ArgumentParser(
        description="Backfill matchup_vote_rollups from matchup_items."
    )
    parser.add_argument(
        "--overwrite",
        action="store_true",
        help="Replace existing rollups instead of inserting missing rows.",
    )
    args = parser.parse_args()

    engine = sa.create_engine(_db_url())
    with engine.begin() as conn:
        _ensure_table(conn)

        if args.overwrite:
            conn.exec_driver_sql("DELETE FROM matchup_vote_rollups")

        totals = pd.read_sql(
            """
            SELECT matchup_id, SUM(votes) AS legacy_votes
            FROM matchup_items
            GROUP BY matchup_id
            """,
            conn,
        )
        if totals.empty:
            print("No matchup item votes found to backfill.")
            return

        if not args.overwrite:
            existing = pd.read_sql(
                "SELECT matchup_id FROM matchup_vote_rollups",
                conn,
            )
            if not existing.empty:
                totals = totals[
                    ~totals["matchup_id"].isin(existing["matchup_id"])
                ]

        if totals.empty:
            print("No new rollups to insert.")
            return

        now = pd.Timestamp.utcnow()
        totals["legacy_votes"] = totals["legacy_votes"].fillna(0).astype(int)
        totals["created_at"] = now
        totals["updated_at"] = now

        totals.to_sql(
            "matchup_vote_rollups",
            conn,
            if_exists="append",
            index=False,
        )

        print(f"Inserted {len(totals)} matchup rollups.")


if __name__ == "__main__":
    main()
