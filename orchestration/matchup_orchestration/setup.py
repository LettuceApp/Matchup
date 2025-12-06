from setuptools import find_packages, setup

setup(
    name="matchup_orchestration",
    packages=find_packages(exclude=["matchup_orchestration_tests"]),
    install_requires=[
        "dagster",
        "dagster-cloud"
    ],
    extras_require={"dev": ["dagster-webserver", "pytest"]},
)
