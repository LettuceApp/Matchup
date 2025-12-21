from dagster import (
    Definitions,
    load_assets_from_modules,
    define_asset_job,
    ScheduleDefinition,
    in_process_executor,
)
import assets as matchup_assets

# Load all the assets
all_assets = load_assets_from_modules([matchup_assets])

# Job must reference the ACTUAL asset name
popular_matchups_job = define_asset_job(
    name="popular_matchups_job",
    selection=["popular_matchups"],
)

# Run this job hourly
popular_matchups_hourly_schedule = ScheduleDefinition(
    job=popular_matchups_job,
    cron_schedule="@hourly",
)

defs = Definitions(
    assets=all_assets,
    jobs=[popular_matchups_job],
    schedules=[popular_matchups_hourly_schedule],
    executor=in_process_executor,
)
