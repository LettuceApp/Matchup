from dagster import Definitions, load_assets_from_modules, in_process_executor
import assets as matchup_assets

from jobs import advance_expired_brackets, popular_matchups_job
from schedules import popular_matchups_minute_schedule
from sensors import advance_expired_brackets_sensor

all_assets = load_assets_from_modules([matchup_assets])

defs = Definitions(
    assets=all_assets,
    jobs=[
        advance_expired_brackets,
        popular_matchups_job,
    ],
    schedules=[
        popular_matchups_minute_schedule,
    ],
    sensors=[
        advance_expired_brackets_sensor,
    ],
    executor=in_process_executor,  # 🔑 THIS IS THE FIX
)
