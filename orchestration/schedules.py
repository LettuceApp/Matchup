from dagster import ScheduleDefinition
from jobs import advance_expired_brackets, popular_matchups_job


# Job-based schedule (imperative)
advance_expired_brackets_schedule = ScheduleDefinition(
    job=advance_expired_brackets,
    cron_schedule="*/1 * * * *",
    description="Advance timer-based brackets whose round has expired",
    tags={"domain": "brackets", "type": "automation"},
)

# Asset-based schedule (implemented as a job schedule)
popular_matchups_minute_schedule = ScheduleDefinition(
    job=popular_matchups_job,
    cron_schedule="*/1 * * * *",
    description="Minute snapshot of top engaged matchups and brackets",
    tags={"domain": "analytics", "type": "snapshot"},
)
