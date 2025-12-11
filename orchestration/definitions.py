from dagster import Definitions, load_assets_from_modules
import assets as matchup_assets

# Load all assets from assets.py automatically.
all_assets = load_assets_from_modules([matchup_assets])

defs = Definitions(assets=all_assets)
