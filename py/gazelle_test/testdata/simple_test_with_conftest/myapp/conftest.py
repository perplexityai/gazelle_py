# conftest.py is hoisted out of the main library into its own
# `:conftest` py_library with testonly=True. The test rule depends on
# both `:myapp` and `:conftest` separately, so production code never
# pulls in test fixtures.
NAME = "world"
