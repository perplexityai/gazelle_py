# Empty package marker. With `python_skip_empty_init = true`, this file must
# still ship in the generated library because `helpers_local.py` below uses a
# relative import (`from . import core`). Stripping it would break runtime
# import resolution.
