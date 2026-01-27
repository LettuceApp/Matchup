import os
import time

import psycopg2
from dagster import RunRequest, SkipReason, sensor

from jobs import advance_expired_brackets


def _pg_dsn() -> str:
    url = os.getenv("DATABASE_URL") or os.getenv("MATCHUP_DB_URL")
    if not url:
        raise Exception("Missing DATABASE_URL (or MATCHUP_DB_URL)")
    return url.replace("postgresql+psycopg2://", "postgresql://")


def _has_expired_brackets() -> bool:
    dsn = _pg_dsn()
    conn = psycopg2.connect(dsn)
    cur = conn.cursor()
    cur.execute(
        """
        SELECT 1
        FROM brackets
        WHERE
          status = 'active'
          AND advance_mode = 'timer'
          AND round_ends_at IS NOT NULL
          AND round_ends_at <= NOW()
        LIMIT 1
        """
    )
    row = cur.fetchone()
    cur.close()
    conn.close()
    return row is not None


@sensor(job=advance_expired_brackets, minimum_interval_seconds=10)
def advance_expired_brackets_sensor():
    try:
        if not _has_expired_brackets():
            yield SkipReason("No expired brackets")
            return
    except Exception as exc:
        yield SkipReason(f"advance_expired_brackets sensor error: {exc}")
        return

    yield RunRequest(run_key=f"advance-expired-brackets-{int(time.time())}")
