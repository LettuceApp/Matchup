from dagster import Definitions, load_assets_from_modules, in_process_executor
import assets as matchup_assets

from jobs import advance_expired_brackets, popular_matchups_job
from schedules import (
    advance_expired_brackets_schedule,
    popular_matchups_hourly_schedule,
)

all_assets = load_assets_from_modules([matchup_assets])

defs = Definitions(
    assets=all_assets,
    jobs=[
        advance_expired_brackets,
        popular_matchups_job,
    ],
    schedules=[
        advance_expired_brackets_schedule,
        popular_matchups_hourly_schedule,
    ],
    executor=in_process_executor,  # 🔑 THIS IS THE FIX
)
