from matchup_orchestration import assets


def test_assets_module_imports():
    assert hasattr(assets, "raw_users")
