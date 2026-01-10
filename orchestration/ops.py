import os
import requests
import psycopg2
from dagster import op, DynamicOut, DynamicOutput, RetryPolicy


def _pg_dsn() -> str:
    """
    psycopg2 needs a postgres DSN. Your env might be SQLAlchemy style.
    We normalize it safely.
    """
    url = os.getenv("DATABASE_URL") or os.getenv("MATCHUP_DB_URL")
    if not url:
        raise Exception("Missing DATABASE_URL (or MATCHUP_DB_URL)")

    # SQLAlchemy url -> psycopg2 url
    url = url.replace("postgresql+psycopg2://", "postgresql://")
    return url


@op(out=DynamicOut(int))
def expired_bracket_ids():
    """
    Emits one bracket_id at a time for dynamic mapping.
    """
    dsn = _pg_dsn()
    conn = psycopg2.connect(dsn)
    cur = conn.cursor()

    cur.execute(
        """
        SELECT id
        FROM brackets
        WHERE
          status = 'active'
          AND advance_mode = 'timer'
          AND round_ends_at IS NOT NULL
          AND round_ends_at <= NOW()
        ORDER BY id ASC
        """
    )

    rows = cur.fetchall()
    cur.close()
    conn.close()

    # rows = [(1,), (2,), ...]
    for (bid,) in rows:
        yield DynamicOutput(int(bid), mapping_key=str(bid))


@op(
    retry_policy=RetryPolicy(max_retries=3),
)
def advance_bracket(bracket_id: int):
    """
    Side-effecting operation.
    Calls the Go API to advance a bracket.
    """
    api_url = os.getenv("API_URL")
    if not api_url:
        raise Exception("Missing API_URL")

    internal_key = os.getenv("INTERNAL_API_KEY") or os.getenv("API_SECRET")
    if not internal_key:
        raise Exception("Missing INTERNAL_API_KEY (or API_SECRET)")

    resp = requests.post(
        f"{api_url}/internal/brackets/{bracket_id}/advance",
        headers={"X-Internal-Key": internal_key},
        timeout=10,
    )

    if resp.status_code != 200:
        raise Exception(
            f"Failed to advance bracket {bracket_id}: "
            f"{resp.status_code} {resp.text}"
        )
