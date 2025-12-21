from dagster import (
    Definitions,
    load_assets_from_modules,
    define_asset_job,
    ScheduleDefinition,
    in_process_executor,
)
import assets as matchup_assets

# Load all assets
all_assets = load_assets_from_modules([matchup_assets])

# Create a job that materializes ONLY the popular_matchup asset
popular_matchup_job = define_asset_job(
    name="popular_matchup_job",
    selection=["popular_matchup"]  # <-- must match your @asset name
)

# Run the job every hour
popular_matchup_hourly_schedule = ScheduleDefinition(
    job=popular_matchup_job,
    cron_schedule="@hourly"
)

defs = Definitions(
    assets=all_assets,
    schedules=[popular_matchup_hourly_schedule],
    jobs=[popular_matchup_job],
    executor=in_process_executor,
)
