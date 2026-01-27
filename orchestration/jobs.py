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
    selection=AssetSelection.keys(
        "popular_matchups",
        "popular_brackets",
        "home_summary_snapshot",
        "home_creators_snapshot",
        "home_new_this_week_snapshot",
    ).upstream(),
    tags={"domain": "analytics", "type": "snapshot"},
)
