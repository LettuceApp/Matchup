from dagster import Definitions, load_assets_from_modules, in_process_executor
import assets as matchup_assets

# Load all assets from assets.py automatically.
all_assets = load_assets_from_modules([matchup_assets])

# Use in-process executor as the default for all jobs / asset materializations
defs = Definitions(
    assets=all_assets,
    executor=in_process_executor,
)
