# definitions.py

from dagster import Definitions, load_assets_from_modules
from . import assets as matchup_assets


# Load all assets from assets.py automatically.
# This will pick up: popular_matchups, users, matchups, trending, engagement, etc.
all_assets = load_assets_from_modules([matchup_assets])

# If you later add jobs, schedules, or sensors, you’ll register them here.
defs = Definitions(
    assets=all_assets,
)
