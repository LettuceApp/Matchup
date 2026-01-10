from dagster import job, define_asset_job, AssetSelection
from ops import expired_bracket_ids, advance_bracket


@job
def advance_expired_brackets():
    """
    Advancing all expired brackets (dynamic mapping).
    """
    expired_bracket_ids().map(advance_bracket)


# Asset job (analytical): runs only the selected assets
popular_matchups_job = define_asset_job(
    name="popular_matchups_job",
    selection=AssetSelection.keys("popular_matchups", "popular_brackets").upstream(),
    tags={"domain": "analytics", "type": "snapshot"},
)
