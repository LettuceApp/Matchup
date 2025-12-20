# definitions.py
from dagster import (
    Definitions,
    load_assets_from_modules,
    define_asset_job,
    in_process_executor,
)
import assets as matchup_assets

# Load all assets from assets.py
all_assets = load_assets_from_modules([matchup_assets])

# Define a job that materializes all assets in-process
all_assets_job = (
    define_asset_job(
        name="all_assets_job",
        selection="*",
    )
    .configure_executor_def(in_process_executor)
)

defs = Definitions(
    assets=all_assets,
    jobs=[all_assets_job],
)
